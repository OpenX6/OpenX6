package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func embedTest(t *testing.T, s *setup) []byte {
	t.Helper()
	b := append([]byte(nil), s.b...)
	at := int(u32(b, 4356)) + int(embeddedOffset)
	if !zero(b[at : at+embeddedSize]) {
		t.Fatal("slot not empty")
	}
	auth := authBytes(b, "1.2.36", false, s.key)
	copy(b[at:], auth)
	return b
}
func inputFile(t *testing.T, s *setup, b []byte) *os.File {
	t.Helper()
	p := filepath.Join(s.p.Boundary, "single.bin")
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.Close() })
	return f
}
func TestEmbeddedAndLegacyStaging(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	b := embedTest(t, s)
	f := inputFile(t, s, b)
	pub := s.key.Public().(ed25519.PublicKey)
	if _, _, _, e := verifyEmbedded(f, pub, s.p); e != nil {
		t.Fatal(e)
	}
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	if e := StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	if e := StageSingle(f, p, nil); e != nil {
		t.Fatal("repeat", e)
	}
	if code, e := verify(p, p.Package, "1.2.36", true); e != nil || code != 0 {
		t.Fatal("legacy verifier", code, e)
	}
	out, _ := os.ReadFile(p.Package)
	if string(out) != string(s.b) {
		t.Fatal("normalization changed unsigned region")
	}
}
func TestEmbeddedTamper(t *testing.T) {
	// "moved-window" and "missing-record" no longer belong here: a slot without our
	// record is an unsigned plaintext package, accepted by owner policy (see
	// unsigned_test.go). A record that is present and wrong is still refused.
	for _, name := range []string{"signature", "payload-repaired-digests", "outside-auth-cert", "version", "unknown-flag", "truncated", "tail", "wrong-key"} {
		t.Run(name, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			b := embedTest(t, s)
			at := int(u32(b, 4356)) + int(embeddedOffset)
			pub := s.key.Public().(ed25519.PublicKey)
			switch name {
			case "signature":
				b[at+manifestSize] ^= 1
			case "payload-repaired-digests":
				b[4864+576] ^= 1
				h := sumForTest(b[4864+576 : int(u32(b, 4100)+u32(b, 4104))])
				copy(b[4864+264:], hex.EncodeToString(h))
				repairPrefix(b)
			case "outside-auth-cert":
				b[at+embeddedSize] ^= 1
			case "version":
				b[24] = '2'
				repairPrefix(b)
			case "unknown-flag":
				b[at+12] = 2
			case "truncated":
				b = b[:len(b)-1]
			case "tail":
				b[len(b)-1] ^= 1
			case "wrong-key":
				pub, _, _ = ed25519.GenerateKey(rand.Reader)
			}
			f := inputFile(t, s, b)
			if _, _, _, e := verifyEmbedded(f, pub, s.p); e == nil {
				t.Fatal("accepted", name)
			}
		})
	}
}
func sumForTest(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
func TestImportFaultRecovery(t *testing.T) {
	for _, point := range []string{"before-copy", "copied", "normalized", "before-publish", "published"} {
		t.Run(point, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			f := inputFile(t, s, embedTest(t, s))
			p := s.p
			p.Package = filepath.Join(p.Boundary, "imported/package.bin")
			if e := StageSingle(f, p, func(s string) error {
				if s == point {
					return errors.New("interrupted")
				}
				return nil
			}); e == nil {
				t.Fatal("no interruption")
			}
			if _, e := os.Stat(filepath.Dir(p.Package)); e == nil {
				if _, e = verify(p, p.Package, "1.2.36", true); e != nil {
					t.Fatal("partial visible", e)
				}
			}
			if e := StageSingle(f, p, nil); e != nil {
				t.Fatal("retry", e)
			}
		})
	}
}
func TestImportRefusesOccupiedDirectory(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	f := inputFile(t, s, embedTest(t, s))
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	os.Mkdir(filepath.Dir(p.Package), 0700)
	os.WriteFile(filepath.Join(filepath.Dir(p.Package), "keep"), []byte("untouched"), 0600)
	if StageSingle(f, p, nil) == nil {
		t.Fatal("overwrote occupied directory")
	}
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(p.Package), "keep"))
	if string(b) != "untouched" {
		t.Fatal("data changed")
	}
}
func TestEmitInertSingleFile(t *testing.T) {
	dir := os.Getenv("X6_EMBEDDED_FIXTURE_OUT")
	if dir == "" {
		t.Skip("explicit inert fixture output only")
	}
	s := makeSetup(t, "1.2.36", false)
	b := embedTest(t, s)
	if e := os.WriteFile(filepath.Join(dir, "inert-single.bin"), b, 0600); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "test-owner.pub"), s.key.Public().(ed25519.PublicKey), 0644)
	f := inputFile(t, s, b)
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	if err := StageSingle(f, p, nil); err != nil {
		t.Fatal(err)
	}
	for from, to := range map[string]string{p.Package: "normalized.bin", p.Package + ".owner-auth": "normalized.bin.owner-auth"} {
		b, e := os.ReadFile(from)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, to), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
