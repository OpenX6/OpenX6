// Compact source-built reconstruction of verified media variants, before media starts.
// No network, OTA, slot writes, signing keys or device identities.
package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed manifest.json
var manifest []byte

type Edit struct {
	Offset int
	Before string
	Data   []byte
}
type Variant struct {
	Base, Runtime, BaseSHA, SHA string
	Size                        int
	Edits                       []Edit
}
type Pref struct {
	Path, State  string
	Max, Default byte
}

var preferences = []Pref{
	{"/data/x6-owner-features/mod-controls-v1/stereo.pref", "/run/x6-stereo-control/state", 1, 0},
	{"/data/x6-owner-features/mod-controls-v1/hnr.pref", "/run/x6-hnr-control/state", 1, 0},
	{"/data/x6-owner-features/mod-controls-v1/vnr.pref", "/run/x6-video-nr-control/state", 3, 0},
	{"/data/x6-owner-features/proxy-v1/disabled", "/run/x6-proxy-control/state", 1, 1},
	{"/data/x6-owner-features/video-iq-v1/sharp", "/run/x6-video-iq/sharp", 4, 0},
	{"/data/x6-owner-features/video-iq-v1/noise", "/run/x6-video-iq/noise", 3, 0},
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func reconstruct(b []byte, v Variant) ([]byte, error) {
	if sum(b) != v.BaseSHA || v.Size <= 0 || v.Size > 160<<20 {
		return nil, fmt.Errorf("base/size rejected")
	}
	out := make([]byte, v.Size)
	copy(out, b)
	end := 0
	for _, e := range v.Edits {
		n := len(e.Data)
		if e.Offset < end || n == 0 || e.Offset > len(out)-n {
			return nil, fmt.Errorf("edit bounds rejected")
		}
		originalEnd := e.Offset + n
		if originalEnd > len(b) {
			originalEnd = len(b)
		}
		originalStart := e.Offset
		if originalStart > len(b) {
			originalStart = len(b)
		}
		if sum(b[originalStart:originalEnd]) != e.Before {
			return nil, fmt.Errorf("edit original rejected")
		}
		copy(out[e.Offset:], e.Data)
		end = e.Offset + n
	}
	if sum(out) != v.SHA {
		return nil, fmt.Errorf("output hash rejected")
	}
	return out, nil
}
func safeDir(p string) error {
	s, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !s.IsDir() || s.Mode()&0022 != 0 || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("unsafe directory: %s", p)
	}
	return nil
}
func dirs(p string) error {
	if p == "/" || p == "." {
		return safeDir(p)
	}
	if e := dirs(filepath.Dir(p)); e != nil {
		return e
	}
	if e := os.Mkdir(p, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	return safeDir(p)
}
func read(p string, max int) ([]byte, error) {
	if e := safeDir(filepath.Dir(p)); e != nil {
		return nil, e
	}
	fd, e := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	ss := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || st.Mode()&0022 != 0 || ss.Uid != uint32(os.Geteuid()) || ss.Nlink != 1 || st.Size() > int64(max) {
		return nil, fmt.Errorf("unsafe file %s", p)
	}
	b := make([]byte, st.Size())
	n, e := f.ReadAt(b, 0)
	if e != nil || n != len(b) {
		return nil, fmt.Errorf("short read %s", p)
	}
	return b, nil
}
func atomic(p string, b []byte, mode os.FileMode, label bool) error {
	if e := safeDir(filepath.Dir(p)); e != nil {
		return e
	}
	if s, e := os.Lstat(p); e == nil {
		if !s.Mode().IsRegular() || s.Mode()&0022 != 0 || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
			return fmt.Errorf("unsafe destination")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".mod-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(mode); e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if label {
		if e = cmd("/usr/sbin/setfilecon", "system_u:object_r:bin_t:s0", f.Name()); e != nil {
			return e
		}
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
func dirsTo(root, p string) error {
	if p == root {
		return safeDir(p)
	}
	rel, e := filepath.Rel(root, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("directory boundary")
	}
	if e = dirsTo(root, filepath.Dir(p)); e != nil {
		return e
	}
	if e = os.Mkdir(p, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	return safeDir(p)
}
func preference(root string, p Pref) (byte, error) {
	path := filepath.Join(root, p.Path)
	if e := dirsTo(root, filepath.Dir(path)); e != nil {
		return 0, e
	}
	b, e := read(path, 2)
	if os.IsNotExist(e) {
		b = []byte{p.Default}
		e = atomic(path, b, 0600, false)
	}
	if e != nil {
		return 0, e
	}
	if len(b) != 1 || b[0] > p.Max {
		return 0, fmt.Errorf("invalid preference %s", p.Path)
	}
	// These video controls were retired: any saved choice is interpreted as
	// native (0). The saved byte itself is left alone, so the release in the
	// other slot still reads the owner's own value after a fallback; shared
	// preferences are never normalised at boot. The byte range check above
	// still fails closed on corruption.
	if (p.Path == preferences[1].Path || p.Path == preferences[2].Path) && b[0] != 0 {
		return 0, nil
	}
	return b[0], nil
}
func cmd(n string, a ...string) error {
	b, e := exec.Command(n, a...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("%s: %w %.600s", n, e, b)
	}
	return nil
}
func platform() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root required")
	}
	b, e := exec.Command("/sbin/abctl", "--boot_slot").Output()
	if e != nil {
		return e
	}
	slot := strings.TrimSpace(string(b))
	if slot != "_a" && slot != "_b" {
		return fmt.Errorf("unknown slot")
	}
	b, e = os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return e
	}
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 6 && f[4] == "/" {
			u, e := os.ReadFile("/sys/dev/block/" + f[2] + "/uevent")
			if e != nil || !bytes.Contains(u, []byte("PARTNAME=system"+slot+"\n")) {
				return fmt.Errorf("root/slot mismatch")
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("root absent")
	}
	for _, p := range []string{"/run/x6-owner/vendor-ota-enabled", "/run/x6-owner/validated-boot"} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			return fmt.Errorf("hold absent")
		}
	}
	for _, unit := range []string{"qmmf-server", "ins_mediad", "ins_gui"} {
		b, e := exec.Command("/bin/systemctl", "show", unit, "-p", "MainPID", "--value").Output()
		if e != nil || strings.TrimSpace(string(b)) != "0" {
			return fmt.Errorf("media already running")
		}
	}
	return nil
}
func prepareAt(root string, run func(string, ...string) error, check func() error, labels bool) (err error) {
	path := func(p string) string { return filepath.Join(root, p) }
	mk := func(p string) error { return dirsTo(root, path(p)) }
	if err = check(); err != nil {
		return
	}
	if err = mk("/run/x6-mod-controls"); err != nil {
		return
	}
	if _, e := os.Lstat(path("/run/x6-mod-controls/ready")); !os.IsNotExist(e) {
		return fmt.Errorf("already prepared/invalid ready")
	}
	fd, e := syscall.Open(path("/run/x6-owner-media.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	defer syscall.Close(fd)
	if err = syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return
	}
	var variants []Variant
	if err = json.Unmarshal(manifest, &variants); err != nil {
		return
	}
	if len(variants) != 9 {
		return fmt.Errorf("variant set")
	}
	// Immutable embedded manifest is fully verified before any persistent default is written.
	for _, v := range variants {
		if !strings.HasPrefix(v.Base, "/usr/lib/") || !strings.HasPrefix(v.Runtime, "/run/x6-") || filepath.Clean(v.Runtime) != v.Runtime {
			return fmt.Errorf("variant path")
		}
		b, e := read(path(v.Base), 160<<20)
		if e != nil {
			return e
		}
		out, e := reconstruct(b, v)
		if e != nil {
			return e
		}
		if e = mk(filepath.Dir(v.Runtime)); e != nil {
			return e
		}
		if e = atomic(path(v.Runtime), out, 0755, labels); e != nil {
			return e
		}
	}
	values := map[string]byte{}
	for _, p := range preferences {
		v, e := preference(root, p)
		if e != nil {
			return e
		}
		values[p.Path] = v
		if e = mk(filepath.Dir(p.State)); e != nil {
			return e
		}
		if e = atomic(path(p.State), []byte{v}, 0600, false); e != nil {
			return e
		}
	}
	mounted := []string{}
	defer func() {
		if err != nil {
			for i := len(mounted) - 1; i >= 0; i-- {
				_ = run("/bin/umount", path(mounted[i]))
			}
			_ = os.Remove(path("/run/x6-mod-controls/ready"))
			for _, p := range preferences {
				_ = atomic(path(p.State), []byte{0}, 0600, false)
			}
		}
	}()
	for _, v := range variants {
		selected := false
		switch v.Base {
		case "/usr/lib/hw/camera.qcom.so":
			name := "native.so"
			if values[preferences[1].Path] == 1 {
				name = "off.so"
			}
			selected = filepath.Base(v.Runtime) == name
		case "/usr/lib/libinsiqsetting.so":
			name := []string{"native.so", "medium.so", "low.so", "minimal.so"}[values[preferences[2].Path]]
			selected = filepath.Base(v.Runtime) == name
		default:
			selected = values[preferences[0].Path] == 1
		}
		if selected {
			if err = run("/bin/mount", "--bind", path(v.Runtime), path(v.Base)); err != nil {
				return
			}
			mounted = append(mounted, v.Base)
			if err = run("/bin/mount", "-o", "remount,bind,ro", path(v.Base)); err != nil {
				return
			}
		}
	}
	return atomic(path("/run/x6-mod-controls/ready"), []byte("1"), 0600, false)
}
func prepare() error { return prepareAt("/", cmd, platform, true) }

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "no arguments accepted")
		os.Exit(64)
	}
	if e := prepare(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
