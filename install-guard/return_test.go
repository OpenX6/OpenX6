package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func putRecord(t *testing.T, root, rel string, b []byte) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func recordBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func samplePred(id string) *predecessor {
	p := &predecessor{Seed: sumBytes([]byte(id)), Version: 1, Owners: map[string]string{}}
	for _, n := range roles {
		p.Owners[n] = sumBytes([]byte(n))
	}
	for _, n := range featurePaths {
		p.Entries = append(p.Entries, entry{Target: n[len("data/x6-owner-features/"):], SHA256: sumBytes([]byte(n)), Mode: 0755, Bytes: 100})
	}
	return p
}
func TestReturningTarget(t *testing.T) {
	for _, c := range []string{"completed", "identical-previous", "no-journal", "partial-rotation", "retired-full", "retired-tx-only", "retired-empty", "incomplete", "foreign", "corrupt", "pending-other", "owner-changed", "post-no-refresh", "journal-symlink", "no-receipt", "post-pending"} {
		t.Run(c, func(t *testing.T) {
			r := t.TempDir()
			old := samplePred("origin")
			fresh := samplePred("next")
			target := sumBytes([]byte("target"))
			if c == "identical-previous" {
				old = fresh
			}
			dir := "data/x6-inherit-" + target
			live := "data/.x6-firstboot-migrate-" + target
			dead := "data/.x6-retired-journal-" + target
			putRecord(t, r, dir+"/previous.json", recordBytes(t, old))
			j := live
			if c == "retired-full" || c == "retired-tx-only" || c == "retired-empty" {
				j = dead
			}
			tx := recordBytes(t, completedTransaction{1, old.Seed, target, old.Seed + "\n", "", false})
			ok := true
			if c != "no-journal" {
				putRecord(t, r, j+"/transaction.json", tx)
				putRecord(t, r, j+"/cleanup.ready", []byte(sumBytes(tx)+"\n"+target+"\n"))
			}
			switch c {
			case "partial-rotation":
				putRecord(t, r, dir+"/.previous.next", recordBytes(t, fresh)[:17])
			case "retired-empty":
				os.Remove(filepath.Join(r, j, "transaction.json"))
				fallthrough
			case "retired-tx-only":
				os.Remove(filepath.Join(r, j, "cleanup.ready"))
			case "incomplete":
				os.Remove(filepath.Join(r, j, "cleanup.ready"))
				ok = false
			case "foreign":
				putRecord(t, r, j+"/old/00", []byte("preserve"))
				ok = false
			case "corrupt":
				putRecord(t, r, j+"/cleanup.ready", []byte("invalid"))
				ok = false
			case "pending-other":
				putRecord(t, r, dir+"/.previous.next", []byte("unexpected contents"))
				ok = false
			case "owner-changed":
				fresh.Owners[roles[0]] = sumBytes([]byte("someone else"))
				ok = false
			case "post-no-refresh":
				ok = false
			case "journal-symlink":
				os.Rename(filepath.Join(r, j), filepath.Join(r, j+".other"))
				os.Symlink(filepath.Join(r, j+".other"), filepath.Join(r, j))
				ok = false
			case "no-receipt":
				os.Remove(filepath.Join(r, dir, "previous.json"))
				ok = false
			case "post-pending":
				putRecord(t, r, dir+"/previous.json", recordBytes(t, fresh))
				putRecord(t, r, dir+"/.previous.next", []byte("unexpected"))
				ok = false
			}
			pre, _ := os.ReadFile(filepath.Join(r, dir, "previous.json"))
			e := publishTransition(r, target, fresh, c == "post-no-refresh" || c == "post-pending")
			if (e == nil) != ok {
				t.Fatal(c, e)
			}
			after, _ := os.ReadFile(filepath.Join(r, dir, "previous.json"))
			if !ok {
				if string(after) != string(pre) {
					t.Fatal("refusal changed active handoff")
				}
				return
			}
			if err := publishTransition(r, target, fresh, false); err != nil {
				t.Fatal("retry", err)
			}
			if err := publishTransition(r, target, fresh, true); err != nil {
				t.Fatal("post", err)
			}
			for _, n := range []string{live, dead, dir + "/.previous.next"} {
				if _, err := os.Lstat(filepath.Join(r, n)); !os.IsNotExist(err) {
					t.Fatal("residue", n)
				}
			}
		})
	}
}
func TestExactSourceManifest(t *testing.T) {
	for _, kind := range []string{"allow", "modified-manifest", "changed-payload", "wrong-receipt", "no-holder-layout", "first-install", "wrong-updater"} {
		t.Run(kind, func(t *testing.T) {
			r, sets := featureFixture(t)
			p := samplePred("sample")
			for i := range p.Entries {
				path := "data/x6-owner-features/" + p.Entries[i].Target
				p.Entries[i].SHA256 = sets[0][path]
				p.Entries[i].Mode = 0600
				p.Entries[i].Bytes = int64(len(path) + 1)
			}
			raw := recordBytes(t, struct {
				Version int
				Entries []entry
			}{1, p.Entries})
			id := sumBytes(raw)
			c, e, _ := fixture("_a")

			c.FeatureSets = nil
			for n, h := range p.Owners {
				e.Source.Preserved[n] = h
			}
			putRecord(t, r, layoutMarker, []byte("1\n"))
			putRecord(t, r, "data/x6-owner-features/complete.json", []byte(id+"\n"))
			switch kind {

			case "modified-manifest":
				p.Entries[0].SHA256 = sumBytes([]byte("not the file"))
				raw = recordBytes(t, struct {
					Version int
					Entries []entry
				}{1, p.Entries})
				// Keep the installation receipt, as when the source manifest alone is corrupted.
			case "changed-payload":
				putRecord(t, r, featurePaths[0], []byte("other bytes"))
			case "wrong-receipt":
				putRecord(t, r, "data/x6-owner-features/complete.json", []byte("wrong"))
			case "no-holder-layout":
				os.Remove(filepath.Join(r, layoutMarker))
			case "first-install":
				e.Source.Version = c.Sources[0].Version
				e.Source.OTASHA256 = c.Sources[0].OTASHA256
			case "wrong-updater":
				e.Source.OTASHA256 = sumBytes([]byte("third-party updater"))
			}
			putRecord(t, r, "usr/share/x6-firstboot-seed/manifest.json", raw)
			pr, err := sourceLayout(r, c, e)
			if kind == "changed-payload" && (err != nil || !pr.Modified || pr.Entries[0].SHA256 != sumBytes([]byte("other bytes"))) {
				t.Fatal("missing actual source snapshot", err)
			}
			if (err == nil) != (kind == "allow" || kind == "changed-payload") {
				t.Fatal(kind, err)
			}
		})
	}
}
