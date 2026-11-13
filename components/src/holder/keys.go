// Package holder reads the complete local trust set committed by the recovery guard.
package holder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const Dir = "/data/x6-holder-v1"
const SSH = "x6_owner_authorized_keys"
const GUI = "x6_owner_gui_ed25519.pub"
const Factory = "x6_owner_factory_ed25519.pub"
const Update = "x6_owner_update_ed25519.pub"
const ADB = "adb_keys"

func Read(root, name string) ([]byte, error) {
	switch name {
	case SSH, GUI, Factory, Update, ADB:
	default:
		return nil, errors.New("unknown holder role")
	}
	dir := filepath.Join(root, Dir)
	for _, p := range []string{filepath.Join(root, "data"), dir} {
		s, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if !s.IsDir() || s.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) || s.Mode().Perm()&0022 != 0 || (p == dir && s.Mode().Perm() != 0700) {
			return nil, errors.New("unsafe holder directory")
		}
	}
	read := func(n string) ([]byte, error) {
		f, e := os.OpenFile(filepath.Join(dir, n), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		s, e := f.Stat()
		if e != nil {
			return nil, e
		}
		st := s.Sys().(*syscall.Stat_t)
		if !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || s.Size() < 1 || s.Size() > 4096 {
			return nil, errors.New("unsafe holder record")
		}
		return io.ReadAll(io.LimitReader(f, 4097))
	}
	pins, e := read("pins.json")
	if e != nil {
		return nil, e
	}
	var m map[string]string
	if json.Unmarshal(pins, &m) != nil || len(m) != 5 {
		return nil, errors.New("invalid holder pins")
	}
	b, e := read(name)
	if e != nil {
		return nil, e
	}
	h := sha256.Sum256(b)
	if m[name] != hex.EncodeToString(h[:]) {
		return nil, errors.New("holder key digest mismatch")
	}
	return b, nil
}
