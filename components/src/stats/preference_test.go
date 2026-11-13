package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreference(t *testing.T) {
	d := t.TempDir()
	os.Chmod(d, 0700)
	p := filepath.Join(d, "enabled.pref")
	if v, e := readPreference(p); e != nil || v != 0 {
		t.Fatal(v, e)
	}
	for _, v := range []byte{1, 0, 1} {
		if e := savePreference(p, v); e != nil {
			t.Fatal(e)
		}
		if got, e := readPreference(p); e != nil || got != v {
			t.Fatal(got, e)
		}
	}
	for _, b := range [][]byte{{2}, {1, 0}, {}} {
		os.WriteFile(p, b, 0600)
		if _, e := readPreference(p); e == nil {
			t.Fatal("accepted malformed")
		}
	}
	os.Remove(p)
	os.Symlink("missing", p)
	if e := savePreference(p, 1); e == nil {
		t.Fatal("accepted symlink")
	}
	os.Remove(p)
	os.WriteFile(p, []byte{1}, 0644)
	if _, e := readPreference(p); e == nil {
		t.Fatal("accepted permissions")
	}
	os.Remove(p)
	os.Chmod(d, 0755)
	if e := savePreference(p, 1); e == nil {
		t.Fatal("accepted public directory")
	}
}
