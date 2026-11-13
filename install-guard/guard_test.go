package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(slot string) (Config, Evidence, *Snapshot) {
	var c Config
	if err := json.Unmarshal(configBytes, &c); err != nil {
		panic(err)
	}
	c.InstallAllowed = true
	e := Evidence{DataFreeBytes: c.MinimumDataFreeBytes + 1, UID: 0, Slot: slot, BootID: "boot-one", PublicSHA256: c.PublicSHA256, AbctlSHA256: c.AbctlSHA256, Devices: map[string]string{}, Sizes: map[string]int64{}, Hashes: map[string]string{}}
	e.Source = SourceEvidence{Version: "1.2.36", OTASHA256: c.Sources[1].OTASHA256, OwnerPublicSHA256: c.PublicSHA256, Preserved: map[string]string{"data/x6-owner-gui/server_ed25519.pub": e.PublicSHA256, "data/x6-owner-ssh/host_ed25519": sumBytes([]byte("fixture only"))}}
	for path, h := range c.OwnerTrust {
		e.Source.Preserved[path] = h
	}
	e.Source.Preserved["data/x6-owner-gui/server_ed25519"] = sumBytes([]byte("fixture GUI identity only"))
	s := &Snapshot{PackageID: packageID(c), Slot: slot, BootID: e.BootID, SourceID: sourceID(e), Hashes: map[string]string{}}
	payload := map[string]Part{}
	for _, p := range c.Targets {
		payload[p.Label] = p
	}
	target, _ := other(slot)
	for _, p := range c.Parts {
		e.Labels = append(e.Labels, p.Label)
		e.Devices[p.Label] = p.Device
		e.Sizes[p.Label] = p.Bytes
		if strings.HasSuffix(p.Label, slot) {
			e.Hashes[p.Label] = sumBytes([]byte(p.Label))
			s.Hashes[p.Label] = e.Hashes[p.Label]
		}
	}
	for _, p := range c.Parts {
		if strings.HasSuffix(p.Label, target) {
			base := strings.TrimSuffix(p.Label, target)
			h := s.Hashes[base+slot]
			if x, ok := payload[base]; ok {
				h = x.SHA256
			}
			e.Hashes["target:"+p.Label] = h
		}
	}
	return c, e, s
}
func TestBothDirections(t *testing.T) {
	for _, slot := range []string{"_a", "_b"} {
		for _, phase := range []string{"inspect", "pre", "post"} {
			t.Run(slot+"/"+phase, func(t *testing.T) {
				c, e, s := fixture(slot)
				if err := validate(c, e, phase, s); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestEvidenceRejections(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Evidence)
	}{
		{"unknown-slot", func(e *Evidence) { e.Slot = "" }}, {"non-root", func(e *Evidence) { e.UID = 1000 }}, {"wrong-camera", func(e *Evidence) { e.PublicSHA256 = "bad" }}, {"changed-abctl", func(e *Evidence) { e.AbctlSHA256 = "bad" }},
		{"unexpected-partition", func(e *Evidence) { e.Labels = append(e.Labels, "extra_a") }}, {"missing-partition", func(e *Evidence) { e.Labels = e.Labels[1:] }}, {"alias", func(e *Evidence) { e.Devices["system_b"] = e.Devices["system_a"] }},
		{"short-partition", func(e *Evidence) { e.Sizes["system_a"]-- }}, {"missing-source-hash", func(e *Evidence) { delete(e.Hashes, "system"+e.Slot) }}, {"bad-source-hash", func(e *Evidence) { e.Hashes["system"+e.Slot] = "bad" }}}
	for _, slot := range []string{"_a", "_b"} {
		for _, x := range cases {
			for _, phase := range []string{"inspect", "pre", "post"} {
				t.Run(slot+"/"+x.name+"/"+phase, func(t *testing.T) {
					c, e, s := fixture(slot)
					x.edit(&e)
					if validate(c, e, phase, s) == nil {
						t.Fatal("accepted")
					}
				})
			}
		}
	}
}
func TestPostIntegrity(t *testing.T) {
	for _, slot := range []string{"_a", "_b"} {
		t.Run(slot, func(t *testing.T) {
			c, e, s := fixture(slot)
			target, _ := other(slot)
			e.Hashes["target:system"+target] = sumBytes([]byte("bad"))
			if validate(c, e, "pre", nil) != nil {
				t.Fatal("pre inspected target contents")
			}
			if validate(c, e, "post", s) == nil {
				t.Fatal("bad payload accepted")
			}
			c, e, s = fixture(slot)
			e.Hashes["target:mdtp"+target] = sumBytes([]byte("bad copy"))
			if validate(c, e, "post", s) == nil {
				t.Fatal("copy-only corruption accepted")
			}
			c, e, s = fixture(slot)
			e.Hashes["system"+slot] = sumBytes([]byte("changed active slot"))
			if validate(c, e, "post", s) == nil {
				t.Fatal("active overwrite accepted")
			}
			c, e, s = fixture(slot)
			if validate(c, e, "post", nil) == nil {
				t.Fatal("missing snapshot")
			}
			s.BootID = "another"
			if validate(c, e, "post", s) == nil {
				t.Fatal("cross-boot snapshot")
			}
			c, e, s = fixture(slot)
			s.Slot = target
			if validate(c, e, "post", s) == nil {
				t.Fatal("cross-slot snapshot")
			}
			c, e, s = fixture(slot)
			s.PackageID = "other"
			if validate(c, e, "post", s) == nil {
				t.Fatal("cross-package snapshot")
			}
			if validate(c, e, "install", nil) == nil {
				t.Fatal("unknown phase")
			}
		})
	}
}
func TestPayloadRestrictions(t *testing.T) {
	for _, name := range []string{"active-suffix", "shared", "oversized", "bad-hash", "duplicate", "device-injection"} {
		t.Run(name, func(t *testing.T) {
			c, e, _ := fixture("_a")
			switch name {
			case "active-suffix":
				c.Targets[0].Label += "_a"
			case "shared":
				c.Targets[0].Label = "persist"
			case "oversized":
				c.Targets[0].CheckBytes = 1 << 50
			case "bad-hash":
				c.Targets[0].SHA256 = "bad"
			case "duplicate":
				c.Targets[0] = c.Targets[1]
			case "device-injection":
				c.Targets[0].Device = "/dev/block/active"
			}
			if validate(c, e, "pre", nil) == nil {
				t.Fatal("accepted invalid payload")
			}
		})
	}
}
func TestSnapshotFilesystem(t *testing.T) {
	for _, slot := range []string{"_a", "_b"} {
		t.Run(slot, func(t *testing.T) {
			c, e, _ := fixture(slot)
			dir := filepath.Join(t.TempDir(), "state")
			if err := saveSnapshot(dir, c, e); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(dir, "source.json"))
			s, err := readSnapshot(dir)
			if err != nil || validate(c, e, "post", s) != nil {
				t.Fatal(err)
			}
			if err = saveSnapshot(dir, c, e); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "source.json"))
			if string(before) != string(after) {
				t.Fatal("snapshot replaced")
			}
			e.Hashes["system"+slot] = sumBytes([]byte("changed"))
			if saveSnapshot(dir, c, e) == nil {
				t.Fatal("changed source reused snapshot")
			}
			after, _ = os.ReadFile(filepath.Join(dir, "source.json"))
			if string(before) != string(after) {
				t.Fatal("changed source overwrote snapshot")
			}
		})
	}
	for _, name := range []string{"directory-symlink", "file-symlink", "directory-mode", "file-mode", "hardlink", "truncated"} {
		t.Run(name, func(t *testing.T) {
			c, e, _ := fixture("_a")
			root := t.TempDir()
			dir := filepath.Join(root, "state")
			if err := saveSnapshot(dir, c, e); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "source.json")
			switch name {
			case "directory-symlink":
				os.Rename(dir, dir+".real")
				os.Symlink(dir+".real", dir)
			case "file-symlink":
				os.Rename(file, file+".real")
				os.Symlink(file+".real", file)
			case "directory-mode":
				os.Chmod(dir, 0777)
			case "file-mode":
				os.Chmod(file, 0644)
			case "hardlink":
				os.Link(file, file+".link")
			case "truncated":
				os.WriteFile(file, []byte("{"), 0600)
			}
			if _, err := readSnapshot(dir); err == nil {
				t.Fatal("unsafe snapshot accepted")
			}
		})
	}
}
func TestHashShortRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	os.WriteFile(p, []byte("abc"), 0600)
	if _, e := hashFile(p, 4); e == nil {
		t.Fatal("short read accepted")
	}
	if h, e := hashFile(p, 3); e != nil || h != sumBytes([]byte("abc")) {
		t.Fatal(h, e)
	}
}

func TestCandidateLocked(t *testing.T) {
	for _, slot := range []string{"_a", "_b"} {
		c, e, s := fixture(slot)
		c.InstallAllowed = false
		for _, phase := range []string{"pre", "post"} {
			if validate(c, e, phase, s) == nil {
				t.Fatal("lock bypass", phase)
			}
		}
	}
}

func TestSpaceBudget(t *testing.T) {
	for _, slot := range []string{"_a", "_b"} {
		for _, phase := range []string{"inspect", "pre", "post"} {
			c, e, s := fixture(slot)
			e.DataFreeBytes = c.MinimumDataFreeBytes
			if validate(c, e, phase, s) != nil {
				t.Fatal("exact boundary")
			}
			e.DataFreeBytes--
			if validate(c, e, phase, s) == nil {
				t.Fatal("low space accepted")
			}
			c.MinimumDataFreeBytes = 0
			if validate(c, e, phase, s) == nil {
				t.Fatal("missing space policy accepted")
			}
		}
	}
}
