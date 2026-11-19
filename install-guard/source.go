package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type SourceRule struct{ Version, OTASHA256, Mode string }
type SourceEvidence struct {
	Version, OTASHA256, OwnerPublicSHA256 string
	RetainedHolder                        bool
	Preserved                             map[string]string
}

var preservedPaths = []string{
	"data/x6-owner-features/mod-controls-v1/stereo.pref", "data/x6-owner-features/mod-controls-v1/hnr.pref", "data/x6-owner-features/mod-controls-v1/vnr.pref", "data/x6-owner-features/proxy-v1/disabled",
	"data/x6-owner-features/vr180-v1/enabled", "data/x6-owner-features/raw-nr-v1/enabled",
	"data/x6-owner-ssh/host_ed25519", "data/x6-owner-gui/server_ed25519", "data/x6-owner-gui/server_ed25519.pub",
	"data/x6-owner-features/vr180-v1/sync", "data/x6-owner-features/raw-nr-v1/level",
	"ins/etc/x6_owner_authorized_keys", "ins/etc/x6_owner_gui_ed25519.pub", "ins/etc/x6_owner_factory_ed25519.pub", "ins/etc/x6_owner_update_ed25519.pub",
}

func sourceID(e Evidence) string {
	b, _ := json.Marshal(struct {
		Source SourceEvidence
		GUI    string
	}{e.Source, e.PublicSHA256})
	return sumBytes(b)
}
func validateSource(c Config, e Evidence) error {
	s := e.Source
	mode := ""
	for _, r := range c.Sources {
		if s.Version == r.Version && s.OTASHA256 == r.OTASHA256 {
			if mode != "" {
				return fmt.Errorf("ambiguous source")
			}
			mode = r.Mode
		}
	}
	if mode == "" {
		return fmt.Errorf("unsupported source build")
	}
	if len(c.OwnerTrust) != 4 {
		return fmt.Errorf("missing owner trust policy")
	}
	dynamic := c.TargetSeed != "" && (mode == "maintenance" || (mode == "first-install" && s.RetainedHolder))
	for path, want := range c.OwnerTrust {
		if dynamic {
			want = s.Preserved[path]
		}
		got := s.Preserved[path]
		if !isHash(want) || (got != "" && got != want) || (mode == "maintenance" && got == "") {
			return fmt.Errorf("owner trust would be lost or changed: %s", path)
		}
	}
	expectedUpdate := c.PublicSHA256
	if dynamic {
		expectedUpdate = s.Preserved["ins/etc/x6_owner_update_ed25519.pub"]
	}
	if s.OwnerPublicSHA256 != "" && s.OwnerPublicSHA256 != expectedUpdate {
		return fmt.Errorf("owner update trust would change")
	}
	if e.PublicSHA256 != "" && !isHash(e.PublicSHA256) {
		return fmt.Errorf("bad GUI identity")
	}
	if p, ok := s.Preserved["data/x6-owner-gui/server_ed25519.pub"]; ok && p != e.PublicSHA256 {
		return fmt.Errorf("GUI identity disagreement")
	}
	for _, h := range s.Preserved {
		if !isHash(h) {
			return fmt.Errorf("bad preservation evidence")
		}
	}
	if mode == "maintenance" {
		if s.OwnerPublicSHA256 != expectedUpdate || !isHash(e.PublicSHA256) || !isHash(s.Preserved["data/x6-owner-ssh/host_ed25519"]) || !isHash(s.Preserved["data/x6-owner-gui/server_ed25519"]) {
			return fmt.Errorf("maintenance identity/trust missing")
		}
	} else if mode != "first-install" {
		return fmt.Errorf("unknown source policy")
	}
	return nil
}
func checkedHash(root, rel string, optional bool) (string, error) {
	p := filepath.Join(root, rel)
	st, err := os.Lstat(p)
	if optional && os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 {
		return "", fmt.Errorf("unsafe preserved file %s", rel)
	}
	ss, ok := st.Sys().(*syscall.Stat_t)
	if !ok || ss.Uid != uint32(os.Geteuid()) || ss.Nlink != 1 {
		return "", fmt.Errorf("unsafe ownership %s", rel)
	}
	for parent := filepath.Dir(p); ; parent = filepath.Dir(parent) {
		s, e := os.Lstat(parent)
		if e != nil {
			return "", e
		}
		v := s.Sys().(*syscall.Stat_t)
		if !s.IsDir() || s.Mode().Perm()&0022 != 0 || v.Uid != uint32(os.Geteuid()) {
			return "", fmt.Errorf("unsafe parent")
		}
		if parent == root {
			break
		}
		if parent == filepath.Dir(parent) {
			return "", fmt.Errorf("boundary missing")
		}
	}
	fd, err := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	now, err := f.Stat()
	if err != nil || !os.SameFile(st, now) {
		return "", fmt.Errorf("file changed")
	}
	// Hash through the held descriptor, not by reopening the path.
	return hashReader(f)
}
func collectSource(root string) (SourceEvidence, error) {
	s := SourceEvidence{Preserved: map[string]string{}}
	var err error
	if _, err = checkedHash(root, "ins/etc/version.txt", false); err != nil {
		return s, err
	}
	b, err := os.ReadFile(filepath.Join(root, "ins/etc/version.txt"))
	if err != nil {
		return s, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "qcs8625_Version:V") {
			if s.Version != "" {
				return s, fmt.Errorf("duplicate SoC version")
			}
			s.Version = strings.TrimSpace(strings.TrimPrefix(line, "qcs8625_Version:V"))
		}
	}
	s.OTASHA256, err = checkedHash(root, "usr/bin/ins_otad", false)
	if err != nil {
		return s, err
	}
	layout, err := effectiveSource(root)
	if err != nil {
		return s, err
	}
	s.RetainedHolder = layout == holderDir
	paths := append([]string{}, preservedPaths...)
	paths = append(paths, "adb_keys")
	for _, p := range paths {
		sourcePath := p
		for _, role := range roles {
			if role == p {
				sourcePath = sourceKey(p, layout)
			}
		}
		h, e := checkedHash(root, sourcePath, true)
		if e != nil {
			return s, e
		}
		if h != "" {
			s.Preserved[p] = h
		}
	}
	s.OwnerPublicSHA256 = s.Preserved["ins/etc/x6_owner_update_ed25519.pub"]
	return s, nil
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, e := io.Copy(h, r); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
