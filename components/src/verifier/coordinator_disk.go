package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func privateDir(path string, p policy) error {
	if e := parents(filepath.Join(filepath.Dir(path), "sentinel"), p); e != nil {
		return e
	}
	if e := os.Mkdir(path, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	s, e := os.Lstat(path)
	if e != nil {
		return e
	}
	u := s.Sys().(*syscall.Stat_t)
	if !s.IsDir() || s.Mode().Perm() != 0700 || u.Uid != p.UID {
		return fail("unsafe coordinator directory")
	}
	return nil
}
func loadSDState(path string, p policy) (SDState, error) {
	f, e := openChecked(path, p, false)
	if os.IsNotExist(e) {
		return SDState{}, nil
	}
	if e != nil {
		return SDState{}, e
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return SDState{}, e
	}
	if s.Mode().Perm() != 0600 || s.Size() > 8192 {
		return SDState{}, fail("unsafe state file")
	}
	b := make([]byte, s.Size())
	if _, e = f.ReadAt(b, 0); e != nil {
		return SDState{}, e
	}
	return decodeState(b)
}
func saveSDState(path string, p policy, s SDState) error {
	if !validState(s) {
		return fail("invalid state")
	}
	if e := privateDir(filepath.Dir(path), p); e != nil {
		return e
	}
	if _, e := loadSDState(path, p); e != nil {
		return e
	}
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".state-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func ownedImportDir(dir string, p policy, digest string) error {
	if e := parents(filepath.Join(dir, "sentinel"), p); e != nil {
		return e
	}
	s, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !s.IsDir() || s.Mode().Perm() != 0700 {
		return fail("not private import")
	}
	marker, e := openChecked(filepath.Join(dir, ".x6-import-receipt"), p, false)
	if e != nil {
		return e
	}
	defer marker.Close()
	b := make([]byte, 128)
	n, e := marker.Read(b)
	if e != nil {
		return e
	}
	if string(b[:n]) != "X6 SINGLE IMPORT V2\n"+digest+"\n" {
		return fail("foreign import receipt")
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	for _, x := range entries {
		if x.Name() != "package.bin" && x.Name() != "package.bin.owner-auth" && x.Name() != ".x6-import-receipt" {
			return fail("foreign staging file")
		}
		f, e := openChecked(filepath.Join(dir, x.Name()), p, false)
		if e != nil {
			return e
		}
		st, e := f.Stat()
		f.Close()
		if e != nil {
			return e
		}
		if st.Size() > 1<<32 {
			return fail("oversize temporary file")
		}
	}
	return nil
}
func cleanImportOrphans(parent string, p policy, digest string) error {
	entries, e := os.ReadDir(parent)
	if e != nil {
		return e
	}
	for _, x := range entries {
		if !strings.HasPrefix(x.Name(), ".x6-single-import-") {
			continue
		}
		dir := filepath.Join(parent, x.Name())
		if ownedImportDir(dir, p, digest) != nil {
			continue
		}
		if e = removeImportDir(dir, p, digest); e != nil {
			return e
		}
	}
	return nil
}
func removeImportDir(dir string, p policy, digest string) error {
	if e := ownedImportDir(dir, p, digest); e != nil {
		// A kill after the final receipt unlink can leave only an empty private dir.
		if e2 := parents(filepath.Join(dir, "sentinel"), p); e2 != nil {
			return e
		}
		st, e2 := os.Lstat(dir)
		if e2 != nil || !st.IsDir() || st.Mode().Perm() != 0700 {
			return e
		}
		entries, e2 := os.ReadDir(dir)
		if e2 != nil || len(entries) != 0 {
			return e
		}
		return os.Remove(dir)
	}
	for _, name := range []string{"package.bin", "package.bin.owner-auth", ".x6-import-receipt"} {
		if e := os.Remove(filepath.Join(dir, name)); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return os.Remove(dir)
}
func retireVerifiedStage(p policy, s SDState, newDigest string) error {
	retired := ""
	if validID(s.Digest) && (s.Phase == "completed" || s.Phase == "resident") {
		retired = filepath.Join(filepath.Dir(filepath.Dir(p.Package)), ".x6-single-retired-"+s.Digest)
		if _, e := os.Lstat(retired); e == nil {
			if e = removeImportDir(retired, p, s.Digest); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}

	if s.Digest == newDigest {
		return nil
	}
	if _, e := os.Lstat(filepath.Dir(p.Package)); os.IsNotExist(e) {
		return nil
	}
	m, e := stagedRecord(p)
	if e != nil {
		return e
	}
	if _, e = verify(p, p.Package, m.Version, true); e != nil {
		return e
	}
	// Publishing the next complete stage can precede the journal commit.
	if fmt.Sprintf("%x", m.Digest) == newDigest {
		return nil
	}
	if retired == "" {
		return fail("previous staging cannot be retired")
	}
	if fmt.Sprintf("%x", m.Digest) != s.Digest {
		return fail("previous digest mismatch")
	}
	if _, e = verify(p, p.Package, m.Version, true); e != nil {
		return e
	}
	dir := filepath.Dir(p.Package)
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	for _, x := range entries {
		if x.Name() != "package.bin" && x.Name() != "package.bin.owner-auth" && x.Name() != ".x6-import-receipt" {
			return fail("unrecognized staging content")
		}
		f, e := openChecked(filepath.Join(dir, x.Name()), p, false)
		if e != nil {
			return e
		}
		f.Close()
	}
	// Move the verified directory out of the publication path atomically.
	if e = os.Rename(dir, retired); e != nil {
		return e
	}
	parent, e := os.Open(filepath.Dir(dir))
	if e != nil {
		return e
	}
	e = parent.Sync()
	parent.Close()
	if e != nil {
		return e
	}
	return removeImportDir(retired, p, s.Digest)
}
