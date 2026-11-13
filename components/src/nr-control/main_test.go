package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnumsAndAtomic(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "state")
	for i := 0; i < 256; i++ {
		if e := atomic(p, []byte{byte(i)}, 0600); e != nil {
			t.Fatal(e)
		}
		v, e := readState(p)
		if (e == nil) != (i < 4) {
			t.Fatal(i, e)
		}
		if i < 4 && v != byte(i) {
			t.Fatal(i, v)
		}
	}
	for _, b := range [][]byte{{}, {0, 1}} {
		os.WriteFile(p, b, 0600)
		if _, e := readState(p); e == nil {
			t.Fatal("bad length accepted")
		}
	}
	os.Remove(p)
	q := filepath.Join(d, "elsewhere")
	os.WriteFile(q, []byte{0}, 0600)
	os.Symlink(q, p)
	if _, e := readState(p); e == nil {
		t.Fatal("symlink accepted")
	}
	if e := atomic(p, []byte{1}, 0600); e == nil {
		t.Fatal("symlink overwrite accepted")
	}
	os.Remove(p)
	os.WriteFile(p, []byte{0}, 0600)
	os.Chmod(p, 0666)
	if _, e := readState(p); e == nil {
		t.Fatal("writable state accepted")
	}
}
func TestInvalidApplyNoPlatformAccess(t *testing.T) {
	for _, v := range []byte{4, 255} {
		if e := apply(v); e == nil {
			t.Fatal(v)
		}
	}
}
