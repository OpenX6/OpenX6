package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The runtime preference helpers (x6-runtime-prepare and the controllers) may
// create data/x6-owner-features before this seed publishes on a fresh install.
// These are exactly the directories the new unit had on 2026-11-02 when the
// previous engine refused to publish.
var foreignPrefs = map[string][]byte{
	"stats-v1/enabled.pref":             {0},
	"preview-v1/disabled":               {0},
	"custom-bitrate-v1/value":           {4, 1, 0, 0}, // 260 Mbps, little-endian
	"interval-controls-v1/shutter.pref": {0},
}
var foreignDirs = []string{"ssd-v1"}

func placeForeign(t *testing.T, e Engine) {
	t.Helper()
	for rel, b := range foreignPrefs {
		put(t, e.path(featurePath+"/"+rel), b, 0600)
		if err := os.Chmod(filepath.Dir(e.path(featurePath+"/"+rel)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range foreignDirs {
		if err := os.MkdirAll(e.path(featurePath+"/"+d), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(e.path(featurePath), 0700); err != nil {
		t.Fatal(err)
	}
}

func checkMerged(t *testing.T, f testFixture) {
	t.Helper()
	if err := f.e.verifyFinal(f.m); err != nil {
		t.Fatal("published tree", err)
	}
	b, err := os.ReadFile(f.e.path(featurePath + "/complete.json"))
	if err != nil || string(b) != f.e.Pin+"\n" {
		t.Fatal("receipt", err)
	}
	for rel, want := range foreignPrefs {
		b, err := os.ReadFile(f.e.path(featurePath + "/" + rel))
		if err != nil || string(b) != string(want) {
			t.Fatal("foreign preference changed", rel, err)
		}
	}
	for _, d := range foreignDirs {
		if st, err := os.Lstat(f.e.path(featurePath + "/" + d)); err != nil || !st.IsDir() {
			t.Fatal("foreign directory lost", d)
		}
	}
	if _, err := os.Lstat(f.e.path("data/.x6-firstboot-stage-" + f.e.Pin)); !os.IsNotExist(err) {
		t.Fatal("staging tree left behind")
	}
	for rel, want := range defaults {
		b, err := os.ReadFile(f.e.path(featurePath + "/" + rel))
		if err != nil || len(b) != 1 || b[0] != want {
			t.Fatal("default preference", rel)
		}
	}
	untouched(t, f.e)
}

func TestForeignNamespaceBeforeSeed(t *testing.T) {
	f := fixture(t)
	placeForeign(t, f.e)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	checkMerged(t, f)
	// Later runs keep saved choices, exactly like a normal installation.
	put(t, f.e.path(featurePath+"/vr180-v1/sync"), []byte{1}, 0600)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f.e.path(featurePath + "/vr180-v1/sync")); len(b) != 1 || b[0] != 1 {
		t.Fatal("preference overwritten")
	}
	checkMergedForeignOnly(t, f)
}

func checkMergedForeignOnly(t *testing.T, f testFixture) {
	t.Helper()
	for rel, want := range foreignPrefs {
		b, err := os.ReadFile(f.e.path(featurePath + "/" + rel))
		if err != nil || string(b) != string(want) {
			t.Fatal("foreign preference changed on repeat", rel, err)
		}
	}
}

func TestForeignNamespaceAppearsDuringStaging(t *testing.T) {
	f := fixture(t)
	f.e.Fault = func(p string) error {
		if p == "before-publish" {
			placeForeign(t, f.e)
		}
		return nil
	}
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	checkMerged(t, f)
}

// The 2026-11-02 sequence: the first run staged everything, found the
// namespace and stopped; every retry then met an unreceipted namespace.
func TestOldRefusalSequenceNowPublishes(t *testing.T) {
	f := fixture(t)
	appeared := false
	f.e.Fault = func(p string) error {
		if p == "before-publish" && !appeared {
			appeared = true
			placeForeign(t, f.e)
			return errors.New("first run ends here")
		}
		return nil
	}
	if f.e.Run() == nil || !appeared {
		t.Fatal("first run should stop at the injected point")
	}
	if _, err := os.Lstat(f.e.path(featurePath + "/complete.json")); err == nil {
		t.Fatal("receipt written by the interrupted run")
	}
	if err := f.e.Run(); err != nil {
		t.Fatal("retry", err)
	}
	checkMerged(t, f)
}

func TestMergeCheckpointsRecover(t *testing.T) {
	points := []string{}
	f := fixture(t)
	placeForeign(t, f.e)
	f.e.Fault = func(p string) error { points = append(points, p); return nil }
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	merged := false
	for _, p := range points {
		if strings.HasPrefix(p, "merged:") {
			merged = true
		}
	}
	if !merged {
		t.Fatal("merge path not exercised")
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			f := fixture(t)
			placeForeign(t, f.e)
			hit := false
			f.e.Fault = func(p string) error {
				if p == point {
					hit = true
					return errors.New("simulated abrupt stop")
				}
				return nil
			}
			if f.e.Run() == nil || !hit {
				t.Fatal("fault not reached")
			}
			// A receipt is only ever visible together with every payload.
			if _, err := os.Lstat(f.e.path(featurePath + "/complete.json")); err == nil {
				if err = f.e.verifyFinal(f.m); err != nil {
					t.Fatal("receipt without payload", err)
				}
			}
			untouched(t, f.e)
			f.e.Fault = nil
			if err := f.e.Run(); err != nil {
				t.Fatal("resume", err)
			}
			checkMerged(t, f)
		})
	}
	t.Logf("%d merge checkpoints recovered", len(points))
}

func TestForeignProcessKillAndResume(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []string{"stage", "synced:vr180-v1/gui", "before-publish", "merged:vr180-v1", "merged:raw-nr-v1", "published", "stage-removed"} {
		t.Run(point, func(t *testing.T) {
			f := fixture(t)
			cmd := exec.Command(exe, "-test.run=^TestCrashWorker$")
			cmd.Env = append(os.Environ(), "X6_FIXTURE_WORKER=1", "X6_FIXTURE_FOREIGN=1", "X6_FIXTURE_ROOT="+f.e.Root, "X6_FIXTURE_PIN="+f.e.Pin, "X6_FIXTURE_KILL_AT="+point)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(err.Error(), "signal: killed") {
				t.Fatalf("not SIGKILL: %v %s", err, output)
			}
			if _, err := os.Lstat(f.e.path(featurePath + "/complete.json")); err == nil {
				if err = f.e.verifyFinal(f.m); err != nil {
					t.Fatal("receipt without payload", err)
				}
			}
			if err = f.e.Run(); err != nil {
				t.Fatal("resume after kill", err)
			}
			checkMerged(t, f)
		})
	}
}

// Anything that is not just other components' directories keeps the old
// refusal: adoption/migration decide, nothing is published or deleted.
func TestUnexpectedNamespaceContentStillRefused(t *testing.T) {
	cases := map[string]func(e Engine){
		"stray-file": func(e Engine) {
			os.MkdirAll(e.path(featurePath), 0700)
			os.WriteFile(e.path(featurePath+"/stray"), []byte("x"), 0600)
		},
		"temporary-receipt": func(e Engine) {
			os.MkdirAll(e.path(featurePath), 0700)
			os.WriteFile(e.path(featurePath+"/.firstboot-receipt-abc"), []byte("x"), 0600)
		},
		"our-directory-with-other-content": func(e Engine) {
			os.MkdirAll(e.path(featurePath+"/vr180-v1"), 0700)
			os.WriteFile(e.path(featurePath+"/vr180-v1/gui"), []byte("not ours"), 0755)
		},
		"world-writable-namespace": func(e Engine) {
			os.MkdirAll(e.path(featurePath+"/stats-v1"), 0700)
			os.Chmod(e.path(featurePath), 0777)
		},
	}
	for name, place := range cases {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			place(f.e)
			if err := f.e.Run(); err == nil {
				t.Fatal("unexpected content accepted")
			}
			if _, err := os.Lstat(f.e.path(featurePath + "/complete.json")); err == nil {
				t.Fatal("receipt written over unknown content")
			}
			if _, err := os.Lstat(f.e.path("data/.x6-firstboot-stage-" + f.e.Pin)); err == nil {
				t.Fatal("staging started for unknown content")
			}
			untouched(t, f.e)
		})
	}
}

func TestStaleStagingTreeRemovedOnceReceipted(t *testing.T) {
	f := fixture(t)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	stage := "data/.x6-firstboot-stage-" + f.e.Pin
	put(t, f.e.path(stage+"/complete.json"), []byte(f.e.Pin+"\n"), 0600)
	os.Chmod(f.e.path(stage), 0700)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f.e.path(stage)); !os.IsNotExist(err) {
		t.Fatal("stale staging tree kept")
	}
	// A tree under the staging name that holds anything else is never removed.
	put(t, f.e.path(stage+"/not-ours"), []byte("x"), 0600)
	os.Chmod(f.e.path(stage), 0700)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f.e.path(stage + "/not-ours")); err != nil {
		t.Fatal("unknown tree removed")
	}
	untouched(t, f.e)
}
