package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Completed migration copies are transaction scratch, not a permanent rollback
// archive. Running feature payloads, holder keys and unknown directories are never
// cleanup targets. A tiny authenticated-by-local-state journal remains for resume.
func (e Engine) cleanupCompleted(m Manifest) error {
	if err := e.verifyFinal(m); err != nil {
		return err
	}
	dir := "data/.x6-firstboot-migrate-" + e.Pin
	if _, err := os.Lstat(e.path(dir)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if e.Migration == nil {
		return errors.New("cleanup lacks predecessor policy")
	}
	if err := metadata(e.path(dir), true, 0700); err != nil {
		return err
	}
	raw, err := e.read(dir+"/transaction.json", 4096, 0600)
	if err != nil {
		return err
	}
	var tx transaction
	if json.Unmarshal(raw, &tx) != nil || tx.Version != 1 || tx.To != e.Pin {
		return errors.New("cleanup journal mismatch")
	}
	var previous *Predecessor
	for _, p := range e.Migration.Previous {
		if p.Seed == tx.From {
			v := p
			previous = &v
		}
	}
	if previous == nil || (tx.Legacy && previous.LegacySSH == nil) {
		return errors.New("unknown cleanup origin")
	}
	if err = e.verifyOwners(*previous); err != nil {
		return err
	}
	items, err := e.migrationItems(*previous, m, tx.Legacy)
	if err != nil {
		return err
	}
	expected := map[string]Snapshot{}
	for i, v := range items {
		expected[fmt.Sprintf("old/%02d", i)] = v.Old
		expected[fmt.Sprintf("new/%02d", i)] = v.New
	}
	marker := []byte(digest(raw) + "\n" + e.Pin + "\n")
	ready := false
	b, err := e.read(dir+"/cleanup.ready", 256, 0600)
	if err == nil {
		if string(b) != string(marker) {
			return errors.New("cleanup marker mismatch")
		}
		ready = true
	} else if !os.IsNotExist(err) {
		return err
	}
	// Check the entire remaining tree before removing anything. WalkDir does not
	// follow symlinks; metadata also rejects hard links and writable ownership.
	err = filepath.WalkDir(e.path(dir), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, x := filepath.Rel(e.path(dir), path)
		if x != nil {
			return x
		}
		if rel == "." || rel == "old" || rel == "new" {
			return metadata(path, true, 0700)
		}
		if rel == ".writing" {
			b, x := e.read(dir+"/.writing", 256, 0600)
			if x != nil {
				return x
			}
			if len(b) > len(marker) || string(b) != string(marker[:len(b)]) {
				return errors.New("unknown partial cleanup marker")
			}
			return nil
		}
		if rel == "transaction.json" || rel == "cleanup.ready" {
			return metadata(path, false, 0600)
		}
		v, ok := expected[rel]
		if !ok {
			return errors.New("unknown file in completed migration")
		}
		return e.hash(dir+"/"+rel, v.Bytes, v.SHA256, os.FileMode(v.Mode))
	})
	if err != nil {
		return err
	}
	if !ready {
		for rel, v := range expected {
			if err = e.hash(dir+"/"+rel, v.Bytes, v.SHA256, os.FileMode(v.Mode)); err != nil {
				return err
			}
		}
		if err = e.saveBytes(dir, "cleanup.ready", marker, 0600); err != nil {
			return err
		}
	}
	if err = os.Remove(e.path(dir + "/.writing")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = e.hit("cleanup-ready"); err != nil {
		return err
	}
	for _, kind := range []string{"old", "new"} {
		for i := range items {
			rel := fmt.Sprintf("%s/%s/%02d", dir, kind, i)
			if err = os.Remove(e.path(rel)); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err = e.hit("cleanup-removed:" + kind + fmt.Sprint(i)); err != nil {
				return err
			}
		}
		if err = os.Remove(e.path(dir + "/" + kind)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return syncDir(e.path(dir))
}
func (e Engine) finish(m Manifest) error {
	if err := e.verifyFinal(m); err != nil {
		return err
	}
	if err := e.cleanupCompleted(m); err != nil {
		// A cleanup refusal must not disable a verified working root/startup path.
		fmt.Fprintln(os.Stderr, "OpenX6 completed migration cleanup deferred:", err)
	}
	return nil
}
