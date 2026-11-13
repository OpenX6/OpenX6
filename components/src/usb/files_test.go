package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerFilePolicy(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "key")
	if e := os.WriteFile(p, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readOwned(p, true); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []os.FileMode{0666, 0620, 0602} {
		os.Chmod(p, mode)
		if _, e := readOwned(p, false); e == nil {
			t.Fatalf("writable public key accepted %o", mode)
		}
	}
	os.Chmod(p, 0644)
	if _, e := readOwned(p, false); e != nil {
		t.Fatal(e)
	}
	if _, e := readOwned(p, true); e == nil {
		t.Fatal("readable private key accepted")
	}
	link := filepath.Join(d, "link")
	os.Symlink(p, link)
	if _, e := readOwned(link, false); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := readOwned(d, false); e == nil {
		t.Fatal("directory accepted")
	}
	if _, e := readOwned(filepath.Join(d, "absent"), false); e == nil {
		t.Fatal("missing key accepted")
	}
}
