// Local fixed-purpose recording policy. No network listener or arbitrary commands.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var base = "/data/x6-owner-features/preview-v1"

const runtime = "/run/x6-preview-control"

func dir(p string) error {
	s, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !s.IsDir() || s.Mode().Perm()&0022 != 0 || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return errors.New("unsafe directory")
	}
	return nil
}
func read(p string) (byte, error) {
	if e := dir(filepath.Dir(p)); e != nil {
		return 0, e
	}
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
	if !s.Mode().IsRegular() || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) || s.Mode().Perm()&0022 != 0 {
		return 0, errors.New("unsafe preference")
	}
	b, e := io.ReadAll(io.LimitReader(f, 2))
	if e != nil {
		return 0, e
	}
	if len(b) != 1 || b[0] > 1 {
		return 0, errors.New("invalid preference")
	}
	return b[0], nil
}
func save(p string, v byte) error {
	if v > 1 {
		return errors.New("invalid preference")
	}
	if e := dir(filepath.Dir(p)); e != nil {
		return e
	}
	if s, e := os.Lstat(p); e == nil && !s.Mode().IsRegular() {
		return errors.New("unsafe destination")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".preview-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write([]byte{v}); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), p); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(p))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func command(name string, args ...string) ([]byte, error) {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(c, name, args...).CombinedOutput()
}

type media struct {
	Capture struct {
		Status string `json:"capture_status"`
	} `json:"capture"`
	Record struct {
		Status string `json:"record_status"`
		Time   int64  `json:"record_time_ms"`
		Data   int64  `json:"record_data_time_ms"`
	} `json:"record"`
	Pipeline struct {
		Status struct {
			States []int `json:"status"`
		} `json:"pipeline_status"`
	} `json:"pipeline"`
}

func getMedia() (media, error) {
	var m media
	b, e := command("/usr/bin/ins_bll_cli", `{"cmd":"share_state","action":"get","type":"media"}`)
	if e != nil {
		return m, e
	}
	var r struct {
		Ret  string `json:"ret"`
		Data media  `json:"data"`
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return m, e
	}
	if r.Ret != "ok" || r.Data.Record.Status == "" {
		return m, errors.New("invalid media response")
	}
	return r.Data, nil
}

// Only the native preview stream is controlled. Never stop the display, GUI,
// recording/audio streams, proxy, Stats service or shared camera pipeline.
var runCommand = command

func stream(start bool) error {
	method := "PipelineStopStream"
	if start {
		method = "PipelineStartStream"
	}
	b, e := runCommand("/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kMediad", "Pipeline", method, "StreamTypePreview")
	if e != nil || !strings.Contains(string(b), "Error code: Success(0)") {
		return fmt.Errorf("preview %s failed: %v %.300s", method, e, b)
	}
	return nil
}

// 1.2.36 Ambisonic emits four-channel AAC while its native audio stream
// stays Starting and the legacy kAudioEnc FPS counter stays zero. Restrict
// compatibility to explicit Ambisonic plus fresh advancing record data.
func isAmbisonic() bool {
	b, e := runCommand("/usr/bin/ins_bll_cli", `{"cmd":"config","action":"get_general","key":"kAudioAlgoMode"}`)
	if e != nil {
		return false
	}
	var r struct {
		Ret  string `json:"ret"`
		Data struct {
			Mode string `json:"algo_mode"`
		} `json:"data"`
	}
	return json.Unmarshal(b, &r) == nil && r.Ret == "ok" && r.Data.Mode == "kAmbisonic"
}

// Factory profiles with two active main streams use the original readiness check.
// The square FreeFrame profile has just one main encoder on either physical lens.
func recordingFreeFrame() (bool, error) {
	raw, err := runCommand("/usr/bin/ins_bll_cli", `{"cmd":"config","action":"get_general","key":"kMediaMode"}`)
	if err != nil {
		return false, err
	}
	var r struct {
		Ret  string
		Data struct {
			Mode string `json:"shoot_mode"`
			Lens string `json:"lens_mode"`
		}
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return false, err
	}
	if r.Ret != "ok" || r.Data.Mode == "" || r.Data.Lens == "" {
		return false, errors.New("unavailable recording mode")
	}
	return r.Data.Mode == "kFreeFrameVideo" && r.Data.Lens == "kSATAngle", nil
}

type policy struct {
	singleProfile   func() (bool, error)
	proxyPreference func() (byte, error)
	profileKnown    bool
	single          bool
	nextCheck       int64
	lastData        int64
	progress        int
	ambisonic       func() bool
	owned           bool
	last            int64
	running         bool
	attempts        int
	mark            func(byte) error
	call            func(bool) error
}

func (p *policy) restore() error {
	if !p.owned {
		return nil
	}
	if e := p.call(true); e != nil {
		return e
	}
	if e := p.mark(0); e != nil {
		return e
	}
	p.owned = false
	p.attempts = 0
	log.Print("PREVIEW_RESTORED")
	return nil
}
func (p *policy) step(enabled bool, m media) error {
	running := m.Record.Status == "RecordRunning"
	if p.owned && (!enabled || m.Record.Status == "RecordIdle") {
		if e := p.restore(); e != nil {
			return e
		}
	}
	if m.Record.Status == "RecordIdle" {
		p.running = false
		p.progress = 0
		p.lastData = 0
		p.attempts = 0
		return nil
	}
	if !running || m.Capture.Status != "CaptureIdle" {
		return nil
	}
	if !p.running || m.Record.Time < p.last {
		p.profileKnown = false
		p.single = false
		p.nextCheck = 0
		p.attempts = 0
		p.progress = 0
		p.lastData = 0
	}
	if m.Record.Data > p.lastData {
		if p.progress < 3 {
			p.progress++
		}
	} else {
		p.progress = 0
	}
	p.lastData = m.Record.Data
	p.running = true
	p.last = m.Record.Time
	// RecordRunning precedes completion of qmmf stream transitions.
	// Give initial proxy/graph removal time to settle before removing preview.
	if !enabled || len(m.Pipeline.Status.States) < 9 || m.Pipeline.Status.States[0] != 3 || m.Pipeline.Status.States[1] != 3 || p.attempts >= 3 {
		return nil
	}
	if secondary := m.Pipeline.Status.States[2]; secondary != 3 {
		// An unused second encoder can be unallocated (0) or already initialized (1).
		// Starting, stopping or a short/stale recording
		// cannot grant permission. Retry transient mode-query failures at most every 5 s.
		lag := m.Record.Time - m.Record.Data
		if (secondary != 0 && secondary != 1) || p.singleProfile == nil {
			return nil
		}
		if !p.profileKnown && m.Record.Time >= p.nextCheck {
			p.nextCheck = m.Record.Time + 5000
			ok, err := p.singleProfile()
			if err != nil {
				return err
			}
			p.profileKnown, p.single = true, ok
			// Reading the configuration can take more than a second. The next tick
			// must reconfirm live streams and the recording clock before acting.
			return nil
		}
		// An RPC timeout can leave the first stop running on the native server.
		// Never queue another on this take. Wait for true proxy teardown (Init)
		// when its switch is enabled; "already stopped" during Stopping is insufficient.
		if p.attempts != 0 {
			return nil
		}
		if p.proxyPreference != nil {
			disabled, err := p.proxyPreference()
			if err != nil {
				return err
			}
			sub := m.Pipeline.Status.States[3]
			rawPortRunning := len(m.Pipeline.Status.States) > 19 && m.Pipeline.Status.States[19] == 3
			absent := p.single && rawPortRunning && (sub == 0 || sub == 6)
			if (disabled == 1 && sub != 1 && !absent) || (disabled == 0 && sub != 3) {
				return nil
			}
		}
		if !p.single || m.Record.Data < 5000 || lag < -2000 || lag > 2000 || p.progress < 2 {
			return nil
		}
	}
	if m.Record.Time < 5000 {
		return nil
	}
	audio := m.Pipeline.Status.States[8]
	if audio != 3 {
		lag := m.Record.Time - m.Record.Data
		if audio != 2 || m.Record.Data < 5000 || lag < -2000 || lag > 2000 || p.progress < 2 || p.ambisonic == nil || !p.ambisonic() {
			return nil
		}
		log.Printf("AMBISONIC_COMPAT audio=Starting data_ms=%d progress=%d", m.Record.Data, p.progress)
	}
	p.attempts++
	// Persist ownership before the stop, so a crash/restart can restore it.
	if !p.owned {
		if e := p.mark(1); e != nil {
			return e
		}
		p.owned = true
	}
	if e := p.call(false); e != nil {
		return e
	}
	log.Printf("PREVIEW_STOPPED record_ms=%d", m.Record.Time)
	return nil
}
func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	if os.Geteuid() != 0 {
		log.Fatal("root required")
	}
	if os.Getenv("X6_PREVIEW_TEMP") == "1" {
		base = "/run/x6-preview-menu"
	}
	if e := dir(base); e != nil {
		log.Fatal(e)
	}
	if e := os.Mkdir(runtime, 0700); e != nil && !os.IsExist(e) {
		log.Fatal(e)
	}
	if e := dir(runtime); e != nil {
		log.Fatal(e)
	}
	fd, e := syscall.Open(runtime+"/lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		log.Fatal(e)
	}
	defer syscall.Close(fd)
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		log.Fatal(e)
	}
	v, e := read(runtime + "/owned")
	if e != nil && !os.IsNotExist(e) {
		log.Fatal(e)
	}
	p := policy{singleProfile: recordingFreeFrame, proxyPreference: func() (byte, error) { return read("/data/x6-owner-features/proxy-v1/disabled") }, ambisonic: isAmbisonic, owned: v == 1, mark: func(v byte) error { return save(runtime+"/owned", v) }, call: stream}
	if len(os.Args) == 2 && os.Args[1] == "--restore" {
		if e := p.restore(); e != nil {
			log.Fatal(e)
		}
		return
	}
	enabled, e := read(base + "/disabled")
	if e != nil {
		log.Fatal(e)
	}
	if e = save(runtime+"/state", enabled); e != nil {
		log.Fatal(e)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(stop)
	stopFraming := startFraming(fishRoot, mediaSettings)
	defer stopFraming()
	fast := time.NewTicker(200 * time.Millisecond)
	defer fast.Stop()
	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	log.Printf("READY disabled=%d owned=%t", enabled, p.owned)
	for {
		select {
		case <-stop:
			if e := p.restore(); e != nil {
				log.Printf("RESTORE_FAILED %v", e)
				os.Exit(1)
			}
			return
		case <-fast.C:
			if _, e = os.Lstat(runtime + "/request"); os.IsNotExist(e) {
				continue
			}
			v, err := read(runtime + "/request")
			if err == nil {
				err = save(base+"/disabled", v)
			}
			if err == nil {
				enabled = v
				err = save(runtime+"/state", v)
			}
			if err != nil {
				log.Printf("REJECT preference: %v", err)
			} else {
				log.Printf("POLICY disabled=%d", v)
			}
			if err = os.Remove(runtime + "/request"); err != nil && !os.IsNotExist(err) {
				log.Printf("request cleanup: %v", err)
			}
		case <-poll.C:
			m, err := getMedia()
			if err != nil {
				log.Printf("media query: %v", err)
				continue
			}
			if err = p.step(enabled == 1, m); err != nil {
				log.Printf("POLICY_ERROR %v", err)
			}
		}
	}
}
