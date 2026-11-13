// Local update handoff. Only public keys and a description of verified source
// files cross the slot boundary. No device private key is copied.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const holderDir = "data/x6-holder-v1"
const layoutMarker = "usr/share/x6-holder-layout"

var roles = []string{"ins/etc/x6_owner_authorized_keys", "ins/etc/x6_owner_gui_ed25519.pub", "ins/etc/x6_owner_factory_ed25519.pub", "ins/etc/x6_owner_update_ed25519.pub", "adb_keys"}

type entry struct {
	Target, SHA256 string
	Bytes          int64
	Mode           uint32
}
type helper struct {
	SHA256 string
	Bytes  int64
}
type predecessor struct {
	Version     int
	Seed        string
	Owners      map[string]string
	Entries     []entry
	LegacySSH   *helper
	Modified    bool `json:",omitempty"`
	extraBytes  uint64
	Receiptless bool
}

func bounded(root, rel string, limit int64) ([]byte, error) {
	h, e := checkedHash(root, rel, false)
	if e != nil {
		return nil, e
	}
	p := filepath.Join(root, rel)
	s, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if s.Size() < 1 || s.Size() > limit {
		return nil, fmt.Errorf("invalid size: %s", rel)
	}
	b, e := os.ReadFile(p)
	if e != nil || sumBytes(b) != h {
		return nil, fmt.Errorf("changed: %s", rel)
	}
	return b, nil
}
func effectiveSource(root string) (string, error) {
	h, e := checkedHash(root, layoutMarker, true)
	if e != nil {
		return "", e
	}
	if h == "" {
		return "", nil
	}
	if h != sumBytes([]byte("1\n")) {
		return "", fmt.Errorf("unknown holder layout")
	}
	return holderDir, nil
}
func sourceKey(path, layout string) string {
	if layout == "" {
		return path
	}
	return filepath.Join(layout, filepath.Base(path))
}
func sourceMode(c Config, s SourceEvidence) string {
	for _, r := range c.Sources {
		if s.Version == r.Version && s.OTASHA256 == r.OTASHA256 {
			return r.Mode
		}
	}
	return ""
}
func sourceLayout(root string, c Config, e Evidence) (*predecessor, error) {
	const seed = "usr/share/x6-firstboot-seed/manifest.json"
	b, err := bounded(root, seed, 1<<20)
	if err != nil {
		return nil, err
	}
	p := predecessor{Version: 1, Seed: sumBytes(b), Owners: map[string]string{}}
	receipt, err := bounded(root, "data/x6-owner-features/complete.json", 65)
	if err != nil || string(receipt) != p.Seed+"\n" {
		return nil, fmt.Errorf("unverified previous seed receipt")
	}
	var m struct {
		Version int
		Entries []entry
	}
	if json.Unmarshal(b, &m) != nil || m.Version != 1 || len(m.Entries) != len(featurePaths) {
		return nil, fmt.Errorf("invalid source seed")
	}
	if sourceMode(c, e.Source) != "maintenance" {
		return nil, fmt.Errorf("program replacement requires maintenance source")
	}
	declared := map[string]string{}
	for i, v := range m.Entries {
		path := "data/x6-owner-features/" + v.Target
		known := false
		for _, allowed := range featurePaths {
			if path == allowed {
				known = true
			}
		}
		if !known || declared[path] != "" || !isHash(v.SHA256) || v.Bytes <= 0 || v.Bytes > 160<<20 || (v.Mode != 0600 && v.Mode != 0644 && v.Mode != 0755) {
			return nil, fmt.Errorf("invalid source entry")
		}
		st, x := os.Lstat(filepath.Join(root, path))
		if x != nil {
			return nil, x
		}
		mode := uint32(st.Mode().Perm())
		if !safeProgramSnapshot(st.Size(), mode) || st.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return nil, fmt.Errorf("unsafe installed program: %s", v.Target)
		}
		hash, x := checkedHash(root, path, false)
		if x != nil {
			return nil, x
		}
		if hash != v.SHA256 || mode != v.Mode || st.Size() != v.Bytes {
			p.Modified = true
		}
		if st.Size() > v.Bytes {
			p.extraBytes += uint64(st.Size() - v.Bytes)
		}
		// These bytes identify the old input for an atomic replacement. Only the
		// destination image supplies and authenticates the new executable.
		m.Entries[i] = entry{v.Target, hash, st.Size(), mode}
		declared[path] = v.SHA256
	}
	// Only the two setup executables contained build-time owner pins. All
	// other payloads must belong to one known layout (no mix-and-match).
	compatible := false
	for _, set := range c.FeatureSets {
		match := len(set) == len(declared)
		for n, h := range declared {
			if !strings.HasSuffix(n, "/setup") && set[n] != h {
				match = false
			}
		}
		if match {
			compatible = true
		}
	}
	if !compatible {
		// The installed slot provides its manifest; user policy accepts any compatible
		// local-holder layout. Its receipt must identify the source manifest.
		layout, x := effectiveSource(root)
		if x != nil {
			return nil, x
		}
		compatible = layout == holderDir && isHash(c.TargetSeed) && sourceMode(c, e.Source) == "maintenance"
	}
	if !compatible {
		return nil, fmt.Errorf("unsupported source feature family")
	}
	if err = checkFeaturePreferencesAt(root); err != nil {
		return nil, err
	}
	for _, name := range roles {
		p.Owners[name] = e.Source.Preserved[name]
		if !isHash(p.Owners[name]) {
			return nil, fmt.Errorf("missing source role")
		}
	}
	p.Entries = m.Entries
	if _, err = os.Lstat(filepath.Join(root, "etc/systemd/system/x6-owner-maintenance.service")); err == nil {
		h, x := checkedHash(root, "usr/local/sbin/x6-owner-maintenance", false)
		if x != nil {
			return nil, x
		}
		st, x := os.Stat(filepath.Join(root, "usr/local/sbin/x6-owner-maintenance"))
		if x != nil {
			return nil, x
		}
		p.LegacySSH = &helper{h, st.Size()}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return &p, nil
}
func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func publish(root, rel string, files map[string][]byte, verifyOnly bool) error {
	dir := filepath.Join(root, rel)
	parent := filepath.Dir(dir)
	check := func() error {
		if err := checkDir(dir); err != nil {
			return err
		}
		names, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(names) != len(files) {
			return fmt.Errorf("partial or foreign handoff")
		}
		for n, want := range files {
			st, x := os.Lstat(filepath.Join(dir, n))
			if x != nil || st.Mode().Perm() != 0600 {
				return fmt.Errorf("unsafe handoff file: %s", n)
			}
			b, x := bounded(root, rel+"/"+n, int64(len(want)))
			if x != nil || !bytes.Equal(b, want) {
				return fmt.Errorf("conflicting handoff: %s", n)
			}
		}
		return nil
	}
	if verifyOnly {
		return check()
	}
	if _, err := os.Lstat(dir); err == nil {
		return check()
	} else if !os.IsNotExist(err) {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".x6-holder-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for n, b := range files {
		f, x := os.OpenFile(filepath.Join(stage, n), os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0600)
		if x != nil {
			return x
		}
		_, x = f.Write(b)
		if x == nil {
			x = f.Sync()
		}
		ce := f.Close()
		if x != nil {
			return x
		}
		if ce != nil {
			return ce
		}
	}
	if err = syncDirectory(stage); err != nil {
		return err
	}
	if err = os.Rename(stage, dir); err != nil {
		if x := check(); x != nil {
			return err
		}
	}
	if err = syncDirectory(parent); err != nil {
		return err
	}
	return check()
}

