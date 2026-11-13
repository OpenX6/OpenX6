package main

import (
	"os"
	"path/filepath"
	"testing"
)

func featureFixture(t *testing.T) (string, []map[string]string) {
	t.Helper()
	r := t.TempDir()
	sets := []map[string]string{{}, {}}
	for i := range sets {
		for _, p := range featurePaths {
			b := []byte(p + string(rune('A'+i)))
			sets[i][p] = sumBytes(b)
			if i == 0 {
				q := filepath.Join(r, p)
				os.MkdirAll(filepath.Dir(q), 0700)
				os.WriteFile(q, b, 0600)
			}
		}
	}
	for p, v := range featurePreferences {
		os.WriteFile(filepath.Join(r, p), []byte{v[0]}, 0600)
	}
	return r, sets
}
func TestKnownLayoutsAndRejections(t *testing.T) {
	for _, kind := range []string{"old", "new", "mixed", "unknown", "missing", "symlink", "parent-symlink", "hardlink", "writable", "ambiguous", "empty"} {
		t.Run(kind, func(t *testing.T) {
			r, s := featureFixture(t)
			p := filepath.Join(r, featurePaths[0])
			switch kind {
			case "new":
				for _, rel := range featurePaths {
					os.WriteFile(filepath.Join(r, rel), []byte(rel+"B"), 0600)
				}
			case "mixed":
				os.WriteFile(p, []byte(featurePaths[0]+"B"), 0600)
			case "unknown":
				os.WriteFile(p, []byte("unknown"), 0600)
			case "missing":
				os.Remove(p)
			case "symlink":
				os.Rename(p, p+".real")
				os.Symlink(p+".real", p)
			case "parent-symlink":
				d := filepath.Dir(p)
				os.Rename(d, d+".real")
				os.Symlink(d+".real", d)
			case "hardlink":
				os.Link(p, p+".hard")
			case "writable":
				os.Chmod(p, 0666)
			case "ambiguous":
				s = append(s, s[0])
			case "empty":
				s = nil
			}
			got, e := checkFeatureFilesAt(r, s)
			if (e == nil) != (kind == "old" || kind == "new") {
				t.Fatal(kind, e)
			}
			if e == nil && len(got) != 9 {
				t.Fatal(got)
			}
		})
	}
}
func TestPreferences(t *testing.T) {
	r, _ := featureFixture(t)
	p := filepath.Join(r, "data/x6-owner-features/raw-nr-v1/level")
	for i := 0; i < 256; i++ {
		os.WriteFile(p, []byte{byte(i)}, 0600)
		if (checkFeaturePreferencesAt(r) == nil) != (i < 4) {
			t.Fatal(i)
		}
	}
	os.WriteFile(p, []byte{0, 1}, 0600)
	if checkFeaturePreferencesAt(r) == nil {
		t.Fatal("length")
	}
}

func TestDisabledFeatureFlag(t *testing.T) {
	r, _ := featureFixture(t)
	for _, p := range []string{"data/x6-owner-features/vr180-v1/enabled", "data/x6-owner-features/raw-nr-v1/enabled"} {
		if err := os.Remove(filepath.Join(r, p)); err != nil {
			t.Fatal(err)
		}
	}
	if e := checkFeaturePreferencesAt(r); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(r, "data/x6-owner-features/vr180-v1/enabled"), []byte("0"), 0600)
	if checkFeaturePreferencesAt(r) == nil {
		t.Fatal("malformed enable flag accepted")
	}
}
