package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSDNoticeLifetime(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notice")
	w, e := holdSDNotice(p)
	if e != nil {
		t.Fatal(e)
	}
	r, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = syscall.Flock(int(r.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); e != syscall.EWOULDBLOCK {
		t.Fatalf("did not show: %v", e)
	}
	w.Close()
	if e = syscall.Flock(int(r.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(p, p+".link"); e != nil {
		t.Fatal(e)
	}
	if f, e := holdSDNotice(p + ".link"); e == nil {
		f.Close()
		t.Fatal("followed link")
	}
}