// Published role formats match the existing one-key-only camera clients.
func validKey(role string, b []byte) bool {
	switch filepath.Base(role) {
	case "x6_owner_authorized_keys":
		fields := strings.Fields(string(b))
		if len(fields) != 2 || string(b) != strings.Join(fields, " ")+"\n" {
			return false
		}
		k, e := base64.StdEncoding.DecodeString(fields[1])
		if e != nil || len(k) < 8 {
			return false
		}
		alg := fields[0]
		if alg != "ssh-ed25519" && alg != "ssh-rsa" {
			return false
		}
		if int(binary.BigEndian.Uint32(k[:4])) != len(alg) || len(k) < 4+len(alg) || string(k[4:4+len(alg)]) != alg {
			return false
		}
		if alg == "ssh-ed25519" {
			return len(k) == 4+len(alg)+4+32 && binary.BigEndian.Uint32(k[4+len(alg):]) == 32
		}
		return len(k) >= 256
	case "adb_keys":
		k, e := base64.StdEncoding.DecodeString(strings.TrimSuffix(string(b), "\n"))
		return e == nil && len(k) == 524 && binary.LittleEndian.Uint32(k) == 64
	default:
		return len(b) == 32 && !bytes.Equal(b, make([]byte, 32))
	}
}
func inherit(root string, c Config, e Evidence, verifyOnly bool) error {
	if !isHash(c.TargetSeed) {
		return fmt.Errorf("missing target seed")
	}
	mode := sourceMode(c, e.Source)
	layout, err := effectiveSource(root)
	if err != nil {
		return err
	}
	keys := map[string][]byte{}
	pins := map[string]string{}
	for _, role := range roles {
		var b []byte
		switch mode {
		case "maintenance":
			b, err = bounded(root, sourceKey(role, layout), 4096)
			if err != nil {
				return err
			}
			if sumBytes(b) != e.Source.Preserved[role] {
				return fmt.Errorf("source role changed")
			}
		case "first-install":
			b = c.InitialKeys[role]
			if len(b) == 0 {
				return fmt.Errorf("missing enrollment key")
			}
		default:
			return fmt.Errorf("unknown source")
		}
		if !validKey(role, b) {
			return fmt.Errorf("invalid source public key: %s", role)
		}
		n := filepath.Base(role)
		keys[n] = b
		pins[n] = sumBytes(b)
	}
	var pred *predecessor
	if mode == "maintenance" {
		pred, err = sourceLayout(root, c, e)
		if err != nil {
			return err
		}
	}
	keys["pins.json"], err = json.Marshal(pins)
	if err != nil {
		return err
	}
	if err = publish(root, holderDir, keys, verifyOnly); err != nil {
		return err
	}
	if pred != nil && (pred.Modified || pred.Seed != c.TargetSeed) {
		return publishTransition(root, c.TargetSeed, pred, verifyOnly)
	}
	return nil
}
