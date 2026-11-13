package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// Owner policy (2026-11-02): a plaintext c9 package without any owner record is
// accepted the way the original factory updater accepted plaintext input. These
// tests pin what that does and does not loosen.

func nullSigned(s *setup, b []byte, version string, down bool) []byte {
	a := authBytes(b, version, down, s.key)
	clear(a[manifestSize:])
	return a
}

func TestUnsignedPlaintextAccepted(t *testing.T) {
	for name, mutate := range map[string]func(*setup, []byte){
		"empty-slot": nil,
		"vendor-bytes-in-slot": func(s *setup, b []byte) {
			at := int(u32(b, 4356)) + int(embeddedOffset)
			for i := 0; i < embeddedSize; i++ {
				b[at+i] = byte(0x5a + i%7)
			}
		},
		"shifted-record": func(s *setup, b []byte) {
			at := int(u32(b, 4356)) + int(embeddedOffset)
			copy(b[at+1:at+embeddedSize], authBytes(b, "1.2.36", false, s.key))
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			b := append([]byte(nil), s.b...)
			if mutate != nil {
				mutate(s, b)
			}
			f := inputFile(t, s, b)
			pub := s.key.Public().(ed25519.PublicKey)
			m, rec, at, e := verifyEmbedded(f, pub, s.p)
			if e != nil {
				t.Fatal(e)
			}
			want := sha256.Sum256(b)
			if !m.Unsigned || at != -1 || len(rec) != embeddedSize || !zero(rec[manifestSize:]) || m.Version != "1.2.36" || m.Downgrade || m.Digest != want || m.Size != uint64(len(b)) {
				t.Fatalf("unsigned description wrong: %+v at=%d", m, at)
			}
			if _, _, _, e = verifyEmbeddedSigned(f, pub, s.p); e == nil {
				t.Fatal("strict form accepted an unsigned package")
			}
			p := s.p
			p.Package = filepath.Join(p.Boundary, "imported/package.bin")
			if e = StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
			out, _ := os.ReadFile(p.Package)
			if string(out) != string(b) {
				t.Fatal("unsigned package bytes were changed while staging")
			}
			auth, _ := os.ReadFile(p.Package + ".owner-auth")
			if string(auth) != string(rec) {
				t.Fatal("staged record differs from description")
			}
			if code, e := verify(p, p.Package, "1.2.36", true); e != nil || code != 0 {
				t.Fatal("legacy verifier", code, e)
			}
			if e = StageSingle(f, p, nil); e != nil {
				t.Fatal("repeat", e)
			}
		})
	}
}

func TestUnsignedDowngradeFollowsVersions(t *testing.T) {
	s := makeSetup(t, "1.0.0", false)
	f := inputFile(t, s, s.b)
	pub := s.key.Public().(ed25519.PublicKey)
	m, _, _, e := verifyEmbedded(f, pub, s.p)
	if e != nil || !m.Unsigned || !m.Downgrade {
		t.Fatal(m, e)
	}
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	if e = StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	if code, e := verify(p, p.Package, "1.0.0", true); e != nil || code != 10 {
		t.Fatal("downgrade code", code, e)
	}
	if _, e := verify(p, p.Package, "1.2.36", true); e == nil {
		t.Fatal("native version mismatch accepted")
	}
}

func TestUnsignedStructuralChecksStillApply(t *testing.T) {
	for name, mutate := range map[string]func([]byte) []byte{
		"other-model":      func(b []byte) []byte { setstr(b, 8, 16, "c8"); repairPrefix(b); return b },
		"module-payload":   func(b []byte) []byte { b[4864+576] ^= 1; repairPrefix(b); return b },
		"FwChk":            func(b []byte) []byte { b[int(u32(b, 4612))+72] ^= 1; return b },
		"encrypted-flag":   func(b []byte) []byte { put32(b, 96, 1); repairPrefix(b); return b },
		"truncated":        func(b []byte) []byte { return b[:len(b)-1] },
		"bad-version-text": func(b []byte) []byte { setstr(b, 24, 64, "abc"); repairPrefix(b); return b },
		"extra-module":     func(b []byte) []byte { put32(b, 92, 4); repairPrefix(b); return b },
	} {
		t.Run(name, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			b := mutate(append([]byte(nil), s.b...))
			f := inputFile(t, s, b)
			if _, _, _, e := verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), s.p); e == nil {
				t.Fatal("accepted", name)
			}
		})
	}
}

func TestNullSignedRecordMustDescribeTheFile(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	s.auth = nullSigned(s, s.b, "1.2.36", false)
	s.write(t)
	if code, e := verify(s.p, s.p.Package, "1.2.36", true); e != nil || code != 0 {
		t.Fatal("null-signed record refused", code, e)
	}
	for name, mutate := range map[string]func(*setup){
		"wrong-size":        func(s *setup) { s.auth[16] ^= 1 },
		"wrong-digest":      func(s *setup) { s.auth[24] ^= 1 },
		"wrong-model":       func(s *setup) { setstr(s.auth, 56, 16, "other") },
		"wrong-version":     func(s *setup) { setstr(s.auth, 72, 32, "1.2.37") },
		"garbage-signature": func(s *setup) { s.auth[manifestSize+3] = 7 },
		"other-owner-signed": func(s *setup) {
			_, k, _ := ed25519.GenerateKey(rand.Reader)
			s.auth = authBytes(s.b, "1.2.36", false, k)
		},
		"unknown-flag":        func(s *setup) { put32(s.auth, 12, 2) },
		"shorter-than-record": func(s *setup) { s.auth = s.auth[:manifestSize] },
	} {
		t.Run(name, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			s.auth = nullSigned(s, s.b, "1.2.36", false)
			mutate(s)
			s.write(t)
			if code, e := verify(s.p, s.p.Package, "", false); e == nil || code != 64 {
				t.Fatalf("accepted: %d %v", code, e)
			}
		})
	}
	// A null-signed record cannot grant a downgrade by itself: the bit is recomputed.
	s = makeSetup(t, "1.2.36", false)
	s.auth = nullSigned(s, s.b, "1.2.36", true)
	s.write(t)
	if code, e := verify(s.p, s.p.Package, "1.2.36", true); e != nil || code != 0 {
		t.Fatal("unsigned downgrade bit honoured", code, e)
	}
}

func TestSealingRequiresRealSignature(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	// The sealer's only allowed output name is single.bin in the boundary, so the
	// input must live elsewhere.
	in := filepath.Join(s.p.Boundary, "input.bin")
	if e := os.WriteFile(in, s.b, 0600); e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(in)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	pub := s.key.Public().(ed25519.PublicKey)
	if e := SealSingle(f, nullSigned(s, s.b, "1.2.36", false), pub, s.p, filepath.Join(s.p.Boundary, "single.bin")); e == nil {
		t.Fatal("sealed with a null signature")
	}
	if _, e := os.Lstat(filepath.Join(s.p.Boundary, "single.bin")); e == nil {
		t.Fatal("output created")
	}
}
