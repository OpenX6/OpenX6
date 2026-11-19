package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stockConfig(t *testing.T) (Config, string) {
	t.Helper()
	var c Config
	if err := json.Unmarshal(configBytes, &c); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"data", "ins/etc", "usr/bin"} {
		if err = os.MkdirAll(filepath.Join(root, d), 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "ins/etc/version.txt"), []byte("qcs8625_Version:V0.8.827\n"), 0644)
	os.WriteFile(filepath.Join(root, "usr/bin/ins_otad"), []byte("stock updater fixture"), 0755)
	for i, r := range c.Sources {
		if r.Version == "0.8.827" {
			c.Sources[i].OTASHA256 = sumBytes([]byte("stock updater fixture"))
		}
	}
	return c, root
}
func TestStockReinstallLifecycle(t *testing.T) {
	for _, kind := range []string{"fresh", "empty-residue", "modified", "partial", "other-holder", "root-keys"} {
		t.Run(kind, func(t *testing.T) {
			c, root := stockConfig(t)
			if kind != "fresh" {
				os.MkdirAll(filepath.Join(root, "data/x6-owner-features"), 0700)
			}
			if kind == "modified" || kind == "partial" || kind == "other-holder" {
				for i, path := range featurePaths {
					if kind == "partial" && i%2 == 0 {
						continue
					}
					os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700)
					os.WriteFile(filepath.Join(root, path), []byte("locally modified "+path), 0600)
				}
				os.WriteFile(filepath.Join(root, "data/x6-owner-features/complete.json"), []byte("old or interrupted receipt"), 0600)
			}
			if kind == "other-holder" {
				files := map[string][]byte{}
				pins := map[string]string{}
				for role, b := range c.InitialKeys {
					b = bytes.Clone(b)
					if strings.HasSuffix(role, ".pub") {
						b[0] ^= 1
					}
					files[filepath.Base(role)] = b
					pins[filepath.Base(role)] = sumBytes(b)
				}
				files["pins.json"], _ = json.Marshal(pins)
				if err := publish(root, holderDir, files, false); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "root-keys" {
				for role, b := range c.InitialKeys {
					b = bytes.Clone(b)
					if strings.HasSuffix(role, ".pub") {
						b[0] ^= 1
					}
					path := filepath.Join(root, role)
					os.MkdirAll(filepath.Dir(path), 0700)
					os.WriteFile(path, b, 0600)
				}
			}
			before := map[string][]byte{}
			for _, path := range featurePaths {
				b, err := os.ReadFile(filepath.Join(root, path))
				if err == nil {
					before[path] = b
				}
			}
			run := func() Evidence {
				s, err := collectSource(root)
				if err != nil {
					t.Fatal(err)
				}
				e := Evidence{Source: s}
				if err = collectPrograms(root, c, &e); err != nil {
					t.Fatal(err)
				}
				if err = validateSource(c, e); err != nil {
					t.Fatal(err)
				}
				return e
			}
			e := run()
			id := sourceID(e)
			if err := inherit(root, c, e, false); err != nil {
				t.Fatal(err)
			}
			after := run()
			if sourceID(after) != id {
				t.Fatal("pre/post source evidence changed")
			}
			if err := inherit(root, c, after, true); err != nil {
				t.Fatal(err)
			}
			for path, b := range before {
				now, _ := os.ReadFile(filepath.Join(root, path))
				if !bytes.Equal(b, now) {
					t.Fatal("old slot mutated", path)
				}
			}
			if kind != "fresh" {
				if after.Source.Preserved["stock-reinstall-plan"] == "" {
					t.Fatal("missing recovery handoff")
				}
			}
			// Historical layout count and content do not govern stock reinstall.
			c.FeatureSets = nil
			if err := collectPrograms(root, c, &after); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestStockReinstallRejectsUnsafeResidue(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "bad-pref", "changed-post"} {
		t.Run(kind, func(t *testing.T) {
			c, r := stockConfig(t)
			dir := filepath.Join(r, "data/x6-owner-features/vr180-v1")
			os.MkdirAll(dir, 0700)
			switch kind {
			case "symlink":
				os.Symlink("/etc/passwd", dir+"/gui")
			case "hardlink":
				os.Link(filepath.Join(r, "usr/bin/ins_otad"), dir+"/gui")
			case "bad-pref":
				os.WriteFile(dir+"/sync", []byte{255}, 0600)
			}
			s, _ := collectSource(r)
			e := Evidence{Source: s}
			err := collectPrograms(r, c, &e)
			if kind != "changed-post" {
				if err == nil {
					t.Fatal("unsafe input accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(dir+"/gui", []byte("new mutation"), 0600)
			if inherit(r, c, e, false) == nil {
				t.Fatal("changed preflight accepted")
			}
		})
	}
}
