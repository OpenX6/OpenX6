// Fixed-purpose local feature manager. No listener or arbitrary command input.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const base = "/data/x6-owner-features/vr180-v1"
const runtime = "/run/x6-vr180"

type payload struct {
	Name, Target, Old, New, Label string
	Mode                          os.FileMode
}

func secureDir(p string) error {
	s, e := os.Lstat(p)
	if e != nil {
		return e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Getuid()) || !s.IsDir() || s.Mode()&0022 != 0 {
		return fmt.Errorf("unsafe directory %s", p)
	}
	return nil
}
func regular(p string, max int64) ([]byte, error) {
	if e := secureDir(filepath.Dir(p)); e != nil {
		return nil, e
	}
	fd, e := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return nil, e
	}
	st := s.Sys().(*syscall.Stat_t)
	if !s.Mode().IsRegular() || st.Uid != uint32(os.Getuid()) || s.Mode()&0022 != 0 || s.Size() > max {
		return nil, fmt.Errorf("unsafe file %s", p)
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}
func digest(p string) (string, error) {
	b, e := regular(p, 160<<20)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}
func atomic(p string, b []byte, mode os.FileMode) error {
	if e := secureDir(filepath.Dir(p)); e != nil {
		return e
	}
	if s, e := os.Lstat(p); e == nil && (!s.Mode().IsRegular() || s.Mode()&0022 != 0) {
		return fmt.Errorf("unsafe destination %s", p)
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".vr180-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, p); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(p))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func readState(p string) (byte, error) {
	b, e := regular(p, 2)
	if e != nil {
		return 0, e
	}
	if len(b) != 1 || b[0] > 1 {
		return 0, errors.New("invalid state")
	}
	return b[0], nil
}
func command(seconds int, name string, args ...string) ([]byte, error) {
	c, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	b, e := exec.CommandContext(c, name, args...).CombinedOutput()
	if e != nil {
		return b, fmt.Errorf("%s %v: %w: %.1000s", name, args, e, b)
	}
	return b, nil
}
func run(name string, args ...string) error { _, e := command(25, name, args...); return e }
func pid(unit string) (int, error) {
	b, e := command(4, "/bin/systemctl", "show", unit, "-p", "MainPID", "--value")
	if e != nil {
		return 0, e
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}
func platform() error {
 if os.Geteuid()!=0{return errors.New("root required")}
 b,e:=command(3,"/sbin/abctl","--boot_slot");if e!=nil{return e};slot:=strings.TrimSpace(string(b));if slot!="_a"&&slot!="_b"{return errors.New("unknown slot")}
 b,e=os.ReadFile("/proc/self/mountinfo");if e!=nil{return e};found:=false
 for _,line:=range strings.Split(string(b),"\n"){f:=strings.Fields(line);if len(f)>6&&f[4]=="/"{parts:=strings.Split(f[2],":");if len(parts)!=2{return errors.New("root device format")};for _,s:=range parts{if _,e:=strconv.ParseUint(s,10,32);e!=nil{return e}};u,e:=os.ReadFile("/sys/dev/block/"+f[2]+"/uevent");if e!=nil||!bytes.Contains(u,[]byte("PARTNAME=system"+slot+"\n")){return errors.New("root device/slot mismatch")};found=true}}
 if !found{return errors.New("root device absent")}
 for _,p:=range []string{"/run/x6-owner/vendor-ota-enabled","/run/x6-owner/validated-boot"}{if _,e:=os.Lstat(p);!os.IsNotExist(e){return errors.New("OTA hold absent")}}
 return nil
}
func mediaLock()(func(),error){
 fd,e:=syscall.Open("/run/x6-owner-media.lock",syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0600);if e!=nil{return nil,e};f:=os.NewFile(uintptr(fd),"media lock");st,e:=f.Stat();if e!=nil{f.Close();return nil,e};s:=st.Sys().(*syscall.Stat_t);if !st.Mode().IsRegular()||s.Uid!=0||st.Mode().Perm()!=0600{f.Close();return nil,errors.New("unsafe media lock")}
 until:=time.Now().Add(60*time.Second);for {e=syscall.Flock(fd,syscall.LOCK_EX|syscall.LOCK_NB);if e==nil{return func(){syscall.Flock(fd,syscall.LOCK_UN);f.Close()},nil};if e!=syscall.EWOULDBLOCK||time.Now().After(until){f.Close();return nil,errors.New("media lock busy")};time.Sleep(100*time.Millisecond)}
}
func bound(p payload) (bool, error) {
	h, e := digest(p.Target)
	if e != nil {
		return false, e
	}
	b, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return false, e
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 5 && f[4] == p.Target {
			s, e := os.Stat(runtime + "/" + p.Name)
			if e != nil {
				return false, e
			}
			t, e := os.Stat(p.Target)
			if e != nil {
				return false, e
			}
			if !os.SameFile(s, t) || h != p.New {
				return false, fmt.Errorf("unowned bind %s", p.Target)
			}
			return true, nil
		}
	}
	if h != p.Old {
		return false, fmt.Errorf("unknown base %s", p.Target)
	}
	return false, nil
}
func setBind(p payload, v bool) error {
	on, e := bound(p)
	if e != nil {
		return e
	}
	if on == v {
		return nil
	}
	if v {
		h, e := digest(runtime + "/" + p.Name)
		if e != nil || h != p.New {
			return errors.New("runtime payload mismatch")
		}
		return run("/bin/mount", "--bind", runtime+"/"+p.Name, p.Target)
	}
	return run("/bin/umount", p.Target)
}
func prepare() (err error) {
	if err = platform(); err != nil {
		return
	}
	if _, err = os.Lstat(base + "/enabled"); os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return
	}
	b, err := regular(base+"/enabled", 2)
	if err != nil || string(b) != "1" {
		return errors.New("bad enable marker")
	}
	v, err := readState(base + "/sync")
	if err != nil {
		return
	}
	g, err := pid("ins_gui")
	if err != nil || g != 0 {
		return errors.New("GUI must be stopped for prepare")
	}
	if v == 1 {
		for _, unit := range []string{"ins_mediad", "qmmf-server"} {
			n, e := pid(unit)
			if e != nil || n != 0 {
				return errors.New("boot sync prepare requires stopped media")
			}
		}
	}
	if err = os.Mkdir(runtime, 0700); err != nil && !os.IsExist(err) {
		return
	}
	if err = secureDir(runtime); err != nil {
		return
	}
	if _, e := os.Lstat(runtime + "/ready"); e == nil {
		return errors.New("already prepared; do not rebuild live mappings")
	}
	for _, p := range payloads {
		on, e := bound(p)
		if e != nil || on {
			return fmt.Errorf("prepare base check %s: %v", p.Target, e)
		}
		b, e := regular(base+"/"+p.Name, 160<<20)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != p.New {
			return errors.New("persistent payload mismatch")
		}
		if err = atomic(runtime+"/"+p.Name, b, p.Mode); err != nil {
			return
		}
		if err = run("/usr/sbin/setfilecon", "system_u:object_r:"+p.Label+":s0", runtime+"/"+p.Name); err != nil {
			return
		}
	}
	defer func() {
		if err != nil {
			for i := len(payloads) - 1; i >= 0; i-- {
				if e := setBind(payloads[i], false); e != nil {
					log.Print("prepare rollback: ", e)
				}
			}
		}
	}()
	if err = setBind(payloads[2], v == 1); err != nil {
		return
	}
	if err = setBind(payloads[0], true); err != nil {
		return
	}
	if err = setBind(payloads[1], true); err != nil {
		return
	}
	if err = atomic(runtime+"/state", []byte{v}, 0600); err != nil {
		return
	}
	if err = atomic(runtime+"/ready", []byte("1"), 0600); err != nil {
		return
	}
	log.Printf("PREPARED sync=%d", v)
	return nil
}
func query(q string) (map[string]interface{}, error) {
	b, e := command(6, "/usr/bin/ins_bll_cli", q)
	if e != nil {
		return nil, e
	}
	var d map[string]interface{}
	if e = json.Unmarshal(b, &d); e != nil {
		return nil, e
	}
	if d["ret"] != "ok" {
		return nil, errors.New("camera query rejected")
	}
	m, ok := d["data"].(map[string]interface{})
	if !ok {
		return nil, errors.New("missing query data")
	}
	return m, nil
}
func idle() error {
	d, e := query(`{"cmd":"share_state","action":"get","type":"media"}`)
	if e != nil {
		return e
	}
	for k, v := range map[string]string{"record": "RecordIdle", "capture": "CaptureIdle"} {
		m, ok := d[k].(map[string]interface{})
		if !ok || m[k+"_status"] != v {
			return errors.New("media busy")
		}
	}
	return nil
}
func modeAllowed(v byte) error {
	if e := idle(); e != nil {
		return e
	}
	if v == 0 {
		return nil
	}
	for _, t := range []struct {
		key  string
		want map[string]string
	}{{"kMediaMode", map[string]string{"lens_mode": "kPanorama", "shoot_mode": "kNormalPhoto"}}, {"kMediaResRatio", map[string]string{"media_res": "kRes16K"}}, {"kMediaPhotoCapMode", map[string]string{"cap_mode": "kNormal"}}} {
		d, e := query(`{"cmd":"config","action":"get_general","key":"` + t.key + `"}`)
		if e != nil {
			return e
		}
		for k, w := range t.want {
			if d[k] != w {
				return errors.New("unsupported mode")
			}
		}
	}
	return nil
}
func startMedia() error {
	if e := run("/bin/systemctl", "start", "qmmf-server.service", "ins_mediad.service"); e != nil {
		return e
	}
	if e := run("/bin/systemctl", "start", "pulseaudio.service"); e != nil {
		return e
	}
	for i := 0; i < 20; i++ {
		n, e := pid("qmmf-server")
		if e == nil && n > 1 {
			b, e := os.ReadFile(fmt.Sprintf("/proc/%d/maps", n))
			if e == nil && bytes.Contains(b, []byte("com.qti.chi.override")) && idle() == nil {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("media not ready")
}
func apply(v byte) error {
 release,lockerr:=mediaLock();if lockerr!=nil{return lockerr};defer release()

	if e := platform(); e != nil {
		return e
	}
	if v > 1 {
		return errors.New("invalid bool")
	}
	if e := run("/bin/systemctl", "is-active", "x6-owner-maintenance.service", "x6-owner-gui-auth.service"); e != nil {
		return e
	}
	p, e := pid("ins_otad")
	if e != nil || p != 0 {
		return errors.New("OTA not idle")
	}
	if e := idle(); e != nil {
		return e
	}
	old, e := bound(payloads[2])
	if e != nil {
		return e
	}
	if old == (v == 1) {
		return nil
	}
	if e = run("/bin/systemctl", "stop", "ins_mediad.service", "qmmf-server.service"); e != nil {
		_ = startMedia()
		return e
	}
	for _, unit := range []string{"ins_mediad", "qmmf-server"} {
		n, e := pid(unit)
		if e != nil || n != 0 {
			_ = startMedia()
			return errors.New("media did not stop")
		}
	}
	if e = setBind(payloads[2], v == 1); e != nil {
		_ = startMedia()
		return e
	}
	if e = startMedia(); e != nil {
		_ = run("/bin/systemctl", "stop", "ins_mediad.service", "qmmf-server.service")
		_ = setBind(payloads[2], old)
		_ = startMedia()
		return e
	}
	return nil
}

type engine struct {
	disk, live string
	guard      func(byte) error
	apply      func(byte) error
	save       func(string, []byte, os.FileMode) error
}

func (e engine) set(v byte) error {
	if v > 1 {
		return errors.New("invalid bool")
	}
	old, err := readState(e.live)
	if err != nil {
		return err
	}
	if old == v {
		return nil
	}
	if err = e.guard(v); err != nil {
		return err
	}
	if err = e.apply(v); err != nil {
		return err
	}
	if err = e.save(e.disk, []byte{v}, 0600); err != nil {
		r := e.apply(old)
		d := e.save(e.disk, []byte{old}, 0600)
		return fmt.Errorf("save rejected, rollback=%v disk=%v: %w", r, d, err)
	}
	if err = e.save(e.live, []byte{v}, 0600); err != nil {
		r := e.apply(old)
		d := e.save(e.disk, []byte{old}, 0600)
		_ = e.save(e.live, []byte{old}, 0600)
		return fmt.Errorf("live commit failed; rollback=%v disk=%v: %w", r, d, err)
	}
	return nil
}
func serve() error {
	if e := platform(); e != nil {
		return e
	}
	if b, e := regular(runtime+"/ready", 2); e != nil || string(b) != "1" {
		return errors.New("not prepared")
	}
	lock, e := os.OpenFile(runtime+"/lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return e
	}
	eng := engine{base + "/sync", runtime + "/state", modeAllowed, apply, atomic}
	log.Print("CONTROLLER_READY")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		if _, e := os.Lstat(runtime + "/request"); e == nil {
			v, e := readState(runtime + "/request")
			if e == nil {
				e = eng.set(v)
			}
			if e != nil {
				log.Print("REQUEST_REFUSED: ", e)
			} else {
				log.Printf("SYNC_COMMITTED=%d", v)
			}
			if e = os.Remove(runtime + "/request"); e != nil {
				return e
			}
		}
		select {
		case <-stop:
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}
func restore() error {
	if e := platform(); e != nil {
		return e
	}
	if e := idle(); e != nil {
		return e
	}
	if e := run("/bin/systemctl", "stop", "x6-vr180-control.service"); e != nil {
		return e
	}
	if e := apply(0); e != nil {
		return e
	}
	if e := run("/bin/systemctl", "stop", "ins_gui.service"); e != nil {
		return e
	}
	defer func() { _ = run("/bin/systemctl", "start", "ins_gui.service") }()
	for _, p := range payloads[:2] {
		if e := setBind(p, false); e != nil {
			return e
		}
	}
	if e := os.Remove(base + "/enabled"); e != nil && !os.IsNotExist(e) {
		return e
	}
	if e := os.Remove(runtime + "/ready"); e != nil && !os.IsNotExist(e) {
		return e
	}
	return run("/bin/systemctl", "start", "ins_gui.service")
}
func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if len(os.Args) != 2 {
		log.Fatal("prepare | serve | restore")
	}
	var e error
	switch os.Args[1] {
	case "prepare":
		e = prepare()
	case "serve":
		e = serve()
	case "restore":
		e = restore()
	default:
		e = errors.New("unknown mode")
	}
	if e != nil {
		log.Fatal(e)
	}
}
