package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
	if len(b) != 1 || b[0] > 3 {
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
