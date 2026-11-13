package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"openx6.local/components/holder"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const gadget = "/sys/kernel/config/usb_gadget/g1"
const runtimeDir = "/run/x6-owner"
const adbPub = holder.Dir + "/" + holder.ADB

func readOwned(path string, secret bool) ([]byte, error) {
	s, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !s.Mode().IsRegular() || s.Mode().Perm()&0022 != 0 || (secret && s.Mode().Perm()&0077 != 0) {
		return nil, fmt.Errorf("unsafe file: %s", path)
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("unexpected file owner: %s", path)
	}
	return os.ReadFile(path)
}
func privateRunDir() error {
	if e := os.Mkdir(runtimeDir, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	s, e := os.Lstat(runtimeDir)
	if e != nil {
		return e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !s.IsDir() || s.Mode().Perm()&0077 != 0 || !ok || st.Uid != 0 {
		return fmt.Errorf("unsafe runtime directory")
	}
	return nil
}
func verifyCandidate() error {
	b, e := readOwned("/ins/etc/x6_owner_patch_manifest.json", false)
	if e != nil {
		return e
	}
	var m struct {
		Files map[string]string `json:"files"`
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	required := []string{"/sbin/x6-stop", "/sbin/adbd", "/usr/lib/libadbd.so.0.0.0", "/usr/lib/libins_factory_api.so", "/usr/bin/ins_hald", "/usr/bin/ins_usb", "/usr/local/sbin/x6-owner-usb", "/sbin/start_usb", "/lib/systemd/system/usb.service.d/x6-owner-init.conf"}
	for _, p := range required {
		if len(m.Files[p]) != 64 {
			return fmt.Errorf("manifest lacks %s", p)
		}
	}
	for p, want := range m.Files {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return fmt.Errorf("invalid manifest path")
		}
		data, e := readOwned(p, false)
		if e != nil {
			return e
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			return fmt.Errorf("candidate mismatch: %s", p)
		}
	}
	return nil
}

func (r *realUSB) StopInsUSB() error { return stopInsUSB() }
func checkADBKey(b []byte) error {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		return fmt.Errorf("exactly one ADB public key required")
	}
	words := strings.Fields(lines[0])
	if len(words) < 1 || len(words) > 2 {
		return fmt.Errorf("invalid ADB key line")
	}
	raw, e := base64.StdEncoding.DecodeString(words[0])
	if e != nil || len(raw) != 524 {
		return fmt.Errorf("invalid ADB RSA public key encoding")
	}
	if binary.LittleEndian.Uint32(raw[:4]) != 64 || binary.LittleEndian.Uint32(raw[520:]) != 65537 {
		return fmt.Errorf("require 2048-bit ADB RSA key, exponent 65537")
	}
	littleInt := func(b []byte) *big.Int {
		r := append([]byte(nil), b...)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return new(big.Int).SetBytes(r)
	}
	n := littleInt(raw[8:264])
	if n.BitLen() != 2048 || n.Bit(0) != 1 {
		return fmt.Errorf("invalid RSA modulus")
	}
	n0 := binary.LittleEndian.Uint32(raw[8:12])
	inv := binary.LittleEndian.Uint32(raw[4:8])
	if n0*inv != 0xffffffff {
		return fmt.Errorf("invalid ADB n0inv")
	}
	rr := new(big.Int).Lsh(big.NewInt(1), 4096)
	rr.Mod(rr, n)
	if rr.Cmp(littleInt(raw[264:520])) != 0 {
		return fmt.Errorf("invalid ADB Montgomery RR")
	}
	return nil
}
func runADBD() error {
	if e := verifyCandidate(); e != nil {
		return e
	}
	b, e := holder.Read("/", holder.ADB)
	if e != nil {
		return e
	}
	if e = checkADBKey(b); e != nil {
		return e
	}
	if e = privateRunDir(); e != nil {
		return e
	}
	// adbd is patched to read this root-controlled /run location. Atomic rename
	// cannot follow an existing leaf symlink; /run must be root-controlled.
	f, e := os.CreateTemp(runtimeDir, "adb.root-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.WriteString("#ROOT###")
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
	if e = os.Rename(f.Name(), "/run/.adb.root"); e != nil {
		return e
	}
	return syscall.Exec("/sbin/adbd", []string{"/sbin/adbd"}, []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "TERM=linux"})
}

type realUSB struct{ lock *os.File }

func (o *realUSB) Preflight(mode string) error {
	if e := verifyCandidate(); e != nil {
		return e
	}
	b, e := holder.Read("/", holder.ADB)
	if e != nil {
		return e
	}
	if e = checkADBKey(b); e != nil {
		return e
	}
	b, e = holder.Read("/", holder.Factory)
	if e != nil || len(b) != 32 {
		return fmt.Errorf("missing/invalid factory owner key: %v", e)
	}
	// Initialization must have completed. In particular, never synchronously
	// start adbd from usb.service's own initializing composition call.
	if e = systemctl("is-active", "--quiet", "usb.service"); e != nil {
		return fmt.Errorf("USB initialization incomplete: %w", e)
	}
	for _, p := range []string{gadget + "/UDC", gadget + "/functions/ffs.adb", "/dev/usb-ffs/adb/ep0"} {
		if _, e = os.Stat(p); e != nil {
			return e
		}
	}
	for _, p := range []string{"/tmp/usb_bind_in_progress", "/data/usb_bind_in_progress"} {
		if _, e = os.Lstat(p); e == nil {
			return fmt.Errorf("USB transition marker already present: %s", p)
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
func (o *realUSB) Lock() error {
	if e := privateRunDir(); e != nil {
		return e
	}
	f, e := os.OpenFile(runtimeDir+"/usb.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return e
	}
	o.lock = f
	return nil
}
func (o *realUSB) Unlock() {
	if o.lock != nil {
		syscall.Flock(int(o.lock.Fd()), syscall.LOCK_UN)
		o.lock.Close()
	}
}
func (o *realUSB) Mark() (err error) {
	made := []string{}
	defer func() {
		if err != nil {
			for _, p := range made {
				err = errors.Join(err, os.Remove(p))
			}
		}
	}()
	for _, p := range []string{"/tmp/usb_bind_in_progress", "/data/usb_bind_in_progress"} {
		f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		made = append(made, p)
		_, e = f.WriteString("x6-owner-usb\n")
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return e
		}
	}
	return nil
}
func (o *realUSB) Unmark() error {
	for _, p := range []string{"/data/usb_bind_in_progress", "/tmp/usb_bind_in_progress"} {
		if e := os.Remove(p); e != nil {
			return e
		}
	}
	return nil
}
func writeSys(path, value string) error { return os.WriteFile(path, []byte(value), 0600) }
func systemctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "/bin/systemctl", args...)
	out, e := c.CombinedOutput()
	if e != nil {
		return fmt.Errorf("systemctl %v: %w: %s", args, e, out)
	}
	return nil
}
func (o *realUSB) Unbind() error  { return writeSys(gadget+"/UDC", "\n") }
func (o *realUSB) StopADB() error { return systemctl("stop", "adbd.service") }
func (o *realUSB) Configure(mode string) error {
	// Drop only function links in known gadget configurations. Reject unexpected
	// extra configs rather than recursively deleting device configuration data.
	entries, e := os.ReadDir(gadget + "/configs")
	if e != nil {
		return e
	}
	for _, d := range entries {
		if d.Name() != "c.1" {
			return fmt.Errorf("extra USB configuration requires audit: %s", d.Name())
		}
	}
	for _, dir := range []string{gadget + "/os_desc", gadget + "/configs/c.1"} {
		entries, e := os.ReadDir(dir)
		if e != nil {
			return e
		}
		for _, d := range entries {
			if d.Type()&os.ModeSymlink != 0 {
				if e = os.Remove(dir + "/" + d.Name()); e != nil {
					return e
				}
			}
		}
	}
	vendor, product, description := "0x05c6", "0x901d", "OWNER_ADB"
	funcs := []string{"ffs.adb"} // No unauthenticated DIAG interface in owner mode.
	if mode == "C001" {
		vendor, product, description = "0x2e1a", "0xc001", "OWNER_SIMPLE_ADB"
		funcs = []string{"ins_simple.0", "ffs.adb"}
		// ins_simple.0 is created by usb.service and has no strings group.
		// Use the supported configuration string below, not a function child.
	}
	for _, pair := range [][2]string{{"idVendor", vendor}, {"idProduct", product}, {"configs/c.1/strings/0x409/configuration", description}, {"configs/c.1/bmAttributes", "0x80"}} {
		if e = writeSys(gadget+"/"+pair[0], pair[1]); e != nil {
			return e
		}
	}
	for i, f := range funcs {
		if e = os.Symlink(gadget+"/functions/"+f, fmt.Sprintf("%s/configs/c.1/f%d", gadget, i+1)); e != nil {
			return e
		}
	}
	return nil
}
func (o *realUSB) StartAuth() error { return systemctl("start", "x6-owner-auth.service") }
func (o *realUSB) InvalidateAuth() error {
	// Restart clears all challenges synchronously, including packets racing a
	// previous USB composition. Requires no dependency on ins_factoryd/GUI.
	return systemctl("restart", "x6-owner-auth.service")
}
func (o *realUSB) StartADB() error { return systemctl("start", "adbd.service") }
func (o *realUSB) WaitReady() error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_, e1 := os.Stat("/dev/usb-ffs/adb/ep1")
		_, e2 := os.Stat("/dev/usb-ffs/adb/ep2")
		ready, e := os.ReadFile(gadget + "/functions/ffs.adb/ready")
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		ffs := e != nil || bytes.Equal(bytes.TrimSpace(ready), []byte("1"))
		c, ce := net.DialTimeout("unix", authSocket, 100*time.Millisecond)
		if ce == nil {
			c.Close()
		}
		if e1 == nil && e2 == nil && ffs && ce == nil && systemctl("is-active", "--quiet", "adbd.service") == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("ADB FunctionFS/auth broker not ready; UDC remains unbound")
}
func (o *realUSB) Bind() error {
	names, e := os.ReadDir("/sys/class/udc")
	if e != nil {
		return e
	}
	list := []string{}
	for _, n := range names {
		if n.Name() != "dummy_udc" {
			list = append(list, n.Name())
		}
	}
	if len(list) != 1 {
		return fmt.Errorf("expected exactly one real UDC")
	}
	if e = writeSys(gadget+"/UDC", list[0]); e != nil {
		return e
	}
	b, e := os.ReadFile(gadget + "/UDC")
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(b)) != list[0] {
		return fmt.Errorf("UDC bind readback mismatch")
	}
	return nil
}
