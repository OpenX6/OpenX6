package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const runtime = "/run/x6-raw-nr-menu"
const base = "/data/x6-owner-features/raw-nr-v1"
const target = "/usr/lib/libbva_camera.so"

var hashes = []string{"866700ee31b4f23d21d1d8591bb38c9845069ab97b91e1fa32eb8c8345ba1618", "50efa43512fbbd97ec698043daa731dc20aa18e42ea507c94d8a28bd2017241a", "9122b444e77c5087daef7aa32dd706b2216689ee594e2fdd2a06cdffa8b67175", "ee80f0e9094b5f3f823473f11b8e4d7f8c6661439a28d5a613fd14d1ee728a68"}
var names = []string{"native", "medium", "low", "minimal"}

func query(cmd string) (map[string]interface{}, error) {
	b, e := command(8, "/usr/bin/ins_bll_cli", cmd)
	if e != nil {
		return nil, e
	}
	var d struct {
		Ret  string                 `json:"ret"`
		Data map[string]interface{} `json:"data"`
	}
	if e = json.Unmarshal(b, &d); e != nil {
		return nil, e
	}
	if d.Ret != "ok" {
		return nil, errors.New("query rejected")
	}
	return d.Data, nil
}
func idle() error {
	d, e := query(`{"cmd":"share_state","action":"get","type":"media"}`)
	if e != nil {
		return e
	}
	r, ok := d["record"].(map[string]interface{})
	if !ok || r["record_status"] != "RecordIdle" {
		return errors.New("record busy")
	}
	c, ok := d["capture"].(map[string]interface{})
	if !ok || c["capture_status"] != "CaptureIdle" {
		return errors.New("capture busy")
	}
	return nil
}
func guard() error {
	if e := platform(); e != nil {
		return e
	}
	for _, p := range []string{"/run/x6-owner/vendor-ota-enabled", "/run/x6-owner/validated-boot", "/run/x6-vr180/request"} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			return errors.New("OTA unlocked or sync request pending")
		}
	}
	d, e := query(`{"cmd":"share_state","action":"get","type":"otad"}`)
	if e != nil || d["update_status"] != "Idle" {
		return errors.New("OTA not idle")
	}
	return idle()
}
func current() (byte, error) {
	h, e := digest(target)
	if e != nil {
		return 0, e
	}
	b, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return 0, e
	}
	mounted := false
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 5 && f[4] == target {
			mounted = true
		}
	}
	for i, v := range hashes {
		if h != v {
			continue
		}
		if i == 0 && !mounted {
			return 0, nil
		}
		if i == 0 {
			return 0, errors.New("unexpected native mount")
		}
		s, e := os.Stat(filepath.Join(base, names[i]+".so"))
		if e != nil {
			return 0, e
		}
		t, e := os.Stat(target)
		if e != nil || !mounted || !os.SameFile(s, t) {
			return 0, errors.New("unowned mount")
		}
		return byte(i), nil
	}
	return 0, errors.New("unknown library")
}
func install(v byte) error {
	if v > 3 {
		return errors.New("invalid enum")
	}
	old, e := current()
	if e != nil {
		return e
	}
	if old == v {
		return nil
	}
	if v != 0 {
		h, e := digest(base + "/" + names[v] + ".so")
		if e != nil || h != hashes[v] {
			return errors.New("payload hash mismatch")
		}
	}
	if old != 0 {
		if e = run("/bin/umount", target); e != nil {
			return e
		}
	}
	if v != 0 {
		if e = run("/bin/mount", "--bind", base+"/"+names[v]+".so", target); e != nil {
			return e
		}
		if e = run("/bin/mount", "-o", "remount,bind,ro", target); e != nil {
			return e
		}
	}
	got, e := current()
	if e != nil || got != v {
		return errors.New("bind verification failed")
	}
	return nil
}
func services() error {
	return run("/bin/systemctl", "start", "qmmf-server", "ins_mediad", "pulseaudio")
}
func ready() error {
	until := time.Now().Add(30 * time.Second)
	for time.Now().Before(until) {
		b, e := command(4, "/bin/systemctl", "is-active", "qmmf-server", "ins_mediad", "pulseaudio")
		if e == nil && bytes.Equal(bytes.TrimSpace(b), []byte("active\nactive\nactive")) && idle() == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("media/audio not ready")
}
func apply(v byte) (err error) {
	if v > 3 {
		return errors.New("invalid enum")
	}
	release, e := mediaLock()
	if e != nil {
		return e
	}
	defer release()
	if err = guard(); err != nil {
		return
	}
	old, e := current()
	if e != nil {
		return e
	}
	if v == old {
		return commitSelection(v, old, atomic)
	}
	// This is a persistent photo preference, selectable even before the first
	// photo-mode initialization. The current mode's format is not an eligibility
	// check: video can return no RAW format. Still verify the library, restart
	// media/audio safely, and commit only after readiness and protected-PID checks.
	units := []string{"x6-owner-maintenance", "x6-owner-gui-auth", "ins_gui"}
	ids := make([]int, len(units))
	for i, u := range units {
		ids[i], e = pid(u)
		if e != nil || ids[i] == 0 {
			return errors.New("protected service absent")
		}
	}
	// Validate source before stopping anything. No user-controlled paths or commands.
	if v > 0 {
		h, e := digest(base + "/" + names[v] + ".so")
		if e != nil || h != hashes[v] {
			return errors.New("payload mismatch")
		}
	}
	if err = idle(); err != nil {
		return
	}
	defer func() {
		if err != nil {
			log.Printf("apply failed, restore %d: %v", old, err)
			_ = run("/bin/systemctl", "stop", "pulseaudio", "ins_mediad", "qmmf-server")
			if e := install(old); e != nil {
				log.Printf("restore library failed: %v", e)
			}
			_ = services()
			if e := ready(); e != nil {
				log.Printf("restore readiness failed: %v", e)
			}
		}
	}()
	if err = run("/bin/systemctl", "stop", "pulseaudio", "ins_mediad", "qmmf-server"); err != nil {
		return
	}
	if err = install(v); err != nil {
		return
	}
	if err = services(); err != nil {
		return
	}
	if err = ready(); err != nil {
		return
	}
	for i, u := range units {
		p, e := pid(u)
		if e != nil || p != ids[i] {
			return fmt.Errorf("protected PID changed: %s", u)
		}
	}
	p, e := pid("ins_mediad")
	if e != nil {
		return e
	}
	maps, e := os.ReadFile("/proc/" + strconv.Itoa(p) + "/maps")
	if e != nil || !bytes.Contains(maps, []byte(target)) {
		return errors.New("library not mapped")
	}
	return commitSelection(v, old, atomic)
}
func serve() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if e := secureDir(runtime); e != nil {
		log.Fatal(e)
	}
	if e := secureDir(base); e != nil {
		log.Fatal(e)
	}
	until := time.Now().Add(45 * time.Second)
	for {
		if e := guard(); e == nil {
			break
		} else if time.Now().After(until) {
			log.Fatal(e)
		}
		time.Sleep(time.Second)
	}
	d, e := query(`{"cmd":"share_state","action":"get","type":"bll"}`)
	if e != nil {
		log.Fatal(e)
	}
	s, ok := d["host_serial_number"].(string)
	h := sha256.Sum256([]byte(s))
	bound, be := regular("/data/x6-firstboot-state/device.sha256", 66)
	if !ok || be != nil || !bindingMatches(bound, hex.EncodeToString(h[:])) {
		log.Fatal("device binding mismatch")
	}
	v, e := current()
	if e != nil {
		log.Fatal(e)
	}
	saved, e := readState(runtime + "/state")
	if e != nil || saved != v {
		log.Fatal("state does not match runtime")
	}
	f, e := os.OpenFile(runtime+"/lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		log.Fatal(e)
	}
	defer f.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(stop)
	for {
		select {
		case <-stop:
			return
		default:
		}
		if _, e = os.Lstat(runtime + "/request"); os.IsNotExist(e) {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
			log.Fatal(e)
		}
		v, e := readState(runtime + "/request")
		if e == nil {
			e = apply(v)
		}
		if e != nil {
			log.Printf("REJECT: %v", e)
		} else {
			log.Printf("COMMITTED %d %s", v, names[v])
		}
		_ = os.Remove(runtime + "/request")
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		time.Sleep(250 * time.Millisecond)
	}
}

