package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSDJournalAndRefusals(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	path := filepath.Join(s.p.Boundary, "journal/state.json")
	v := SDState{Version: 1, Phase: "idle"}
	if e := saveSDState(path, s.p, v); e != nil {
		t.Fatal(e)
	}
	r, e := loadSDState(path, s.p)
	if e != nil || r != v {
		t.Fatal(r, e)
	}
	for _, kind := range []string{"mode", "corrupt", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			os.Remove(path)
			os.WriteFile(path, []byte(`{"Version":1,"Phase":"idle"}`), 0600)
			switch kind {
			case "mode":
				os.Chmod(path, 0644)
			case "corrupt":
				os.WriteFile(path, []byte(`{`), 0600)
			case "symlink":
				os.Rename(path, path+".back")
				os.Symlink(path+".back", path)
			case "hardlink":
				os.Link(path, path+".link")
			}
			if _, e := loadSDState(path, s.p); e == nil {
				t.Fatal("accepted unsafe journal")
			}
			if e := saveSDState(path, s.p, v); e == nil {
				t.Fatal("replaced unsafe journal")
			}
		})
	}
}
func TestImportOrphanCleanup(t *testing.T) {
	for _, bad := range []string{"", "extra-file", "symlink", "wrong-receipt"} {
		t.Run(bad, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			pub := s.key.Public().(ed25519.PublicKey)
			f := inputFile(t, s, embedTest(t, s))
			m, _, _, e := verifyEmbedded(f, pub, s.p)
			if e != nil {
				t.Fatal(e)
			}
			dig := fmt.Sprintf("%x", m.Digest)
			dir, e := os.MkdirTemp(s.p.Boundary, ".x6-single-import-")
			if e != nil {
				t.Fatal(e)
			}
			marker := []byte("X6 SINGLE IMPORT V2\n" + dig + "\n")
			if bad == "wrong-receipt" {
				marker = []byte("foreign")
			}
			os.WriteFile(filepath.Join(dir, ".x6-import-receipt"), marker, 0400)
			os.WriteFile(filepath.Join(dir, "package.bin"), []byte("partial"), 0400)
			if bad == "extra-file" {
				os.WriteFile(filepath.Join(dir, "photos"), []byte("keep"), 0400)
			}
			if bad == "symlink" {
				os.Remove(filepath.Join(dir, "package.bin"))
				os.Symlink(s.p.Public, filepath.Join(dir, "package.bin"))
			}
			if e = cleanImportOrphans(s.p.Boundary, s.p, dig); e != nil {
				t.Fatal(e)
			}
			_, e = os.Stat(dir)
			if bad == "" && !os.IsNotExist(e) {
				t.Fatal("own orphan retained")
			}
			if bad != "" && e != nil {
				t.Fatal("foreign orphan removed")
			}
		})
	}
}
func TestReleaseMetadataAuthenticated(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	at := int(u32(s.b, 4356)) + int(releaseOffset)
	copy(s.b[at:], append([]byte("X6REL01\x00"), []byte(strings.Repeat("R", 32))...))
	b := embedTest(t, s)
	f := inputFile(t, s, b)
	v, e := packageRelease(f)
	if e != nil || v != strings.Repeat("52", 32) {
		t.Fatal(v, e)
	}
	if _, _, _, e = verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), s.p); e != nil {
		t.Fatal(e)
	}
	b[at+9] ^= 1
	os.WriteFile(f.Name(), b, 0600)
	if _, _, _, e = verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), s.p); e == nil {
		t.Fatal("release tamper accepted")
	}
}
func TestCoordinatorIdentityMustSurvive(t *testing.T) {
	a := strings.Repeat("a", 64)
	c := SDCoordinator{State: SDState{Version: 1, Phase: "dispatched", Boot: "old", SourceSlot: "_a", Digest: a, Release: a, Identity: a}, Save: func(SDState) error { return nil }}
	o := SDObservation{Boot: "new", Slot: "_b", CurrentRelease: a, Ready: true, CompletionVerified: true, Identity: strings.Repeat("b", 64)}
	status, e := c.Step(o)
	if e != nil || status != "pending-outcome-no-retry" {
		t.Fatal(status, e)
	}
	o.Identity = a
	status, e = c.Step(o)
	if e != nil || status != "completed" {
		t.Fatal(status, e)
	}
}
func TestCoordinatorSIGKILL(t *testing.T) {
	if phase := os.Getenv("X6_KILL_PHASE"); phase != "" {
		dir := os.Getenv("X6_KILL_DIR")
		p := policy{Boundary: dir, UID: uint32(os.Getuid())}
		path := filepath.Join(dir, "journal/state.json")
		a := strings.Repeat("a", 64)
		c := SDCoordinator{Save: func(s SDState) error { return saveSDState(path, p, s) }, Stage: func() error { return nil }, Dispatch: func() error { return os.WriteFile(filepath.Join(dir, "dispatch"), []byte("1"), 0600) }, Fault: func(s string) error {
			if s == phase {
				syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		}}
		_, e := c.Step(SDObservation{Boot: "old", Slot: "_a", CurrentRelease: strings.Repeat("b", 64), Presence: "present", PackageDigest: a, PackageRelease: a, Ready: true, InstallAllowed: true})
		if e != nil {
			t.Fatal(e)
		}
		t.Fatal("did not kill")
	}
	for _, phase := range []string{"staged-before-save", "staged", "intent", "dispatched-before-save", "dispatched"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCoordinatorSIGKILL$")
			cmd.Env = append(os.Environ(), "X6_KILL_PHASE="+phase, "X6_KILL_DIR="+dir)
			if e := cmd.Run(); e == nil {
				t.Fatal("child survived")
			}
			p := policy{Boundary: dir, UID: uint32(os.Getuid())}
			s, e := loadSDState(filepath.Join(dir, "journal/state.json"), p)
			if e != nil {
				t.Fatal(e)
			}
			want := "intent"
			if phase == "staged" || phase == "dispatched" {
				want = phase
			}
			if phase == "staged-before-save" {
				want = ""
			}
			if s.Phase != want {
				t.Fatal(s)
			}
			calls := 0
			c := SDCoordinator{State: s, Save: func(SDState) error { return nil }, Stage: func() error { return nil }, Dispatch: func() error { calls++; return nil }}
			_, e = c.Step(SDObservation{Boot: "old", Slot: "_a", CurrentRelease: strings.Repeat("b", 64), Presence: "present", PackageDigest: strings.Repeat("a", 64), PackageRelease: strings.Repeat("a", 64), Ready: true, InstallAllowed: true})
			if e != nil {
				t.Fatal(e)
			}
			safe := phase == "staged" || phase == "staged-before-save"
			if safe && calls != 1 || !safe && calls != 0 {
				t.Fatal("duplicate or lost safe resume", calls)
			}
		})
	}
}
func TestImportSIGKILLAndResume(t *testing.T) {
	if point := os.Getenv("X6_IMPORT_KILL"); point != "" {
		root := os.Getenv("X6_IMPORT_ROOT")
		p := policy{filepath.Join(root, "imported/package.bin"), filepath.Join(root, "owner.pub"), root, "c9", "1.2.36", uint32(os.Geteuid())}
		f, e := os.Open(filepath.Join(root, "single.bin"))
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		e = StageSingle(f, p, func(at string) error {
			if at == point {
				syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		})
		t.Fatal("did not kill", e)
	}
	for _, point := range []string{"before-copy", "copied", "normalized", "before-publish", "published"} {
		t.Run(point, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			f := inputFile(t, s, embedTest(t, s))
			p := s.p
			p.Package = filepath.Join(p.Boundary, "imported/package.bin")
			cmd := exec.Command(os.Args[0], "-test.run=^TestImportSIGKILLAndResume$")
			cmd.Env = append(os.Environ(), "X6_IMPORT_KILL="+point, "X6_IMPORT_ROOT="+s.p.Boundary)
			if e := cmd.Run(); e == nil {
				t.Fatal("child survived")
			}
			if e := StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
			if _, e := verify(p, p.Package, "1.2.36", true); e != nil {
				t.Fatal(e)
			}
			left, _ := filepath.Glob(filepath.Join(s.p.Boundary, ".x6-single-import-*"))
			if len(left) != 0 {
				t.Fatal("unreclaimed orphan", left)
			}
		})
	}
}
func TestPrepareFailureAndLockNeverDispatch(t *testing.T) {
	c, o, n := sdFixture()
	prepared := 0
	c.Prepare = func() error { prepared++; return fmt.Errorf("not idle") }
	o.InstallAllowed = false
	c.Step(o)
	if prepared != 0 || *n != 0 {
		t.Fatal("lock bypass")
	}
	o.InstallAllowed = true
	_, e := c.Step(o)
	if e == nil || prepared != 1 || *n != 0 || c.State.Phase != "staged" {
		t.Fatal("preparation failure crossed intent")
	}
}
func TestVerifiedStageRetirement(t *testing.T) {
	for _, kind := range []string{"completed", "pending", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			f := inputFile(t, s, embedTest(t, s))
			p := s.p
			p.Package = filepath.Join(p.Boundary, "imported/package.bin")
			m, _, _, e := verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), p)
			if e != nil {
				t.Fatal(e)
			}
			if e = StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
			state := SDState{Version: 1, Phase: "completed", Boot: "old", SourceSlot: "_a", Digest: fmt.Sprintf("%x", m.Digest), Release: strings.Repeat("a", 64)}
			if kind == "pending" {
				state.Phase = "intent"
			}
			if kind == "foreign" {
				os.WriteFile(filepath.Join(filepath.Dir(p.Package), "keep-photo"), []byte("keep"), 0400)
			}
			e = retireVerifiedStage(p, state, strings.Repeat("b", 64))
			if kind == "completed" {
				if e != nil {
					t.Fatal(e)
				}
				if _, e = os.Stat(p.Package); !os.IsNotExist(e) {
					t.Fatal("not retired")
				}
			} else {
				if e == nil {
					t.Fatal("unsafe retirement")
				}
				if _, e = os.Stat(p.Package); e != nil {
					t.Fatal("removed protected stage")
				}
			}
		})
	}
}
func TestFullLockedCandidate(t *testing.T) {
	source := os.Getenv("X6_FULL_SINGLE")
	if source == "" {
		t.Skip("explicit offline artifact required")
	}
	root := t.TempDir()
	pub, e := os.ReadFile(os.Getenv("X6_FULL_PUBLIC"))
	if e != nil {
		t.Fatal(e)
	}
	p := policy{filepath.Join(root, "imported/package.bin"), filepath.Join(root, "owner.pub"), root, "c9", "1.2.36", uint32(os.Geteuid())}
	os.WriteFile(p.Public, pub, 0644)
	f, e := os.Open(source)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	m, _, _, e := verifyEmbedded(f, pub, p)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", m.Digest) != os.Getenv("X6_FULL_CANONICAL_SHA") {
		t.Fatal("canonical identity mismatch")
	}
	release, e := packageRelease(f)
	if e != nil || release != os.Getenv("X6_FULL_RELEASE") {
		t.Fatal("release mismatch", e)
	}
	if e = StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	if e = StageSingle(f, p, nil); e != nil {
		t.Fatal("repeat", e)
	}
	if _, e = verify(p, p.Package, "", false); e != nil {
		t.Fatal("entry", e)
	}
	if _, e = verify(p, p.Package, "1.2.36", true); e != nil {
		t.Fatal("target version", e)
	}
	if _, e = verify(p, p.Package, "0.8.825", true); e == nil {
		t.Fatal("wrong target metadata accepted")
	}
	if m.Downgrade {
		t.Fatal("unrequested downgrade authorization")
	}
	calls := 0
	journal := filepath.Join(root, "journal/state.json")
	c := SDCoordinator{Save: func(s SDState) error { return saveSDState(journal, p, s) }, Stage: func() error { return StageSingle(f, p, nil) }, Dispatch: func() error { calls++; return nil }}
	o := SDObservation{Boot: "source", Slot: "_a", CurrentRelease: strings.Repeat("0", 64), Presence: "present", PackageDigest: fmt.Sprintf("%x", m.Digest), PackageRelease: release, Ready: true, InstallAllowed: true, Identity: strings.Repeat("1", 64)}
	if _, e = c.Step(o); e != nil || calls != 1 {
		t.Fatal("model handoff", e)
	}
	c.State, e = loadSDState(journal, p)
	if e != nil {
		t.Fatal(e)
	}
	o.Boot = "target"
	o.Slot = "_b"
	o.CurrentRelease = release
	o.CompletionVerified = true
	if _, e = c.Step(o); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5; i++ {
		if _, e = c.Step(o); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 1 || c.State.Phase != "completed" {
		t.Fatal("resident loop")
	}

	t.Log("Full production-signed locked file verified, imported, normalized, repeated, legacy checks passed; no native updater executed")
}
func TestPublishedBeforeJournalRecovery(t *testing.T) {
	for _, phase := range []string{"idle", "completed"} {
		t.Run(phase, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			f := inputFile(t, s, embedTest(t, s))
			p := s.p
			p.Package = filepath.Join(p.Boundary, "imported/package.bin")
			m, _, _, _ := verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), p)
			if e := StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
			old := SDState{Version: 1, Phase: phase, Digest: strings.Repeat("d", 64)}
			if e := retireVerifiedStage(p, old, fmt.Sprintf("%x", m.Digest)); e != nil {
				t.Fatal(e)
			}
			if e := StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestRetirementInterruptionRecovery(t *testing.T) {
	for _, removed := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(removed), func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			f := inputFile(t, s, embedTest(t, s))
			p := s.p
			p.Package = filepath.Join(p.Boundary, "imported/package.bin")
			m, _, _, _ := verifyEmbedded(f, s.key.Public().(ed25519.PublicKey), p)
			if e := StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
			dig := fmt.Sprintf("%x", m.Digest)
			ret := filepath.Join(p.Boundary, ".x6-single-retired-"+dig)
			os.Rename(filepath.Dir(p.Package), ret)
			names := []string{"package.bin", "package.bin.owner-auth", ".x6-import-receipt"}
			for i := 0; i < removed; i++ {
				os.Remove(filepath.Join(ret, names[i]))
			}
			old := SDState{Version: 1, Phase: "completed", Digest: dig}
			if e := retireVerifiedStage(p, old, strings.Repeat("a", 64)); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(ret); !os.IsNotExist(e) {
				t.Fatal("retired stage remains")
			}
			if e := StageSingle(f, p, nil); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestProductionDispatchLock(t *testing.T) {
	if e := dispatchNative(SDPolicy{AutomaticEnabled: false}, nil); e == nil {
		t.Fatal("disabled dispatch accepted")
	}
}
