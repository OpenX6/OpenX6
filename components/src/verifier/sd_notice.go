package main

import (
	"os"
	"syscall"
)

// Closing the last descriptor also ends the indication after an unexpected exit.
func holdSDNotice(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	s, err := f.Stat()
	if err == nil && (!s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) || s.Sys().(*syscall.Stat_t).Nlink != 1) {
		err = fail("invalid stage notice")
	}
	if err == nil {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
