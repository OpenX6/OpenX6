package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// The target bootstrap uses directories keyed by target seed. Re-entering a
// former slot must start a fresh transaction even if the source is identical.
func readPrivateRecord(root, rel string, limit int64) ([]byte, error) {
	st, err := os.Lstat(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	if st.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("unsafe record mode: %s", rel)
	}
	return bounded(root, rel, limit)
}

type completedTransaction struct {
	Version                     int
	From, To, Receipt, Preserve string
	Legacy                      bool
}

func journalContents(root, rel, target string, old *predecessor, retiring bool) (map[string]bool, error) {
	dir := filepath.Join(root, rel)
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	rows, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	for _, r := range rows {
		switch r.Name() {
		case "cleanup.ready", "transaction.json":
			found[r.Name()] = true
		default:
			return nil, fmt.Errorf("target has unfinished migration or foreign journal file")
		}
	}
	if retiring && len(rows) == 0 {
		return found, nil
	}
	if old == nil || !found["transaction.json"] || (!retiring && !found["cleanup.ready"]) {
		return nil, errors.New("target migration has no verified completion")
	}
	raw, err := readPrivateRecord(root, rel+"/transaction.json", 4096)
	if err != nil {
		return nil, err
	}
	var tx completedTransaction
	if json.Unmarshal(raw, &tx) != nil || tx.Version != 1 || tx.To != target || tx.From != old.Seed || (tx.From == target && !old.Modified) || tx.Receipt != old.Seed+"\n" || (tx.Preserve != "" && tx.Preserve != old.Seed+"\n") {
		return nil, errors.New("old migration journal does not match handoff")
	}
	if found["cleanup.ready"] {
		mark, err := readPrivateRecord(root, rel+"/cleanup.ready", 256)
		if err != nil {
			return nil, err
		}
		if string(mark) != sumBytes(raw)+"\n"+target+"\n" {
			return nil, errors.New("old migration completion checksum mismatch")
		}
	}
	return found, nil
}

// Only two fully cleaned metadata records may be retired. The complete live
// directory is renamed first so a crash can never look like a resumable journal.
func retireFinished(root, target string, old *predecessor) error {
	live := "data/.x6-firstboot-migrate-" + target
	dead := "data/.x6-retired-journal-" + target
	base := filepath.Join(root, "data")
	drain := func() error {
		files, err := journalContents(root, dead, target, old, true)
		if os.IsNotExist(err) {
			if _, x := os.Lstat(filepath.Join(root, dead)); os.IsNotExist(x) {
				return nil
			}
		}
		if err != nil {
			return err
		}
		d := filepath.Join(root, dead)
		for _, name := range []string{"cleanup.ready", "transaction.json"} {
			if files[name] {
				if err = os.Remove(filepath.Join(d, name)); err != nil {
					return err
				}
				if err = syncDirectory(d); err != nil {
					return err
				}
			}
		}
		if err = os.Remove(d); err != nil {
			return err
		}
		return syncDirectory(base)
	}
	if err := drain(); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(root, live)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := journalContents(root, live, target, old, false); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(root, live), filepath.Join(root, dead)); err != nil {
		return err
	}
	if err := syncDirectory(base); err != nil {
		return err
	}
	return drain()
}

// Bound the aggregate space arithmetic. The actual limit is free space on /data.
func safeProgramSnapshot(size int64, mode uint32) bool {
	return size >= 0 && size <= 1<<50 && mode&^0777 == 0 && mode&0022 == 0 && mode&0400 != 0
}

func readOldHandoff(root, rel string, owners map[string]string) (*predecessor, []byte, error) {
	dir := filepath.Join(root, rel)
	if err := checkDir(dir); os.IsNotExist(err) {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range files {
		if f.Name() != "previous.json" && f.Name() != ".previous.next" {
			return nil, nil, errors.New("unexpected handoff member")
		}
	}
	raw, err := readPrivateRecord(root, rel+"/previous.json", 16384)
	if err != nil {
		return nil, nil, err
	}
	var p predecessor
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || !isHash(p.Seed) || len(p.Owners) != len(roles) || len(p.Entries) != len(featurePaths) {
		return nil, nil, errors.New("invalid old handoff")
	}
	for _, role := range roles {
		if p.Owners[role] != owners[role] || !isHash(p.Owners[role]) {
			return nil, nil, errors.New("holder mismatch in old handoff")
		}
	}
	unseen := map[string]bool{}
	for _, path := range featurePaths {
		unseen[path] = true
	}
	for _, v := range p.Entries {
		path := "data/x6-owner-features/" + v.Target
		if !unseen[path] || !isHash(v.SHA256) || !safeProgramSnapshot(v.Bytes, v.Mode) {
			return nil, nil, errors.New("bad old handoff entry")
		}
		delete(unseen, path)
	}
	return &p, raw, nil
}

func publishTransition(root, target string, p *predecessor, verifyOnly bool) error {
	rel := "data/x6-inherit-" + target
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	files := map[string][]byte{"previous.json": raw}
	if verifyOnly {
		return publish(root, rel, files, true)
	}
	base, err := os.Open(filepath.Join(root, "data"))
	if err != nil {
		return err
	}
	defer base.Close()
	if err = syscall.Flock(int(base.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	old, prior, err := readOldHandoff(root, rel, p.Owners)
	if err != nil {
		return err
	}
	next := rel + "/.previous.next"
	part, err := readPrivateRecord(root, next, 16384)
	if err != nil && !os.IsNotExist(err) {
		// A crash immediately after exclusive create may leave a zero-length file.
		s, e := os.Lstat(filepath.Join(root, next))
		if e != nil || s.Size() != 0 || s.Mode().Perm() != 0600 {
			return err
		}
		if _, e = checkedHash(root, next, false); e != nil {
			return e
		}
		part = []byte{}
	}
	if part != nil && !bytes.HasPrefix(raw, part) {
		return errors.New("conflicting pending migration handoff")
	}
	if err = retireFinished(root, target, old); err != nil {
		return err
	}
	if old == nil {
		return publish(root, rel, files, false)
	}
	dir := filepath.Join(root, rel)
	if part != nil {
		if err = os.Remove(filepath.Join(root, next)); err != nil {
			return err
		}
	}
	if !bytes.Equal(prior, raw) {
		fd, err := os.OpenFile(filepath.Join(root, next), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		if _, err = fd.Write(raw); err == nil {
			err = fd.Sync()
		}
		ce := fd.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
		if err = os.Rename(filepath.Join(root, next), filepath.Join(dir, "previous.json")); err != nil {
			return err
		}
	}
	if err = syncDirectory(dir); err != nil {
		return err
	}
	return publish(root, rel, files, true)
}
