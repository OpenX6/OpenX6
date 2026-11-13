package main

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestSealAndImportRoundTrip(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	src, e := os.Open(s.p.Package)
	if e != nil {
		t.Fatal(e)
	}
	defer src.Close()
	auth, e := os.ReadFile(s.p.Package + ".owner-auth")
	if e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(s.p.Boundary, "single.bin")
	pub := s.key.Public().(ed25519.PublicKey)
	if e = SealSingle(src, auth, pub, s.p, out); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(out)
	if !bytes.Equal(b, embedTest(t, s)) {
		t.Fatal("different envelope")
	}
	if e = SealSingle(src, auth, pub, s.p, out); e == nil {
		t.Fatal("overwrote output")
	}
	f, e := os.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	p := s.p
	p.Package = filepath.Join(p.Boundary, "sealed-import/package.bin")
	if e = StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(p.Package)
	if !bytes.Equal(got, s.b) {
		t.Fatal("canonical data changed")
	}
}
func TestSealInvalidAuthNoOutput(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	f, e := os.Open(s.p.Package)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	auth, _ := os.ReadFile(s.p.Package + ".owner-auth")
	auth[199] ^= 1
	out := filepath.Join(s.p.Boundary, "single.bin")
	if e = SealSingle(f, auth, s.key.Public().(ed25519.PublicKey), s.p, out); e == nil {
		t.Fatal("invalid accepted")
	}
	if _, e = os.Lstat(out); !os.IsNotExist(e) {
		t.Fatal("output created")
	}
}
