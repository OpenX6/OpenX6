package main

import (
	"encoding/json"
	"fmt"
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

// Snapshot replaceable files, not a history of accepted executable hashes. The
// new slot reconstructs programs exclusively from its own pinned seed.
func stockRestore(root string, c Config, s SourceEvidence) (*restorePlan, error) {
	const base = "data/x6-owner-features"
	if sourceMode(c, s) != "first-install" || !isHash(c.TargetSeed) {
		return nil, fmt.Errorf("not stock reinstall")
	}
	if _, err := os.Lstat(filepath.Join(root, base)); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if err := checkDir(filepath.Join(root, base)); err != nil {
		return nil, err
	}
	p := &restorePlan{1, c.TargetSeed, map[string]string{}, map[string]restoreFile{}}
	for _, role := range roles {
		h := sumBytes(c.InitialKeys[role])
		if s.RetainedHolder {
			h = s.Preserved[role]
		}
		if !isHash(h) {
			return nil, fmt.Errorf("missing retained role")
		}
		p.Owners[role] = h
	}
	paths := append([]string{}, featurePaths...)
	for _, n := range []string{"complete.json", "preserve-native-settings", "vr180-v1/sync", "raw-nr-v1/level"} {
		paths = append(paths, base+"/"+n)
	}
	for _, path := range paths {
		h, err := checkedHash(root, path, true)
		if err != nil {
			return nil, err
		}
		v := restoreFile{}
		if h != "" {
			st, err := os.Lstat(filepath.Join(root, path))
			if err != nil {
				return nil, err
			}
			if !safeProgramSnapshot(st.Size(), uint32(st.Mode().Perm())) {
				return nil, fmt.Errorf("unsafe recovery file")
			}
			v = restoreFile{h, st.Size(), uint32(st.Mode().Perm())}
		}
		p.Files[strings.TrimPrefix(path, base+"/")] = v
	}
	// Absent toggles remain absent. Malformed preferences are never interpreted as defaults.
	for path, allowed := range featurePreferences {
		h, err := checkedHash(root, path, true)
		if err != nil {
			return nil, err
		}
		if h == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, path))
		ok := false
		if err == nil && len(b) == 1 {
			for _, v := range allowed {
				if b[0] == v {
					ok = true
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("invalid retained preference: %s", path)
		}
	}
	return p, nil
}

func publishStockRestore(root string, c Config, s SourceEvidence, p *restorePlan, verifyOnly bool) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if sumBytes(raw) != s.Preserved["stock-reinstall-plan"] {
		return fmt.Errorf("stock state changed after preflight")
	}
	// Content-addressed handoffs cannot collide with a previous attempt, even
	// when returning to a previously installed target seed.
	rel := "data/x6-stock-restore-" + c.TargetSeed
	if _, err = os.Lstat(filepath.Join(root, rel)); os.IsNotExist(err) {
		if verifyOnly {
			return err
		}
		if err = os.Mkdir(filepath.Join(root, rel), 0700); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err = checkDir(filepath.Join(root, rel)); err != nil {
		return err
	}
	name := sumBytes(raw) + ".json"
	planDir := rel + "/" + sumBytes(raw)
	done, doneErr := bounded(root, planDir+"/done", 65)
	if doneErr == nil {
		prior, x := bounded(root, planDir+"/plan.json", 16384)
		names, y := os.ReadDir(filepath.Join(root, planDir))
		if string(done) != sumBytes(raw)+"\n" || x != nil || y != nil || string(prior) != string(raw) || len(names) != 2 {
			return fmt.Errorf("invalid consumed recovery handoff")
		}
	} else if !os.IsNotExist(doneErr) {
		return doneErr
	} else if err = publish(root, planDir, map[string][]byte{"plan.json": raw}, verifyOnly); err != nil {
		return err
	}
	// Only a tiny pointer changes; no live feature, preference or identity is altered.
	want := []byte(name + "\n")
	active := filepath.Join(root, rel, "active")
	if verifyOnly {
		b, err := bounded(root, rel+"/active", 128)
		if err != nil || string(b) != string(want) {
			return fmt.Errorf("reinstall handoff changed")
		}
		return nil
	}
	if _, err = checkedHash(root, rel+"/active", true); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Join(root, rel), ".active-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(want)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = os.Rename(f.Name(), active); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Join(root, rel)); err != nil {
		return err
	}
	return syncDirectory(filepath.Join(root, "data"))
}
