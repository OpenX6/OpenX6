// The guard reads block devices only. pre saves a small root-only snapshot in
// RAM; post compares the running slot to it and checks the inactive payload.
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed guard-config.json
var configBytes []byte

type Part struct {
	Label, Device string
	Bytes         int64
	SHA256        string
	CheckBytes    int64
}
type Config struct {
	TargetSeed                string
	InitialKeys               map[string][]byte
	InstallAllowed            bool
	MinimumDataFreeBytes      uint64
	FeatureSets               []map[string]string
	OwnerTrust                map[string]string
	PublicSHA256, AbctlSHA256 string
	Sources                   []SourceRule
	Parts                     []Part
	Targets                   []Part
}
type Evidence struct {
	Slot, BootID, PublicSHA256, AbctlSHA256 string
	Source                                  SourceEvidence
	UID                                     int
	DataFreeBytes                           uint64
	Labels                                  []string
	Devices                                 map[string]string
	Sizes                                   map[string]int64
	Hashes                                  map[string]string
}
type Snapshot struct {
	PackageID, Slot, BootID string
	SourceID                string
	Hashes                  map[string]string
}

func sumBytes(b []byte) string  { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func packageID(c Config) string { b, _ := json.Marshal(c); return sumBytes(b) }
func other(slot string) (string, error) {
	if slot == "_a" {
		return "_b", nil
	}
	if slot == "_b" {
		return "_a", nil
	}
	return "", fmt.Errorf("unknown boot slot")
}
func hashFile(path string, n int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if n > 0 {
		_, err = io.CopyN(h, f, n)
	} else {
		_, err = io.Copy(h, f)
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func isHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == strings.ToLower(s)
}
func validate(c Config, e Evidence, phase string, s *Snapshot) error {
	if phase != "inspect" && !c.InstallAllowed {
		return fmt.Errorf("candidate installation locked")
	}
	if phase != "pre" && phase != "post" && phase != "inspect" {
		return fmt.Errorf("unknown phase")
	}
	if c.MinimumDataFreeBytes < 32<<20 || c.MinimumDataFreeBytes > 2<<30 || e.DataFreeBytes < c.MinimumDataFreeBytes {
		return fmt.Errorf("insufficient data headroom or invalid space policy")
	}
	target, err := other(e.Slot)
	if err != nil {
		return err
	}
	if e.UID != 0 || e.BootID == "" {
		return fmt.Errorf("requires root and a boot identity")
	}
	if err := validateSource(c, e); err != nil {
		return err
	}
	if e.AbctlSHA256 != c.AbctlSHA256 {
		return fmt.Errorf("camera identity or abctl mismatch")
	}
	if len(c.Parts) != 64 || len(c.Targets) != 27 {
		return fmt.Errorf("bad signed partition configuration")
	}
	expected := []string{}
	parts := map[string]Part{}
	seen := map[string]bool{}
	count := 0
	for _, p := range c.Parts {
		if !strings.HasSuffix(p.Label, "_a") && !strings.HasSuffix(p.Label, "_b") {
			return fmt.Errorf("non-slot partition")
		}
		if _, ok := parts[p.Label]; ok {
			return fmt.Errorf("duplicate partition")
		}
		parts[p.Label] = p
		expected = append(expected, p.Label)
		if p.Bytes <= 0 || e.Devices[p.Label] != p.Device || e.Sizes[p.Label] != p.Bytes || seen[p.Device] {
			return fmt.Errorf("partition mapping/size mismatch: %s", p.Label)
		}
		seen[p.Device] = true
		if strings.HasSuffix(p.Label, e.Slot) {
			count++
			if !isHash(e.Hashes[p.Label]) {
				return fmt.Errorf("missing active-slot digest: %s", p.Label)
			}
		}
	}
	if count != 32 {
		return fmt.Errorf("wrong active partition count")
	}
	sort.Strings(expected)
	actual := append([]string(nil), e.Labels...)
	sort.Strings(actual)
	if strings.Join(expected, "\n") != strings.Join(actual, "\n") {
		return fmt.Errorf("A/B partition set mismatch")
	}
	for name, p := range parts {
		peer := name[:len(name)-2] + "_a"
		if strings.HasSuffix(name, "_a") {
			peer = name[:len(name)-2] + "_b"
		}
		if parts[peer].Bytes != p.Bytes {
			return fmt.Errorf("unequal slot sizes")
		}
	}
	payload := map[string]Part{}
	for _, p := range c.Targets {
		if _, ok := payload[p.Label]; ok {
			return fmt.Errorf("duplicate payload")
		}
		dst, ok := parts[p.Label+target]
		if !ok || p.Device != "" || p.CheckBytes <= 0 || p.CheckBytes > dst.Bytes || !isHash(p.SHA256) {
			return fmt.Errorf("invalid target payload")
		}
		payload[p.Label] = p
	}
	if _, ok := payload["system"]; !ok {
		return fmt.Errorf("missing system payload")
	}
	if s != nil {
		if s.PackageID != packageID(c) || s.Slot != e.Slot || s.BootID != e.BootID || s.SourceID != sourceID(e) || len(s.Hashes) != 32 {
			return fmt.Errorf("snapshot identity/slot/boot mismatch")
		}
		for _, p := range c.Parts {
			if strings.HasSuffix(p.Label, e.Slot) && e.Hashes[p.Label] != s.Hashes[p.Label] {
				return fmt.Errorf("active slot changed: %s", p.Label)
			}
		}
	}
	if phase == "post" {
		if s == nil {
			return fmt.Errorf("missing pre-install snapshot")
		}
		for _, p := range c.Parts {
			if strings.HasSuffix(p.Label, target) {
				base := strings.TrimSuffix(p.Label, target)
				want := s.Hashes[base+e.Slot]
				if x, ok := payload[base]; ok {
					want = x.SHA256
				}
				if e.Hashes["target:"+p.Label] != want {
					return fmt.Errorf("target readback mismatch: %s", p.Label)
				}
			}
		}
	}
	return nil
}
func collectPrograms(root string, c Config, e *Evidence) error {
	if isHash(c.TargetSeed) && sourceMode(c, e.Source) == "first-install" {
		// Normalize enrollment evidence before and after publishing initial keys.
		for _, role := range roles {
			if e.Source.Preserved[role] == "" {
				e.Source.Preserved[role] = sumBytes(c.InitialKeys[role])
			}
		}
		e.Source.RetainedHolder = true
		e.Source.OwnerPublicSHA256 = e.Source.Preserved["ins/etc/x6_owner_update_ed25519.pub"]
		plan, err := stockRestore(root, c, e.Source)
		if err != nil {
			return err
		}
		if plan != nil {
			raw, _ := json.Marshal(plan)
			e.Source.Preserved["stock-reinstall-plan"] = sumBytes(raw)
		}
		return nil
	}
	if c.TargetSeed != "" && sourceMode(c, e.Source) == "maintenance" {
		pr, err := sourceLayout(root, c, *e)
		if err != nil {
			return err
		}
		if e.DataFreeBytes < c.MinimumDataFreeBytes+pr.extraBytes {
			return fmt.Errorf("insufficient space for installed program snapshots")
		}
		for _, v := range pr.Entries {
			e.Source.Preserved["data/x6-owner-features/"+v.Target] = v.SHA256
		}
		return nil
	}
	_, err := os.Lstat(filepath.Join(root, "data/x6-owner-features"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	features, err := checkFeatureFilesAt(root, c.FeatureSets)
	if err != nil {
		return err
	}
	for p, h := range features {
		e.Source.Preserved[p] = h
	}
	return nil
}

func collect(c Config, phase string) (Evidence, error) {
	e := Evidence{UID: os.Geteuid(), Devices: map[string]string{}, Sizes: map[string]int64{}, Hashes: map[string]string{}}
	var err error
	var dataFS syscall.Statfs_t
	if err = syscall.Statfs("/data", &dataFS); err != nil {
		return e, err
	}
	e.DataFreeBytes = uint64(dataFS.Bavail) * uint64(dataFS.Bsize)
	if e.Source, err = collectSource("/"); err != nil {
		return e, err
	}
	e.AbctlSHA256, err = hashFile("/sbin/abctl", 0)
	if err != nil {
		return e, err
	}
	if e.AbctlSHA256 != c.AbctlSHA256 {
		return e, fmt.Errorf("unexpected abctl")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/sbin/abctl", "--boot_slot").Output()
	if err != nil {
		return e, err
	}
	e.Slot = strings.TrimSpace(string(b))
	target, err := other(e.Slot)
	if err != nil || e.UID != 0 {
		return e, fmt.Errorf("invalid slot/UID")
	}
	b, err = os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return e, err
	}
	e.BootID = strings.TrimSpace(string(b))
	if h, ok := e.Source.Preserved["data/x6-owner-gui/server_ed25519.pub"]; ok {
		e.PublicSHA256 = h
	}
	if err = collectPrograms("/", c, &e); err != nil {
		return e, err
	}
	if err = validateSource(c, e); err != nil {
		return e, err
	}
	dir := "/dev/block/bootdevice/by-name"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return e, err
	}
	for _, x := range entries {
		if strings.HasSuffix(x.Name(), "_a") || strings.HasSuffix(x.Name(), "_b") {
			e.Labels = append(e.Labels, x.Name())
		}
	}
	payload := map[string]Part{}
	for _, p := range c.Targets {
		payload[p.Label] = p
	}
	for _, p := range c.Parts {
		real, err := filepath.EvalSymlinks(filepath.Join(dir, p.Label))
		if err != nil {
			return e, err
		}
		e.Devices[p.Label] = real
		if real != p.Device {
			return e, fmt.Errorf("wrong mapping: %s", p.Label)
		}
		st, err := os.Stat(real)
		if err != nil {
			return e, err
		}
		if st.Mode()&os.ModeDevice == 0 || st.Mode()&os.ModeCharDevice != 0 {
			return e, fmt.Errorf("not a block device")
		}
		b, err := os.ReadFile("/sys/class/block/" + filepath.Base(real) + "/size")
		if err != nil {
			return e, err
		}
		sectors, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err != nil {
			return e, err
		}
		e.Sizes[p.Label] = sectors * 512
		if e.Sizes[p.Label] != p.Bytes {
			return e, fmt.Errorf("wrong partition size")
		}
		if strings.HasSuffix(p.Label, e.Slot) {
			e.Hashes[p.Label], err = hashFile(real, p.Bytes)
			if err != nil {
				return e, err
			}
		}
		if phase == "post" && strings.HasSuffix(p.Label, target) {
			n := p.Bytes
			if x, ok := payload[strings.TrimSuffix(p.Label, target)]; ok {
				n = x.CheckBytes
			}
			if n <= 0 || n > p.Bytes {
				return e, fmt.Errorf("invalid readback size")
			}
			e.Hashes["target:"+p.Label], err = hashFile(real, n)
			if err != nil {
				return e, err
			}
		}
	}
	return e, nil
}
func checkDir(dir string) error {
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.IsDir() || st.Mode().Perm() != 0700 || int(s.Uid) != os.Geteuid() {
		return fmt.Errorf("unsafe snapshot directory")
	}
	return nil
}
func readSnapshot(dir string) (*Snapshot, error) {
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "source.json"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || int(s.Uid) != os.Geteuid() || s.Nlink != 1 || st.Size() > 32768 {
		return nil, fmt.Errorf("unsafe snapshot file")
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var snapshot Snapshot
	if err = json.Unmarshal(b, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}
func saveSnapshot(dir string, c Config, e Evidence) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if err := checkDir(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, "source.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if os.IsExist(err) {
		old, err := readSnapshot(dir)
		if err != nil {
			return err
		}
		return validate(c, e, "pre", old)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	s := Snapshot{PackageID: packageID(c), Slot: e.Slot, BootID: e.BootID, SourceID: sourceID(e), Hashes: map[string]string{}}
	for _, p := range c.Parts {
		if strings.HasSuffix(p.Label, e.Slot) {
			s.Hashes[p.Label] = e.Hashes[p.Label]
		}
	}
	if err = json.NewEncoder(f).Encode(s); err != nil {
		return err
	}
	return f.Sync()
}
func main() {
	time.AfterFunc(5*time.Minute, func() { fmt.Fprintln(os.Stderr, "guard read timeout"); os.Exit(75) })
	var c Config
	if err := json.Unmarshal(configBytes, &c); err != nil {
		panic(err)
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: x6-install-guard inspect|pre|post")
		os.Exit(2)
	}
	phase := os.Args[1]
	if phase != "inspect" && !c.InstallAllowed {
		fmt.Fprintln(os.Stderr, "REFUSED: candidate installation locked")
		os.Exit(1)
	}
	if phase != "pre" && phase != "post" && phase != "inspect" {
		fmt.Fprintln(os.Stderr, "unknown phase")
		os.Exit(2)
	}
	dir := "/tmp/x6-owner-install-" + packageID(c)[:24]
	var err error
	var snapshot *Snapshot
	if phase != "inspect" {
		var fs syscall.Statfs_t
		err = syscall.Statfs("/tmp", &fs)
		if err == nil && uint64(fs.Type) != 0x01021994 && uint64(fs.Type) != 0x858458f6 {
			err = fmt.Errorf("snapshot location is not RAM")
		}
	}
	if err == nil && phase == "post" {
		snapshot, err = readSnapshot(dir)
	}
	var e Evidence
	if err == nil {
		e, err = collect(c, phase)
	}
	if err == nil {
		err = validate(c, e, phase, snapshot)
	}
	if err == nil && phase == "pre" {
		err = saveSnapshot(dir, c, e)
	}
	if err == nil && c.TargetSeed != "" && (phase == "pre" || phase == "post") {
		err = inherit("/", c, e, phase == "post")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "REFUSED:", err)
		os.Exit(1)
	}
	target, _ := other(e.Slot)
	json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": phase, "passed": true, "slot": e.Slot, "target": target, "block_writes": false, "RAM_snapshot_written": phase == "pre", "snapshot_directory": dir, "evidence": e})
}