func platform() error {
	if os.Geteuid() != 0 {
		return errors.New("root required")
	}
	b, e := command(3, "/sbin/abctl", "--boot_slot")
	if e != nil {
		return e
	}
	slot := strings.TrimSpace(string(b))
	if slot != "_a" && slot != "_b" {
		return errors.New("unknown slot")
	}
	b, e = os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return e
	}
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 6 && f[4] == "/" {
			parts := strings.Split(f[2], ":")
			if len(parts) != 2 {
				return errors.New("root device format")
			}
			for _, s := range parts {
				if _, e := strconv.ParseUint(s, 10, 32); e != nil {
					return e
				}
			}
			u, e := os.ReadFile("/sys/dev/block/" + f[2] + "/uevent")
			if e != nil || !bytes.Contains(u, []byte("PARTNAME=system"+slot+"\n")) {
				return errors.New("root device/slot mismatch")
			}
			found = true
		}
	}
	if !found {
		return errors.New("root device absent")
	}
	for _, p := range []string{"/run/x6-owner/vendor-ota-enabled", "/run/x6-owner/validated-boot"} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			return errors.New("OTA hold absent")
		}
	}
	return nil
}
func mediaLock() (func(), error) {
	fd, e := syscall.Open("/run/x6-owner-media.lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "media lock")
	st, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	s := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || s.Uid != 0 || st.Mode().Perm() != 0600 {
		f.Close()
		return nil, errors.New("unsafe media lock")
	}
	until := time.Now().Add(60 * time.Second)
	for {
		e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			return func() { syscall.Flock(fd, syscall.LOCK_UN); f.Close() }, nil
		}
		if e != syscall.EWOULDBLOCK || time.Now().After(until) {
			f.Close()
			return nil, errors.New("media lock busy")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Commit persistent preference only after actual media/audio readiness. Roll back
// both copies if the live-state commit fails, then apply() restores the old library.
func commitSelection(v, old byte, save func(string, []byte, os.FileMode) error) error {
	if v > 3 || old > 3 {
		return errors.New("invalid preference")
	}
	if e := save(base+"/level", []byte{v}, 0600); e != nil {
		d := save(base+"/level", []byte{old}, 0600)
		r := save(runtime+"/state", []byte{old}, 0600)
		return fmt.Errorf("disk commit failed, restore disk=%v live=%v: %w", d, r, e)
	}
	if e := save(runtime+"/state", []byte{v}, 0600); e != nil {
		d := save(base+"/level", []byte{old}, 0600)
		r := save(runtime+"/state", []byte{old}, 0600)
		return fmt.Errorf("state commit failed, restore disk=%v live=%v: %w", d, r, e)
	}
	return nil
}
func prepare() (err error) {
	if err = platform(); err != nil {
		return
	}
	if err = secureDir(base); err != nil {
		return
	}
	flag, err := regular(base+"/enabled", 2)
	if err != nil || string(flag) != "1" {
		return errors.New("disabled or invalid marker")
	}
	selected, err := readState(base + "/level")
	if err != nil {
		return
	}
	for _, u := range []string{"ins_mediad", "qmmf-server"} {
		p, e := pid(u)
		if e != nil || p != 0 {
			return errors.New("camera and media must be stopped for preparation")
		}
	}
	for i := 1; i < 4; i++ {
		h, e := digest(base + "/" + names[i] + ".so")
		if e != nil || h != hashes[i] {
			return errors.New("persistent payload mismatch")
		}
	}
	if err = os.Mkdir(runtime, 0700); err != nil && !os.IsExist(err) {
		return
	}
	if err = secureDir(runtime); err != nil {
		return
	}
	if _, e := os.Lstat(runtime + "/ready"); e == nil {
		return errors.New("already prepared")
	}
	v, err := current()
	if err != nil || v != 0 {
		return errors.New("unknown initial library")
	}
	defer func() {
		if err != nil {
			_ = install(0)
			_ = atomic(runtime+"/state", []byte{0}, 0600)
		}
	}()
	if err = install(selected); err != nil {
		return
	}
	if err = atomic(runtime+"/state", []byte{selected}, 0600); err != nil {
		return
	}
	if err = atomic(runtime+"/ready", []byte("1"), 0600); err != nil {
		return
	}
	log.Printf("PREPARED %d %s", selected, names[selected])
	return nil
}
func main() {
	if len(os.Args) != 2 {
		log.Fatal("prepare or serve required")
	}
	switch os.Args[1] {
	case "prepare":
		if e := prepare(); e != nil {
			log.Fatal(e)
		}
	case "serve":
		if b, e := regular(runtime+"/ready", 2); e != nil || string(b) != "1" {
			log.Fatal("not prepared")
		}
		serve()
	default:
		log.Fatal("unknown action")
	}
}
