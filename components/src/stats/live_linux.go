package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const runtimeDir = "/run/x6-stats-overlay"
const tracing = "/sys/kernel/tracing"
const inst = tracing + "/instances/x6_stats"

func mono() float64 {
	var t syscall.Timespec
	_, _, e := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&t)), 0)
	if e != 0 {
		panic(e)
	}
	return float64(t.Sec) + float64(t.Nsec)/1e9
}
func put(p, s string) error { return os.WriteFile(p, []byte(s), 0600) }
func atomic(p string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(p), ".stats-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(b)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}
func byteRead(p string) (byte, error) {
	fd, e := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return 0, e
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return 0, e
	}
	if !s.Mode().IsRegular() || s.Mode().Perm()&0022 != 0 || s.Sys().(*syscall.Stat_t).Uid != 0 {
		return 0, errors.New("unsafe file")
	}
	b, e := io.ReadAll(io.LimitReader(f, 2))
	if e != nil || len(b) != 1 || b[0] > 1 {
		return 0, errors.New("invalid byte")
	}
	return b[0], nil
}
func status(text string, sev uint32) error {
	text = temperatures.decorate(text)
	if len(text) > 220 {
		return errors.New("status too long")
	}
	b := make([]byte, 16+len(text)+1)
	binary.LittleEndian.PutUint64(b, uint64(mono()))
	binary.LittleEndian.PutUint32(b[8:], sev)
	copy(b[16:], text)
	return atomic(runtimeDir+"/status", b)
}
func temp() float64 {
	ps, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/type")
	for _, p := range ps {
		b, e := os.ReadFile(p)
		if e == nil && strings.TrimSpace(string(b)) == "soc-chip-therm" {
			b, e = os.ReadFile(filepath.Dir(p) + "/temp")
			if e != nil {
				return -999
			}
			v, e := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			if e == nil {
				return v / 1000
			}
		}
	}
	return -999
}
func media() (string, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/usr/bin/ins_bll_cli", `{"cmd":"share_state","action":"get","type":"media"}`).Output()
	if e != nil {
		return "", 0, e
	}
	var r struct {
		Ret  string `json:"ret"`
		Data struct {
			Record struct {
				Status string `json:"record_status"`
				Time   int64  `json:"record_time_ms"`
			} `json:"record"`
		} `json:"data"`
	}
	e = json.Unmarshal(b, &r)
	if e != nil || r.Ret != "ok" {
		return "", 0, errors.New("invalid media response")
	}
	return r.Data.Record.Status, r.Data.Record.Time, nil
}
func kcmd(s string) error {
	f, e := os.OpenFile(tracing+"/kprobe_events", os.O_WRONLY|os.O_APPEND, 0)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = fmt.Fprintln(f, s)
	return e
}
func counters() (map[string]uint64, error) {
	out := map[string]uint64{}
	ps, e := filepath.Glob(inst + "/per_cpu/cpu*/stats")
	if e != nil || len(ps) == 0 {
		return nil, errors.New("missing trace counters")
	}
	for _, p := range ps {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		for _, l := range strings.Split(string(b), "\n") {
			v := strings.Split(l, ":")
			if len(v) == 2 && (strings.Contains(v[0], "overrun") || strings.Contains(v[0], "dropped events")) {
				n, e := strconv.ParseUint(strings.TrimSpace(v[1]), 10, 64)
				if e != nil {
					return nil, e
				}
				out["trace:"+p+":"+v[0]] = n
			}
		}
	}
	b, e := os.ReadFile(tracing + "/kprobe_profile")
	if e != nil {
		return nil, e
	}
	found := 0
	for _, l := range strings.Split(string(b), "\n") {
		v := strings.Fields(l)
		if len(v) != 3 {
			continue
		}
		n := v[0]
		if i := strings.LastIndex(n, "/"); i >= 0 {
			n = n[i+1:]
		}
		switch n {
		case "x6stats_hfi", "x6stats_batch_frame", "x6stats_input_response", "x6stats_output_response":
			val, e := strconv.ParseUint(v[2], 10, 64)
			if e != nil {
				return nil, e
			}
			prefix := "input:"
			if n == "x6stats_output_response" {
				prefix = "output:"
			}
			out[prefix+n] = val
			found++
		}
	}
	if found != 4 {
		return nil, errors.New("missing probe counters")
	}
	return out, nil
}
func live() error {
	pref := ""
	initial := byte(0)
	if os.Getenv("X6_STATS_PERSIST") == "1" {
		pref = "/data/x6-owner-features/stats-v1/enabled.pref"
		var err error
		initial, err = readPreference(pref)
		if err != nil {
			return err
		}
	}
	rel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return err
	}
	module, err := os.ReadFile("/lib/modules/" + strings.TrimSpace(string(rel)) + "/msm_video.ko")
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(module)) != "257fdcb7c7cef4dc2c4c6d59c1486a693afb1338b688f8113f030197213333bf" {
		return errors.New("unrecognized video module")
	}
	if os.Geteuid() != 0 {
		return errors.New("root required")
	}
	s, e := os.Lstat(runtimeDir)
	if e != nil || !s.IsDir() || s.Mode().Perm()&0022 != 0 {
		return errors.New("unsafe runtime")
	}
	if _, e = os.Lstat(inst); !os.IsNotExist(e) {
		return errors.New("trace instance exists")
	}
	if e = os.Mkdir(inst, 0700); e != nil {
		return e
	}
	registered := []string{}
	var pipe *os.File
	defer func() {
		put(inst+"/tracing_on", "0")
		put(inst+"/events/x6_stats/enable", "0")
		if pipe != nil {
			pipe.Close()
		}
		for _, n := range registered {
			kcmd("-:x6_stats/" + n)
		}
		os.Remove(inst)
		atomic(runtimeDir+"/state", []byte{0})
		os.Remove(runtimeDir + "/status")
	}()
	common := " sid=+332($arg1):u32 domain=+308($arg1):u32 width=+1880($arg1):u32 height=+1884($arg1):u32"
	defs := [][3]string{{"hfi", "venus_hfi_queue_buffer", common + " type=+24($arg2):u32 index=+32($arg2):u32 pts=+72($arg2):u64 addr=+56($arg2):x64"}, {"input_response", "handle_input_buffer", common + " type=+0($arg2):u32 index=+4($arg2):u32 pts=+32($arg2):u64 addr=+8($arg2):x64 bytes=+28($arg2):u32"}, {"output_response", "handle_output_buffer", common + " type=+0($arg2):u32 index=+4($arg2):u32 pts=+32($arg2):u64 addr=+8($arg2):x64 bytes=+28($arg2):u32"}}
	// On this exact driver, high-speed sessions submit multiple pictures through a
	// super-buffer. At +0x334 x7 points to the individual firmware packet,
	// x19 to the session and x22 to the original input buffer. An index and an
	// address are shared across pictures; use the individual PTS as well.
	defs = append(defs, [3]string{"batch_frame", "venus_hfi_queue_super_buffer+0x334",
		" sid=+332(%x19):u32 domain=+308(%x19):u32 width=+1880(%x19):u32 height=+1884(%x19):u32" +
			" type=+24(%x22):u32 index=+4(%x7):u32 pts=+32(%x7):u64 addr=+8(%x7):x64"})
	for _, d := range defs {
		if e = kcmd("p:x6_stats/x6stats_" + d[0] + " msm_video:" + d[1] + d[2]); e != nil {
			return e
		}
		registered = append(registered, "x6stats_"+d[0])
		filter := "domain==1 && width>=1280 && height>=720"
		if d[0] == "hfi" || d[0] == "batch_frame" {
			filter += " && type==1"
		}
		if e = put(inst+"/events/x6_stats/x6stats_"+d[0]+"/filter", filter); e != nil {
			return e
		}
	}
	for p, v := range map[string]string{"trace_clock": "mono", "buffer_size_kb": "128", "tracing_on": "1", "events/x6_stats/enable": "0"} {
		if e = put(inst+"/"+p, v); e != nil {
			return e
		}
	}
	pipe, e = os.Open(inst + "/trace_pipe")
	if e != nil {
		return e
	}
	events := make(chan measuredEvent, 8192)
	fail := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		sc := bufio.NewScanner(pipe)
		for sc.Scan() {
			l := sc.Text()
			if strings.Contains(l, "LOST") {
				select {
				case fail <- errors.New("trace lost"):
				default:
				}
				continue
			}
			if ev, ok := parseMeasurement(l); ok {
				select {
				case events <- ev:
				case <-done:
					return
				}
			}
		}
		select {
		case fail <- fmt.Errorf("trace ended: %v", sc.Err()):
		case <-done:
		}
	}()
	atomic(runtimeDir+"/state", []byte{0})
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	requests := time.NewTicker(200 * time.Millisecond)
	defer requests.Stop()
	enabled := false
	m := fresh()
	idle := true
	firstAfterEnable := false
	lastMS := int64(-1)
	var lastText string
	var lastSev uint32
	lastError := false
	baseline, baseErr := counters()
	previous := baseline
	var lastDiagnostic string
	var route routing
	consume := func(ev measuredEvent) {
		if enabled {
			if idle {
				if ev.Kind != "hfi" {
					return
				}
				m = fresh()
				route = routing{}
				idle = false
				lastText = ""
				lastError = false
				fmt.Println("NEW_RECORD")
			}
			route.accept(m, ev)
		}
	}
	if pref != "" {
		if e = atomic(runtimeDir+"/request", []byte{initial}); e != nil {
			return e
		}
	}
	fmt.Println("READY; waiting for main encoder profile")
	for {
		select {
		case <-stop:
			return nil
		case err := <-fail:
			m.Lost = true
			m.LossReason = "trace_reader: " + err.Error()
			lastError = true
			fmt.Println(err)
		case ev := <-events:
			consume(ev)
		case <-requests.C:
			if _, e = os.Lstat(runtimeDir + "/request"); os.IsNotExist(e) {
				continue
			}
			v, er := byteRead(runtimeDir + "/request")
			os.Remove(runtimeDir + "/request")
			if er != nil {
				fmt.Println("REJECT", er)
				continue
			}
			if pref != "" {
				if er = savePreference(pref, v); er != nil {
					fmt.Println("REJECT preference", er)
					continue
				}
			}
			if v == 1 && !enabled {
				m = fresh()
				route = routing{}
				idle = true
				firstAfterEnable = true
				lastError = false
				baseline, baseErr = counters()
				if e = put(inst+"/events/x6_stats/enable", "1"); e != nil {
					return e
				}
			} else if v == 0 && enabled {
				if e = put(inst+"/events/x6_stats/enable", "0"); e != nil {
					return e
				}
				m = fresh()
				route = routing{}
				idle = true
				os.Remove(runtimeDir + "/status")
				os.Remove(runtimeDir + "/thermal-expanded")
			}
			enabled = v == 1
			if e = atomic(runtimeDir+"/state", []byte{v}); e != nil {
				return e
			}
			fmt.Println("ENABLED", enabled)
		case <-tick.C:
			if !enabled {
				continue
			}
			st, ms, err := media()
			drain(events, consume)
			if err != nil {
				status("Stats unavailable", 3)
				continue
			}
			now, ce := counters()
			outputLost, inputLost, reason := lossSince(baseline, now)
			if ce != nil || baseErr != nil {
				outputLost = true
				reason = "counter_read_error"
			}
			if outputLost {
				m.Lost = true
				if m.LossReason == "" {
					m.LossReason = reason
				}
			}
			_, inputRecent, _ := lossSince(previous, now)
			previous = now
			if inputRecent {
				for _, s := range m.Streams {
					s.InputInvalidUntil = mono() + 1
				}
			}
			if st == "RecordIdle" {
				idle = true
				firstAfterEnable = false
				lastMS = -1
				lastError = false
				baseline, baseErr = now, ce
				status(fmt.Sprintf("Return idle\nRate stopped\nGaps --/--\nSoC %s C", socDegrees(temp())), 3)
				continue
			}
			if st != "RecordRunning" {
				status("Stats sampling", 3)
				continue
			}
			if firstAfterEnable {
				m.Lost = true
				m.LossReason = "enabled_mid_record"
				firstAfterEnable = false
			}
			if lastMS >= 0 && ms < lastMS {
				m = fresh()
				route = routing{}
				m.Lost = true
				m.LossReason = "record_transition_not_observed"
				lastError = false
				baseline, baseErr = now, ce
			}
			lastMS = ms
			if lastError {
				m.Lost = true
			}
			if route.profile.Eyes == 0 {
				if p, ok := recordingSource(); ok {
					route.bind(m, p)
				} else {
					status("Stats sampling", 3)
					continue
				}
			}
			lastText, lastSev = m.display(mono(), temp())
			diag, _ := json.Marshal(map[string]any{"record_ms": ms, "main_profile": route.profile, "streams": m.Streams, "pending": len(m.Pending), "output_invalid": m.Lost, "output_reason": m.LossReason, "counter_output_lost": outputLost, "counter_input_lost": inputLost, "counter_reason": reason})
			atomic(runtimeDir+"/diagnostic.json", diag)
			if m.LossReason != "" && m.LossReason != lastDiagnostic {
				fmt.Println("OUTPUT_INVALID", m.LossReason)
				lastDiagnostic = m.LossReason
			}
			if e = status(lastText, lastSev); e != nil {
				return e
			}
		}
	}
}
