package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestForeignCardPipeline(t *testing.T) {
	for _, typ := range []string{"plain-factory", "other-signer", "unverifiable-record", "noncanonical-modules", "no-manifest-short"} {
		t.Run(typ, func(t *testing.T) {
			s := makeSetup(t, "1.0.0", false)
			raw := append([]byte(nil), s.b...)
			switch typ {
			case "other-signer":
				_, k, _ := ed25519.GenerateKey(rand.Reader)
				copy(raw[int(u32(raw, 4356))+512:], authBytes(raw, "1.0.0", true, k))
			case "unverifiable-record":
				copy(raw[int(u32(raw, 4356))+512:], []byte("X6UPD01\x00"))
				raw[int(u32(raw, 4356))+650] = 2
			case "noncanonical-modules":
				put32(raw, 4, 2)
				put32(raw, 92, 6)
				setstr(raw, 24, 64, "Vendor test build")
			case "no-manifest-short":
				raw = []byte("MWFI test data delegated for native rejection")
			}
			f := inputFile(t, s, raw)
			m, _, at, e := inspectCard(f, nil, s.p)
			if e != nil || !m.Native || at != -1 || m.Digest != sha256.Sum256(raw) {
				t.Fatal(m, at, e)
			}
			p := s.p
			p.Package = filepath.Join(p.Boundary, "stage/package.bin")
			p.Public = filepath.Join(p.Boundary, "no-such-publisher.pub")
			if e = stageCard(f, p, nil); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(p.Package)
			if e != nil || !bytes.Equal(b, raw) {
				t.Fatal("copy differs", e)
			}
			if again := stageCard(f, p, nil); again != nil {
				t.Fatal("repeat", again)
			}
			if code, e := verify(p, p.Package, "", false); code != 0 || e != nil {
				t.Fatal("entry", code, e)
			}
			if code, e := verify(p, p.Package, "vendor-candidate-version", true); code != 10 || e != nil {
				t.Fatal("version", code, e)
			}
			if _, _, _, e := verifyEmbeddedSigned(f, s.key.Public().(ed25519.PublicKey), s.p); e == nil {
				t.Fatal("foreign falsely attested")
			}
			if release, e := cardRelease(f, m); e != nil || release != fmt.Sprintf("%x", m.Digest) {
				t.Fatal("raw release marker required", release, e)
			}
			b[len(b)-1] ^= 1
			os.Chmod(p.Package, 0600)
			os.WriteFile(p.Package, b, 0600)
			os.Chmod(p.Package, 0400)
			if _, e := verify(p, p.Package, "", false); e == nil {
				t.Fatal("changed copy accepted")
			}
		})
	}
}
func TestNativeReturnPreservesCard(t *testing.T) {
	r := t.TempDir()
	path := filepath.Join(r, filepath.Base(consumedCardPath))
	b := []byte("MWFI my image")
	os.WriteFile(path, b, 0600)
	digest := fmt.Sprintf("%x", sha256.Sum256(b))
	if e := archiveNativeCard(path, digest); e != nil {
		t.Fatal(e)
	}
	if v, e := os.ReadFile(path + "." + digest); e != nil || !bytes.Equal(v, b) {
		t.Fatal("archive changed")
	}
	os.WriteFile(path, b, 0600)
	if e := archiveNativeCard(path, digest); e != nil {
		t.Fatal(e)
	}
	c, o, n := sdFixture()
	o.Native = true
	o.PackageRelease = digest
	if _, e := c.Step(o); e != nil || *n != 1 || !c.State.Native {
		t.Fatal("not dispatched", n, c.State, e)
	}
	if status, e := c.Step(o); e != nil || status != "pending-outcome-no-retry" || *n != 1 {
		t.Fatal(status, e)
	}
	o.Boot = "next-boot"
	o.NativeIdle = true
	if status, e := c.Step(o); e != nil || status != "native-session-ended" || *n != 1 {
		t.Fatal(status, e)
	}
	for _, size := range []uint64{4, 1700, 5 << 30} {
		plan, e := planInputStage(size, 128<<20, 9<<30, true, importMargin)
		if e != nil || !plan.Native {
			t.Fatal(size, plan, e)
		}
	}
}
