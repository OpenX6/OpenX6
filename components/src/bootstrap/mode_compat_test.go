package main

import (
	"os"
	"testing"
)

func TestExistingPrivateSyncLibraryMode(t *testing.T) {
	f := fixture(t)
	if e := f.e.Run(); e != nil {
		t.Fatal(e)
	}
	p := f.e.path(featurePath + "/vr180-v1/sync.so")
	if e := os.Chmod(p, 0600); e != nil {
		t.Fatal(e)
	}
	os.Remove(f.e.path(featurePath + "/complete.json"))
	if e := f.e.Run(); e != nil {
		t.Fatal(e)
	}
	s, _ := os.Stat(p)
	if s.Mode().Perm() != 0600 {
		t.Fatal("mode changed")
	}
	os.Chmod(p, 0666)
	if e := f.e.Run(); e == nil {
		t.Fatal("writable library accepted")
	}
}
