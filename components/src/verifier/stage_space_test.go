package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageSpaceBudget(t *testing.T) {
	size := uint64(1755419522)
	for _, x := range []struct {
		name       string
		data, card uint64
		backend    string
	}{
		{"ample-data", size + importMargin + migrationReserve, 0, "data"},
		{"migration-alone", migrationReserve, 3 << 30, "sd"},
		{"data-short", migrationReserve - 1, 100 << 30, ""},
		{"card-short", migrationReserve, 0, ""},
		{"import-alone-is-not-enough", size + importMargin, 3 << 30, "sd"},
	} {
		t.Run(x.name, func(t *testing.T) {
			p, e := planStage(size, x.data, x.card)
			if x.backend == "" {
				if e == nil {
					t.Fatal("accepted insufficient space")
				}
				return
			}
			if e != nil || p.Backend != x.backend {
				t.Fatalf("%+v %v", p, e)
			}
			if p.Backend == "sd" && (p.ImageBytes < size+importMargin || p.CardRequired > x.card) {
				t.Fatal("bad ext4 budget")
			}
		})
	}
	for _, n := range []uint64{0, 9983, 1 << 32, ^uint64(0)} {
		if _, e := planStage(n, ^uint64(0), ^uint64(0)); e == nil {
			t.Fatal("invalid size accepted")
		}
	}
}
func TestStageRecoveryPolicy(t *testing.T) {
	for _, phase := range []string{"idle", "resident", "staged", "completed"} {
		if !stageMayRecover(phase) {
			t.Fatal(phase)
		}
	}
	for _, phase := range []string{"intent", "dispatched", "uncertain", "garbage", ""} {
		if stageMayRecover(phase) {
			t.Fatal(phase)
		}
	}
}

func TestStagedRebootRestoresBeforeHandoff(t *testing.T) {
	c, o, n := sdFixture()
	o.InstallAllowed = false
	if status, e := c.Step(o); e != nil || status != "staged-install-locked" {
		t.Fatal(status, e)
	}
	o.Boot = "new-boot"
	o.InstallAllowed = true
	restored := false
	c.Restage = func() error { restored = true; return nil }
	c.Prepare = func() error {
		if !restored {
			t.Fatal("handoff before restaging")
		}
		return nil
	}
	if status, e := c.Step(o); e != nil || status != "dispatched" || *n != 1 {
		t.Fatal(status, e, *n)
	}
}
func TestRestageFailureNeverDispatches(t *testing.T) {
	c, o, n := sdFixture()
	c.Restage = func() error { return fail("SD absent/tampered") }
	if status, e := c.Step(o); e == nil || status != "restage-refused" || *n != 0 || c.State.Phase != "staged" {
		t.Fatal(status, e, *n, c.State)
	}
}
func TestLeaseValidation(t *testing.T) {
	good := StageLease{Version: 1, Card: strings.Repeat("a", 64), Digest: strings.Repeat("b", 64), Boot: "boot", Image: "OpenX6-private-stage-" + strings.Repeat("c", 32) + ".img", Phase: "creating", Bytes: 2 << 30, CardMountID: 10}
	for _, phase := range []string{"creating", "allocated", "mounted", "published"} {
		s := good
		s.Phase = phase
		b, _ := json.Marshal(s)
		if _, e := decodeStageLease(b); e != nil {
			t.Fatal(phase, e)
		}
	}
	for _, bad := range []string{"version", "card", "digest", "path", "size", "phase", "mount", "boot", "unknown-field"} {
		t.Run(bad, func(t *testing.T) {
			s := good
			switch bad {
			case "version":
				s.Version = 2
			case "card":
				s.Card = ""
			case "digest":
				s.Digest = "invalid"
			case "path":
				s.Image = "../system.img"
			case "size":
				s.Bytes = 7 << 30
			case "phase":
				s.Phase = "anything"
			case "mount":
				s.CardMountID = 0
			case "boot":
				s.Boot = ""
			}
			b, _ := json.Marshal(s)
			if bad == "unknown-field" {
				b = append(b[:len(b)-1], []byte(",\"unknown\":true}")...)
			}
			if _, e := decodeStageLease(b); e == nil {
				t.Fatal("accepted invalid lease")
			}
		})
	}
}
func TestRelocatedPrivateStageLegacyVerification(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	f := inputFile(t, s, embedTest(t, s))
	p := s.p
	fs := filepath.Join(p.Boundary, "private-ext4")
	if e := os.Mkdir(fs, 0700); e != nil {
		t.Fatal(e)
	}
	p.Package = filepath.Join(fs, "work/package.bin")
	if e := StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	if code, e := verify(p, p.Package, "1.2.36", true); e != nil || code != 0 {
		t.Fatal(code, e)
	}
	b, e := os.ReadFile(p.Package)
	if e != nil || !bytes.Equal(b, s.b) {
		t.Fatal("relocation changed authenticated contents", e)
	}
}

