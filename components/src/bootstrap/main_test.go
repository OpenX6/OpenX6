package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCrashWorker(t *testing.T) {
	if os.Getenv("X6_FIXTURE_WORKER") != "1" {
		return
	}
	e := Engine{Root: os.Getenv("X6_FIXTURE_ROOT"), Pin: os.Getenv("X6_FIXTURE_PIN"), Free: func(string) (uint64, error) { return 1 << 40, nil }}
	if os.Getenv("X6_FIXTURE_FOREIGN") == "1" {
		placeForeign(t, e) // another component created the namespace first
	}
	e.Fault = func(point string) error {
		if point == os.Getenv("X6_FIXTURE_KILL_AT") {
			p, _ := os.FindProcess(os.Getpid())
			_ = p.Kill()
			select {}
		}
		return nil
	}
	if err := e.Run(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("expected kill point was not reached")
}
func TestActualProcessKillAndResume(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []string{"stage", "base:raw-nr-v1/low.so", "chunk:raw-nr-v1/low.so:0", "synced:vr180-v1/gui", "file:vr180-v1/enabled", "file:complete.json", "before-publish", "after-rename", "published"} {
		t.Run(point, func(t *testing.T) {
			f := fixture(t)
			cmd := exec.Command(exe, "-test.run=^TestCrashWorker$")
			cmd.Env = append(os.Environ(), "X6_FIXTURE_WORKER=1", "X6_FIXTURE_ROOT="+f.e.Root, "X6_FIXTURE_PIN="+f.e.Pin, "X6_FIXTURE_KILL_AT="+point)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(err.Error(), "signal: killed") {
				t.Fatalf("not SIGKILL: %v %s", err, output)
			}
			if _, err := os.Lstat(f.e.path(featurePath)); err == nil {
				if err = f.e.verifyFinal(f.m); err != nil {
					t.Fatal("partial publish", err)
				}
			}
			if err = f.e.Run(); err != nil {
				t.Fatal("resume after kill", err)
			}
			untouched(t, f.e)
		})
	}
}

type testFixture struct {
	e    Engine
	m    Manifest
	blob []byte
}

func put(t *testing.T, p string, b []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) testFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Root: root, Free: func(string) (uint64, error) { return 1 << 40, nil }}
	put(t, e.path("data/stock-calibration"), []byte("preserve factory data"), 0600)
	for _, p := range []string{"data/x6-owner-ssh/host_ed25519", "data/x6-owner-gui/server_ed25519", "etc/stock-settings"} {
		put(t, e.path(p), []byte("untouched"), 0600)
	}
	put(t, e.path(".x6-offline-fixture"), []byte("OFFLINE TEST ROOT\n"), 0600)
	m := Manifest{Version: 1}
	blob := []byte{}
	keys := []string{}
	for k := range policies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := policies[k]
		base := bytes.Repeat([]byte{42}, 8192)
		put(t, e.path(p.base), base, 0644)
		target := append([]byte{}, base...)
		v := Entry{Target: k, Base: p.base, BaseSHA256: digest(base), BaseBytes: int64(len(base)), Mode: p.mode}
		if !strings.Contains(k, "/backup/") {
			target[4100] = byte(len(m.Entries) + 1)
			v.Chunks = []Chunk{{4096, int64(len(blob)), 4096}}
			blob = append(blob, target[4096:]...)
		}
		v.Bytes = int64(len(target))
		v.SHA256 = digest(target)
		m.Entries = append(m.Entries, v)
	}
	m.BlobBytes = int64(len(blob))
	m.BlobSHA256 = digest(blob)
	f := testFixture{e, m, blob}
	f.save(t)
	return f
}
func (f *testFixture) save(t *testing.T) {
	t.Helper()
	b, err := json.Marshal(f.m)
	if err != nil {
		t.Fatal(err)
	}
	f.e.Pin = digest(b)
	put(t, f.e.path(seedPath+"/manifest.json"), b, 0644)
	put(t, f.e.path(seedPath+"/delta.bin"), f.blob, 0644)
}
func untouched(t *testing.T, e Engine) {
	t.Helper()
	for _, p := range []string{"data/x6-owner-ssh/host_ed25519", "data/x6-owner-gui/server_ed25519", "etc/stock-settings"} {
		b, err := os.ReadFile(e.path(p))
		if err != nil || string(b) != "untouched" {
			t.Fatal("protected file changed", p)
		}
	}
	b, err := os.ReadFile(e.path("data/stock-calibration"))
	if err != nil || string(b) != "preserve factory data" {
		t.Fatal("factory data changed")
	}
}
func TestInitializeAndRepeat(t *testing.T) {
	f := fixture(t)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	put(t, f.e.path(featurePath+"/vr180-v1/sync"), []byte{1}, 0600)
	put(t, f.e.path(featurePath+"/raw-nr-v1/level"), []byte{2}, 0600)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]byte{"vr180-v1/sync": 1, "raw-nr-v1/level": 2} {
		b, _ := os.ReadFile(f.e.path(featurePath + "/" + p))
		if len(b) != 1 || b[0] != want {
			t.Fatal("preference overwritten")
		}
	}
	untouched(t, f.e)
}
func TestEveryCheckpointRecovers(t *testing.T) {
	f := fixture(t)
	points := []string{}
	f.e.Fault = func(p string) error { points = append(points, p); return nil }
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			f := fixture(t)
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
			if _, err := os.Lstat(f.e.path(featurePath)); err == nil {
				if err = f.e.verifyFinal(f.m); err != nil {
					t.Fatal("partial tree visible", err)
				}
			}
			untouched(t, f.e)
			f.e.Fault = nil
			if err := f.e.Run(); err != nil {
				t.Fatal("resume", err)
			}
			if err := f.e.verifyFinal(f.m); err != nil {
				t.Fatal(err)
			}
			untouched(t, f.e)
		})
	}
	t.Logf("%d interruption checkpoints recovered", len(points))
}
func TestRejectManifest(t *testing.T) {
	cases := map[string]func(*testFixture){
		"version":           func(f *testFixture) { f.m.Version = 2 },
		"path traversal":    func(f *testFixture) { f.m.Entries[0].Target = "../escape" },
		"base traversal":    func(f *testFixture) { f.m.Entries[0].Base = "../source" },
		"duplicate target":  func(f *testFixture) { f.m.Entries[1] = f.m.Entries[0] },
		"extra entry":       func(f *testFixture) { f.m.Entries = append(f.m.Entries, f.m.Entries[0]) },
		"output too large":  func(f *testFixture) { f.m.Entries[0].Bytes = maxFile + 1 },
		"negative size":     func(f *testFixture) { f.m.Entries[0].Bytes = -1 },
		"bad mode":          func(f *testFixture) { f.m.Entries[0].Mode = 0777 },
		"malformed hash":    func(f *testFixture) { f.m.Entries[0].SHA256 = "bad" },
		"wrong output hash": func(f *testFixture) { f.m.Entries[0].SHA256 = strings.Repeat("0", 64) },
		"wrong base hash":   func(f *testFixture) { f.m.Entries[0].BaseSHA256 = strings.Repeat("0", 64) },
		"offset overflow":   func(f *testFixture) { f.m.Entries[0].Chunks[0].Offset = 1 << 62 },
		"negative offset":   func(f *testFixture) { f.m.Entries[0].Chunks[0].Offset = -4096 },
		"blob overflow":     func(f *testFixture) { f.m.Entries[0].Chunks[0].BlobOffset = 1 << 62 },
		"short interior":    func(f *testFixture) { f.m.Entries[0].Chunks[0].Length = 2 },
		"chunk overlap":     func(f *testFixture) { f.m.Entries[0].Chunks = append(f.m.Entries[0].Chunks, f.m.Entries[0].Chunks[0]) },
		"trailing blob":     func(f *testFixture) { f.blob = append(f.blob, 0); f.m.BlobBytes++; f.m.BlobSHA256 = digest(f.blob) },
		"wrong blob hash":   func(f *testFixture) { f.m.BlobSHA256 = strings.Repeat("0", 64) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			change(&f)
			f.save(t)
			if f.e.Run() == nil {
				t.Fatal("accepted malformed input")
			}
			if _, err := os.Lstat(f.e.path(featurePath)); !os.IsNotExist(err) {
				t.Fatal("published rejected input")
			}
			untouched(t, f.e)
		})
	}
}
func TestRejectFilesystem(t *testing.T) {
	cases := map[string]func(*testing.T, *testFixture){
		"unpinned manifest": func(t *testing.T, f *testFixture) { f.e.Pin = strings.Repeat("0", 64) },
		"truncated blob":    func(t *testing.T, f *testFixture) { os.Truncate(f.e.path(seedPath+"/delta.bin"), 1) },
		"wrong base bytes":  func(t *testing.T, f *testFixture) { put(t, f.e.path(f.m.Entries[0].Base), []byte("bad"), 0644) },
		"source symlink": func(t *testing.T, f *testFixture) {
			p := f.e.path(f.m.Entries[0].Base)
			os.Remove(p)
			os.Symlink(f.e.path("data/stock-calibration"), p)
		},
		"source hardlink": func(t *testing.T, f *testFixture) {
			p := f.e.path(f.m.Entries[0].Base)
			os.Link(p, f.e.path("second-link"))
		},
		"seed writable": func(t *testing.T, f *testFixture) { os.Chmod(f.e.path(seedPath+"/manifest.json"), 0666) },
		"data symlink": func(t *testing.T, f *testFixture) {
			os.Rename(f.e.path("data"), f.e.path("saved-data"))
			os.Symlink(f.e.path("saved-data"), f.e.path("data"))
		},
		"data writable":              func(t *testing.T, f *testFixture) { os.Chmod(f.e.path("data"), 0777) },
		"existing unknown namespace": func(t *testing.T, f *testFixture) { put(t, f.e.path(featurePath+"/unknown"), []byte("keep me"), 0600) },
		"target symlink":             func(t *testing.T, f *testFixture) { os.Symlink(f.e.path("data/x6-owner-ssh"), f.e.path(featurePath)) },
		"stage symlink": func(t *testing.T, f *testFixture) {
			os.Symlink(f.e.path("data/x6-owner-ssh"), f.e.path("data/.x6-firstboot-stage-"+f.e.Pin))
		},
		"unknown stage file": func(t *testing.T, f *testFixture) {
			put(t, f.e.path("data/.x6-firstboot-stage-"+f.e.Pin+"/unknown"), []byte("keep"), 0600)
		},
		"stage temp hardlink": func(t *testing.T, f *testFixture) {
			p := f.e.path("data/.x6-firstboot-stage-" + f.e.Pin)
			os.Mkdir(p, 0700)
			os.Link(f.e.path("data/stock-calibration"), p+"/.writing")
		},
		"space exhausted": func(t *testing.T, f *testFixture) { f.e.Free = func(string) (uint64, error) { return 0, nil } },
		"space query fails": func(t *testing.T, f *testFixture) {
			f.e.Free = func(string) (uint64, error) { return 0, errors.New("statfs failed") }
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			change(t, &f)
			if f.e.Run() == nil {
				t.Fatal("accepted unsafe filesystem")
			}
			untouched(t, f.e)
		})
	}
}
func TestFinalTamperAndConcurrentLock(t *testing.T) {
	for _, kind := range []string{"marker", "payload", "preference", "lock"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			if kind == "lock" {
				release, err := f.e.lock()
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				if f.e.Run() == nil {
					t.Fatal("concurrent lock ignored")
				}
				return
			}
			if err := f.e.Run(); err != nil {
				t.Fatal(err)
			}
			rel := map[string]string{"marker": "complete.json", "payload": "vr180-v1/gui", "preference": "raw-nr-v1/level"}[kind]
			p := f.e.path(featurePath + "/" + rel)
			b := []byte{255}
			put(t, p, b, 0600)
			if f.e.Run() == nil {
				t.Fatal("tamper accepted")
			}
			after, _ := os.ReadFile(p)
			if !bytes.Equal(after, b) {
				t.Fatal("silently repaired unknown state")
			}
			untouched(t, f.e)
		})
	}
}
func TestMalformedJSON(t *testing.T) {
	f := fixture(t)
	for _, b := range [][]byte{[]byte(`{"Version":1,"Unexpected":true}`), []byte(`{} {}`)} {
		put(t, f.e.path(seedPath+"/manifest.json"), b, 0644)
		f.e.Pin = digest(b)
		if f.e.Run() == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
func Example_validate() {
	fmt.Println("Pinned manifest + exact base + bounded chunks + exact output; publish one complete directory") // Output: Pinned manifest + exact base + bounded chunks + exact output; publish one complete directory
}
