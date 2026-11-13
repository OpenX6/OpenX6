package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func prefDir(path string) error {
	s, e := os.Lstat(filepath.Dir(path))
	if e != nil {
		return e
	}
	if !s.IsDir() || s.Mode().Perm() != 0700 || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("unsafe preference directory")
	}
	return nil
}
func readPreference(path string) (byte, error) {
	if e := prefDir(path); e != nil {
		return 0, e
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(e) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return 0, e
	}
	if !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return 0, fmt.Errorf("unsafe preference")
	}
	b, e := io.ReadAll(io.LimitReader(f, 2))
	if e != nil || len(b) != 1 || b[0] > 1 {
		return 0, fmt.Errorf("invalid preference")
	}
	return b[0], nil
}
func savePreference(path string, v byte) error {
	if v > 1 {
		return fmt.Errorf("invalid preference")
	}
	if _, e := readPreference(path); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".pref-")
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
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