func TestCompletedCleanupResumesAfterRetirementRename(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	f := inputFile(t, s, embedTest(t, s))
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	if e := StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	m, _, _, e := verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), p)
	if e != nil {
		t.Fatal(e)
	}
	state := SDState{Version: 1, Phase: "completed", Digest: fmt.Sprintf("%x", m.Digest)}
	retired := filepath.Join(p.Boundary, ".x6-single-retired-"+state.Digest)
	if e = os.Rename(filepath.Dir(p.Package), retired); e != nil {
		t.Fatal(e)
	}
	if e = cleanupCompletedStage(p, state); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(retired); !os.IsNotExist(e) {
		t.Fatal("retired import leaked", e)
	}
}

func TestRealLeaseFileVariableLength(t *testing.T) {
	dir := t.TempDir()
	p := policy{Boundary: dir, UID: uint32(os.Getuid())}
	path := filepath.Join(dir, "lease.json")
	s := StageLease{Version: 1, Card: strings.Repeat("a", 64), Digest: strings.Repeat("b", 64), Boot: "real-boot", Image: "OpenX6-private-stage-" + strings.Repeat("c", 32) + ".img", Phase: "mounted", Bytes: 2 << 30, CardMountID: 86}
	b, _ := json.Marshal(s)
	if len(b) == 4096 {
		t.Fatal("test must exercise short JSON")
	}
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	got, e := loadStageLeaseAt(path, p)
	if e != nil || got != s {
		t.Fatal(got, e)
	}
	os.Chmod(path, 0644)
	if _, e := loadStageLeaseAt(path, p); e == nil {
		t.Fatal("loose lease accepted")
	}
	os.Chmod(path, 0600)
	os.WriteFile(path, bytes.Repeat([]byte(" "), 4097), 0600)
	if _, e := loadStageLeaseAt(path, p); e == nil {
		t.Fatal("oversize lease accepted")
	}
}
func TestExistingStageEntryWithoutNativeVersion(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	f := inputFile(t, s, embedTest(t, s))
	defer f.Close()
	if e := StageSingle(f, s.p, nil); e != nil {
		t.Fatal(e)
	}
	pub, e := readSmall(s.p.Public, s.p, false, 32)
	if e != nil {
		t.Fatal(e)
	}
	m, _, _, e := verifyEmbedded(f, pub, s.p)
	if e != nil {
		t.Fatal(e)
	}
	path := f.Name()
	if e := stageWithSpace(f, &path, "unused", fmt.Sprintf("%x", m.Digest), "unused", s.p, nil); e != nil {
		t.Fatal(e)
	}
}

func TestForeignReleaseHandoffSpace(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	raw := embedTest(t, s)
	offset := int(u32(raw, 4356)) + int(releaseOffset)
	copy(raw[offset:], "X6REL01\x00")
	raw[offset+8] = 42
	f := inputFile(t, s, raw)
	p := s.p
	p.Package = filepath.Join(p.Boundary, "proof-stage/package.bin")
	if err := stageCard(f, p, nil); err != nil {
		t.Fatal(err)
	}
	m, err := stagedRecord(p)
	if err != nil || !m.Native {
		t.Fatal(m, err)
	}
	reserve, err := inputReserve(f, m.Native)
	if err != nil || reserve != migrationReserve {
		t.Fatal(reserve, err)
	}
	plan, err := planInputStage(1760471446, 2450874368, 4<<30, true, reserve)
	if err != nil || plan.Backend != "sd" || !plan.Native {
		t.Fatal(plan, err)
	}
	for _, free := range []uint64{681275392, migrationReserve - 1, migrationReserve} {
		err = checkHandoffSpace(p, func(string) (uint64, error) { return free, nil })
		if (err != nil) != (free < migrationReserve) {
			t.Fatal(free, err)
		}
	}
	c, o, _ := sdFixture()
	c.Dispatch = func() error { return errHandoffSpace }
	if status, e := c.Step(o); status != "prepare-refused" || e == nil || c.State.Phase != "staged" {
		t.Fatal(status, e, c.State)
	}
}
