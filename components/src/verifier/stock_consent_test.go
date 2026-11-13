package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func TestFactoryClassAndDigest(t *testing.T) {
	for _, enc := range []uint32{0, 1, 2} {
		b := make([]byte, 777)
		copy(b, []byte("MWFI"))
		copy(b[8:], []byte("c9"))
		binary.LittleEndian.PutUint32(b[96:], enc)
		s := makeSetup(t, "1.2.36", false)
		f := inputFile(t, s, b)
		m, _, _, e := inspectCard(f, nil, s.p)
		if e != nil || !m.Native || m.Factory != (enc == 1) || m.Digest != sha256.Sum256(b) {
			t.Fatal(enc, e)
		}
	}
}
func TestFactoryTwoDecisions(t *testing.T) {
	for _, deny := range []byte{0, 1, 2} {
		c, o, n := sdFixture()
		o.Factory = true
		o.Native = true
		d := stockDecision{}
		var pages []byte
		stages := 0
		c.Stage = func() error { stages++; return nil }
		c.ConfirmFactory = func() bool {
			return d.allow(o.PackageDigest, func(p byte) bool { pages = append(pages, p); return p != deny })
		}
		_, e := c.Step(o)
		if e != nil {
			t.Fatal(e)
		}
		if deny == 0 {
			if *n != 1 || stages != 1 || !bytes.Equal(pages, []byte{1, 2}) {
				t.Fatal(n, pages)
			}
			continue
		}
		c.Step(o)
		if *n != 0 || stages != 0 || len(pages) != int(deny) {
			t.Fatal(n, pages)
		}
		old := o.PackageDigest
		o.PackageDigest = o.CurrentRelease
		o.CurrentRelease = old
		c.Step(o)
		if len(pages) != 2*int(deny) {
			t.Fatal("changed package did not get fresh prompt")
		}
	}
	c, o, n := sdFixture()
	o.Factory = true
	c.Step(o)
	if *n != 0 {
		t.Fatal("missing UI accepted")
	}
	c.State = SDState{Version: 1, Phase: "staged", Boot: o.Boot, SourceSlot: o.Slot, Digest: o.PackageDigest, Release: o.PackageRelease, Card: o.Card, Native: true}
	c.Step(o)
	if *n != 0 {
		t.Fatal("staged recovery bypassed confirmation")
	}
	for _, native := range []bool{false, true} {
		c, o, n := sdFixture()
		o.Native = native
		c.ConfirmFactory = func() bool { t.Fatal("unexpected prompt"); return false }
		c.Step(o)
		if *n != 1 {
			t.Fatal("ordinary handoff blocked")
		}
	}
}
