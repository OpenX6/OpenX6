package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type restoreFile struct {
	SHA256 string
	Bytes  int64
	Mode   uint32
}
type restorePlan struct {
	Version int
	Seed    string
	Owners  map[string]string
	Files   map[string]restoreFile
}

func (e Engine) restoreMatches(rel string, v restoreFile) bool {
	if v.SHA256 == "" {
		_, err := os.Lstat(e.path(rel))
		return os.IsNotExist(err)
	}
	return e.hash(rel, v.Bytes, v.SHA256, os.FileMode(v.Mode)) == nil
}

// This bounded repair is authorized by the stock installer's pre/post handoff.
// It never trusts an old executable as input for a new executable and never
// writes holder identities or optional settings, including absent Off flags.
func (e Engine) restoreStock(m Manifest, blob []byte) (bool, error) {
	if e.Migration == nil || !e.Migration.LocalHolder {
		return false, nil
	}
	base := "data/x6-stock-restore-" + e.Pin
	active, err := e.read(base+"/active", 128, 0600)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	id := strings.TrimSuffix(string(active), ".json\n")
	if !validHash(id) || string(active) != id+".json\n" {
		return true, errors.New("invalid stock restore pointer")
	}
	dir := base + "/" + id
	raw, err := e.read(dir+"/plan.json", 16384, 0600)
	if err != nil {
		return true, err
	}
	if digest(raw) != id {
		return true, errors.New("stock restore plan digest")
	}
	done, err := e.optional(dir + "/done")
	if err != nil {
		return true, err
	}
	if done == id+"\n" {
		return false, nil
	}
	if done != "" {
		return true, errors.New("invalid stock restore receipt")
	}
	var p restorePlan
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || p.Seed != e.Pin || len(p.Files) != len(policies)+4 {
		return true, errors.New("invalid stock restore plan")
	}
	if err = e.verifyOwners(Predecessor{Owners: p.Owners}); err != nil {
		return true, err
	}
	expected := map[string]bool{}
	for rel := range policies {
		expected[rel] = true
	}
	for _, rel := range []string{"complete.json", "preserve-native-settings", "vr180-v1/sync", "raw-nr-v1/level"} {
		expected[rel] = true
	}
	for rel, v := range p.Files {
		if !expected[rel] {
			return true, errors.New("unknown stock restore target")
		}
		if v.SHA256 == "" {
			if v.Bytes != 0 || v.Mode != 0 {
				return true, errors.New("invalid absent snapshot")
			}
		} else if !validHash(v.SHA256) || v.Bytes < 0 || v.Bytes > 1<<50 || v.Mode&^0777 != 0 || v.Mode&0022 != 0 || v.Mode&0400 == 0 {
			return true, errors.New("invalid restore snapshot")
		}
	}
	// Verify all old or already replaced files before staging or mutating anything.
	for rel, v := range p.Files {
		path := featurePath + "/" + rel
		if e.restoreMatches(path, v) {
			continue
		}
		accepted := false
		for _, n := range m.Entries {
			if n.Target == rel && e.hash(path, n.Bytes, n.SHA256, os.FileMode(n.Mode)) == nil {
				accepted = true
			}
		}
		if rel == "complete.json" || rel == "preserve-native-settings" {
			b, x := e.read(path, 65, 0600)
			accepted = x == nil && string(b) == e.Pin+"\n"
		}
		if (rel == "vr180-v1/sync" || rel == "raw-nr-v1/level") && v.SHA256 == "" {
			b, x := e.read(path, 1, 0600)
			accepted = x == nil && string(b) == "\x00"
		}
		if !accepted {
			return true, fmt.Errorf("stock restore state changed: %s", rel)
		}
	}
	stage := "data/.x6-stock-payload-" + id
	if err = e.stagePayloads(m, blob, stage); err != nil {
		return true, err
	}
	if err = e.hit("stock-restore-ready"); err != nil {
		return true, err
	}
	for _, n := range m.Entries {
		path := featurePath + "/" + n.Target
		if e.hash(path, n.Bytes, n.SHA256, os.FileMode(n.Mode)) == nil {
			continue
		}
		if !e.restoreMatches(path, p.Files[n.Target]) {
			return true, errors.New("stock restore input changed")
		}
		parent := filepath.Dir(path)
		if err = e.stageDir(parent); err != nil {
			return true, err
		}
		err = e.write(parent, filepath.Base(path), os.FileMode(n.Mode), func(out *os.File) error {
			in, x := e.open(stage+"/"+n.Target, os.FileMode(n.Mode))
			if x != nil {
				return x
			}
			defer in.Close()
			if _, x = io.Copy(out, in); x != nil {
				return x
			}
			if p.Files[n.Target].SHA256 != "" {
				return e.copyLabel(path, out)
			}
			// New files inherit the label from the corresponding verified base.
			return e.copyLabel(n.Base, out)
		})
		if err != nil {
			return true, err
		}
		if err = e.hit("stock-restore-file:" + n.Target); err != nil {
			return true, err
		}
	}
	// Required scalar values have a neutral fallback. Optional flag absence is
	// preserved exactly; fresh-install enabled defaults are never applied here.
	for _, rel := range []string{"vr180-v1/sync", "raw-nr-v1/level"} {
		if p.Files[rel].SHA256 == "" {
			if err = e.saveBytes(featurePath, rel, []byte{0}, 0600); err != nil {
				return true, err
			}
		}
	}
	if err = e.verifyContent(m); err != nil {
		return true, err
	}
	if err = e.saveBytes(featurePath, "preserve-native-settings", []byte(e.Pin+"\n"), 0600); err != nil {
		return true, err
	}
	if err = e.hit("stock-restore-preserved"); err != nil {
		return true, err
	}
	if err = e.saveBytes(featurePath, "complete.json", []byte(e.Pin+"\n"), 0600); err != nil {
		return true, err
	}
	if err = e.verifyFinal(m); err != nil {
		return true, err
	}
	if err = e.hit("stock-restore-complete"); err != nil {
		return true, err
	}
	if err = e.removeStage(stage); err != nil {
		return true, err
	}
	if err = e.saveBytes(dir, "done", []byte(id+"\n"), 0600); err != nil {
		return true, err
	}
	return true, e.finish(m)
}
