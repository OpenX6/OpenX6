package main

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompletionUsesRealMaintenanceProbeNotGUI(t *testing.T) {
	const helper = "/usr/local/sbin/x6-owner-maintenance"
	for _, bad := range []string{"", "holder", "pid", "show", "uid", "status", "running-binary", "disk-binary", "disk-error", "probe"} {
		t.Run("case-"+bad, func(t *testing.T) {
			cfg := SDPolicy{Files: map[string][]string{helper: {"trusted"}, "/usr/bin/ins_gui": {"OLD-GUI"}}}
			probes := 0
			command := func(sec int, args ...string) ([]byte, error) {
				switch args[0] {
				case "/bin/systemctl":
					if !reflect.DeepEqual(args, []string{"/bin/systemctl", "show", "x6-owner-maintenance.service", "-p", "MainPID", "--value"}) {
						t.Fatal(args)
					}
					if bad == "show" {
						return nil, errors.New("show")
					}
					if bad == "pid" {
						return []byte("0"), nil
					}
					return []byte("123\n"), nil
				case helper:
					probes++
					if !reflect.DeepEqual(args, []string{helper, "--probe-root", "--listen", "127.0.0.1:2222"}) || sec != 6 {
						t.Fatal(args, sec)
					}
					if bad == "probe" {
						return nil, errors.New("wrong key or not root")
					}
					return nil, nil
				default:
					t.Fatal("unexpected GUI or other command", args)
					return nil, nil
				}
			}
			read := func(p string) ([]byte, error) {
				if p != "/proc/123/status" {
					t.Fatal("GUI dependency", p)
				}
				if bad == "status" {
					return nil, errors.New("gone")
				}
				if bad == "uid" {
					return []byte("Uid:\t0\t1000\t0\t0\n"), nil
				}
				return []byte("Uid:\t0\t0\t0\t0\n"), nil
			}
			digest := func(p string) (string, error) {
				if p != helper {
					t.Fatal("GUI digest dependency", p)
				}
				if bad == "disk-error" {
					return "", errors.New("gone")
				}
				if bad == "disk-binary" {
					return "changed", nil
				}
				return "trusted", nil
			}
			matches := func(pid int, p string, _ SDPolicy) bool {
				if pid != 123 || p != helper {
					t.Fatal(pid, p)
				}
				return bad != "running-binary"
			}
			got := completionRootReady(cfg, bad != "holder", command, read, digest, matches)
			if got != (bad == "") {
				t.Fatal(bad, got)
			}
			if bad != "" && bad != "probe" && probes != 0 {
				t.Fatal("ran helper before validating", bad)
			}
		})
	}
}
func TestVisibleUpdateWithConsumedPredecessor(t *testing.T) {
	for _, kind := range []string{"same", "different", "corrupt", "symlink", "replaced-after-validation"} {
		t.Run(kind, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			pub := s.key.Public().(ed25519.PublicKey)
			dir := t.TempDir()
			visible := filepath.Join(dir, "Insta360X6FW.bin")
			hidden := filepath.Join(dir, ".x6-owner-package.bin")
			body := embedTest(t, s)
			if e := os.WriteFile(visible, body, 0600); e != nil {
				t.Fatal(e)
			}
			old := append([]byte(nil), body...)
			if kind == "different" {
				other := makeSetup(t, "1.2.36", false)
				other.key = s.key
				other.b[5000] ^= 1
				repairPrefix(other.b)
				old = embedTest(t, other)
			}
			if kind == "corrupt" {
				old = []byte("not firmware")
			}
			if kind == "symlink" {
				if e := os.Symlink(visible, hidden); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := os.WriteFile(hidden, old, 0600); e != nil {
					t.Fatal(e)
				}
			}
			f, path, e := openCardPackageAt(visible, hidden)
			if kind == "symlink" {
				if e == nil {
					f.Close()
					t.Fatal("followed symlink")
				}
				return
			}
			if e != nil || path != visible {
				t.Fatal(path, e)
			}
			defer f.Close()
			if _, _, _, e = inspectCard(f, pub, s.p); e != nil {
				t.Fatal(e)
			}
			prior, e := validatedConsumed(hidden, pub, s.p)
			if kind == "corrupt" {
				if e == nil {
					t.Fatal("unvalidated predecessor")
				}
				b, _ := os.ReadFile(hidden)
				if string(b) != string(old) {
					t.Fatal("deleted bad input")
				}
				return
			}
			if e != nil || prior == nil {
				t.Fatal(e)
			}
			// Validation is read-only; replacement occurs only at the consume boundary.
			b, _ := os.ReadFile(hidden)
			if string(b) != string(old) {
				t.Fatal("early mutation")
			}
			if kind == "replaced-after-validation" {
				replacement := filepath.Join(dir, "replacement")
				os.WriteFile(replacement, []byte("user-file"), 0600)
				os.Rename(replacement, hidden)
			}
			e = consumeCardNameAt(f, path, prior, visible, hidden)
			if kind == "replaced-after-validation" {
				if e == nil {
					t.Fatal("replaced a different inode")
				}
				b, _ = os.ReadFile(hidden)
				if string(b) != "user-file" {
					t.Fatal("replacement touched")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			b, _ = os.ReadFile(hidden)
			if string(b) != string(body) {
				t.Fatal("new input missing")
			}
		})
	}
}
func TestCompletedCleanupIndependentOfVisibleInput(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	pub := s.key.Public().(ed25519.PublicKey)
	dir := t.TempDir()
	visible := filepath.Join(dir, "Insta360X6FW.bin")
	hidden := filepath.Join(dir, ".x6-owner-package.bin")
	body := embedTest(t, s)
	f := inputFile(t, s, body)
	m, _, _, e := inspectCard(f, pub, s.p)
	if e != nil {
		t.Fatal(e)
	}
	dig := fmt.Sprintf("%x", m.Digest)
	state := SDState{Phase: "completed", Digest: dig}
	for _, bad := range []string{"pending", "not-cleaned", "different-digest", ""} {
		os.WriteFile(visible, []byte("owner new input"), 0600)
		os.WriteFile(hidden, body, 0600)
		st := state
		cleaned := dig
		if bad == "pending" {
			st.Phase = "dispatched"
		}
		if bad == "not-cleaned" {
			cleaned = ""
		}
		if bad == "different-digest" {
			st.Digest = strings.Repeat("f", 64)
			cleaned = st.Digest
		}
		e := cleanupConsumedCompleted(hidden, st, cleaned, pub, s.p)
		if bad == "" {
			if e != nil {
				t.Fatal(e)
			}
			if _, e = os.Stat(hidden); !os.IsNotExist(e) {
				t.Fatal("not cleaned")
			}
		} else {
			if _, e = os.Stat(hidden); e != nil {
				t.Fatal("unsafe removal", bad)
			}
		}
		b, _ := os.ReadFile(visible)
		if string(b) != "owner new input" {
			t.Fatal("visible touched")
		}
	}
	// Retry after a crash immediately following unlink is harmless.
	if e := cleanupConsumedCompleted(hidden, state, dig, pub, s.p); e != nil {
		t.Fatal(e)
	}
}
func TestUpdatedGUIPendingInstallCompletesOnce(t *testing.T) {
	c, o, n := sdFixture()
	o.Identity = strings.Repeat("e", 64)
	c.Step(o)
	o.Boot = "new-boot"
	o.Slot = "_b"
	o.CurrentRelease = o.PackageRelease
	o.CompletionVerified = true
	if status, e := c.Step(o); e != nil || status != "completed" {
		t.Fatal(status, e)
	}
	for i := 0; i < 3; i++ {
		if status, e := c.Step(o); e != nil || status != "left-card-suppressed" {
			t.Fatal(status, e)
		}
	}
	if *n != 1 {
		t.Fatal("repeated installation")
	}
}
