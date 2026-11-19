// Offline first-install filesystem engine. No network, service, mount, OTA or
// block-device operations. The CLI deliberately accepts only marked fixtures.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const seedPath = "usr/share/x6-firstboot-seed"
const featurePath = "data/x6-owner-features"
const maxFile = 160 << 20
const maxBlob = 16 << 20

type Chunk struct {
	Offset     int64
	BlobOffset int64
	Length     int64
}
type Entry struct {
	Target     string
	Base       string
	BaseSHA256 string
	BaseBytes  int64
	SHA256     string
	Bytes      int64
	Mode       uint32
	Chunks     []Chunk
}
type Manifest struct {
	Version    int
	BlobSHA256 string
	BlobBytes  int64
	Entries    []Entry
}
type policy struct {
	base string
	mode uint32
}

var policies = map[string]policy{
	"vr180-v1/gui":            {"usr/bin/ins_gui", 0755},
	"vr180-v1/setup":          {"usr/local/sbin/x6-owner-gui-setup", 0755},
	"vr180-v1/sync.so":        {"usr/lib/hw/com.qti.chi.override.so", 0644},
	"vr180-v1/backup/gui":     {"usr/bin/ins_gui", 0600},
	"vr180-v1/backup/setup":   {"usr/local/sbin/x6-owner-gui-setup", 0600},
	"vr180-v1/backup/sync.so": {"usr/lib/hw/com.qti.chi.override.so", 0600},
	"raw-nr-v1/medium.so":     {"usr/lib/libbva_camera.so", 0644},
	"raw-nr-v1/low.so":        {"usr/lib/libbva_camera.so", 0644},
	"raw-nr-v1/minimal.so":    {"usr/lib/libbva_camera.so", 0644},
}
var defaults = map[string]byte{"vr180-v1/enabled": '1', "vr180-v1/sync": 1, "raw-nr-v1/enabled": '1', "raw-nr-v1/level": 0}
var dirs = []string{"vr180-v1", "vr180-v1/backup", "raw-nr-v1"}

// seedTop are the namespace entries this seed owns. Other top-level directories
// belong to runtime preference helpers and are never touched.
var seedTop = []string{"vr180-v1", "raw-nr-v1"}

type Engine struct {
	Root, Pin    string
	Migration    *MigrationPolicy
	ReplaceLabel func(*os.File, *os.File) error
	// Only tests inject faults or replace the free-space observation.
	Fault func(string) error
	Free  func(string) (uint64, error)
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == strings.ToLower(s)
}
func (e Engine) hit(s string) error {
	if e.Fault != nil {
		return e.Fault(s)
	}
	return nil
}
func (e Engine) path(rel string) string { return filepath.Join(e.Root, rel) }
func safeRel(s string) bool {
	return s != "" && !filepath.IsAbs(s) && filepath.Clean(s) == s && s != "." && s != ".." && !strings.HasPrefix(s, "../")
}

// All mutable parent directories must be owner-controlled. This engine's threat
// model excludes another malicious root concurrently changing those directories.
func metadata(p string, dir bool, exact os.FileMode) error {
	st, err := os.Lstat(p)
	if err != nil {
		return err
	}
	ss, ok := st.Sys().(*syscall.Stat_t)
	if !ok || ss.Uid != uint32(os.Geteuid()) || st.Mode().Perm()&0022 != 0 || st.Mode()&(os.ModeSymlink|os.ModeSetuid|os.ModeSetgid) != 0 {
		return fmt.Errorf("unsafe metadata: %s", p)
	}
	if dir {
		if !st.IsDir() {
			return fmt.Errorf("not directory: %s", p)
		}
	} else if !st.Mode().IsRegular() || ss.Nlink != 1 {
		return fmt.Errorf("not single-link regular file: %s", p)
	}
	if exact != 0 && st.Mode().Perm() != exact {
		return fmt.Errorf("unexpected mode: %s", p)
	}
	return nil
}
func (e Engine) parents(rel string) error {
	if !safeRel(rel) {
		return errors.New("invalid path")
	}
	if err := metadata(e.Root, true, 0); err != nil {
		return err
	}
	parts := strings.Split(filepath.Dir(rel), string(os.PathSeparator))
	cur := e.Root
	for _, part := range parts {
		if part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		if err := metadata(cur, true, 0); err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) open(rel string, mode os.FileMode) (*os.File, error) {
	if err := e.parents(rel); err != nil {
		return nil, err
	}
	p := e.path(rel)
	if err := metadata(p, false, mode); err != nil {
		return nil, err
	}
	prior, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), p)
	now, err := f.Stat()
	if err != nil || !os.SameFile(prior, now) {
		f.Close()
		return nil, errors.New("file replaced while opening")
	}
	return f, nil
}
func (e Engine) read(rel string, max int64, mode os.FileMode) ([]byte, error) {
	f, err := e.open(rel, mode)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > max {
		return nil, errors.New("file exceeds bound")
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}
func (e Engine) hash(rel string, size int64, want string, mode os.FileMode) error {
	f, err := e.open(rel, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() != size {
		return fmt.Errorf("size mismatch: %s", rel)
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fmt.Errorf("hash mismatch: %s", rel)
	}
	return nil
}
func validate(m Manifest) error {
	if m.Version != 1 || len(m.Entries) != len(policies) || m.BlobBytes < 0 || m.BlobBytes > maxBlob || !validHash(m.BlobSHA256) {
		return errors.New("manifest bounds/version")
	}
	seen := map[string]bool{}
	var cursor int64
	for _, v := range m.Entries {
		p, ok := policies[v.Target]
		if !ok || seen[v.Target] || v.Base != p.base || v.Mode != p.mode {
			return errors.New("entry not allowlisted or duplicate")
		}
		seen[v.Target] = true
		if !validHash(v.SHA256) || !validHash(v.BaseSHA256) || v.Bytes <= 0 || v.Bytes > maxFile || v.BaseBytes <= 0 || v.BaseBytes > maxFile || len(v.Chunks) > maxFile/4096 {
			return errors.New("entry bounds")
		}
		var end int64
		for _, c := range v.Chunks {
			if c.Offset < end || c.Offset < 0 || c.Offset%4096 != 0 || c.Offset >= v.Bytes || c.Length <= 0 || c.Length > 4096 || c.Length > v.Bytes-c.Offset || c.BlobOffset != cursor || c.Length > m.BlobBytes-cursor {
				return errors.New("chunk overlap/order/bounds")
			}
			if c.Length != 4096 && c.Offset+c.Length != v.Bytes {
				return errors.New("short interior chunk")
			}
			end = c.Offset + c.Length
			cursor += c.Length
		}
		if strings.Contains(v.Target, "/backup/") && (len(v.Chunks) != 0 || v.SHA256 != v.BaseSHA256 || v.Bytes != v.BaseBytes) {
			return errors.New("backup must be exact base")
		}
	}
	if cursor != m.BlobBytes {
		return errors.New("unreferenced blob data")
	}
	return nil
}
func (e Engine) load() (Manifest, []byte, error) {
	var m Manifest
	b, err := e.read(seedPath+"/manifest.json", 1<<20, 0644)
	if err != nil {
		return m, nil, err
	}
	if !validHash(e.Pin) || digest(b) != e.Pin {
		return m, nil, errors.New("unpinned manifest")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&m); err != nil {
		return m, nil, err
	}
	var extra interface{}
	if dec.Decode(&extra) != io.EOF {
		return m, nil, errors.New("trailing JSON")
	}
	if err = validate(m); err != nil {
		return m, nil, err
	}
	blob, err := e.read(seedPath+"/delta.bin", maxBlob, 0644)
	if err != nil {
		return m, nil, err
	}
	if int64(len(blob)) != m.BlobBytes || digest(blob) != m.BlobSHA256 {
		return m, nil, errors.New("delta integrity")
	}
	return m, blob, nil
}
func syncDir(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (e Engine) makeDir(rel string) error {
	if err := e.parents(rel); err != nil {
		return err
	}
	p := e.path(rel)
	if err := os.Mkdir(p, 0700); err != nil {
		if !os.IsExist(err) {
			return err
		}
		return metadata(p, true, 0700)
	}
	return syncDir(filepath.Dir(p))
}
func (e Engine) lock() (func(), error) {
	rel := "data/.x6-firstboot.lock"
	if err := e.parents(rel); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(e.path(rel), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), rel)
	if err = metadata(e.path(rel), false, 0600); err != nil {
		f.Close()
		return nil, err
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("initializer busy")
	}
	return func() { syscall.Flock(fd, syscall.LOCK_UN); f.Close() }, nil
}
func (e Engine) stageTree(stage string) error {
	allowed := map[string]bool{".": true, ".writing": true, "wifi-minutes": true, "complete.json": true}
	for _, s := range dirs {
		allowed[s] = true
	}
	for s := range policies {
		allowed[s] = true
	}
	for s := range defaults {
		allowed[s] = true
	}
	for _, pref := range captureDefaults {
		allowed[filepath.Dir(pref.path)] = true
		allowed[pref.path] = true
	}
	return filepath.Walk(e.path(stage), func(p string, st os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(e.path(stage), p)
		if err != nil {
			return err
		}
		if !allowed[rel] {
			return fmt.Errorf("unknown staged path: %s", rel)
		}
		return metadata(p, st.IsDir(), 0)
	})
}
func (e Engine) write(stage, rel string, mode os.FileMode, fill func(*os.File) error) error {
	tmp := stage + "/.writing"
	p := e.path(tmp)
	if _, err := os.Lstat(p); err == nil {
		if err = metadata(p, false, 0); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	fd, err := syscall.Open(p, syscall.O_CREAT|syscall.O_TRUNC|syscall.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), tmp)
	if err = fill(f); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err = e.hit("synced:" + rel); err != nil {
		return err
	}
	if err = os.Rename(p, e.path(stage+"/"+rel)); err != nil {
		return err
	}
	if err = syncDir(e.path(filepath.Dir(stage + "/" + rel))); err != nil {
		return err
	}
	if err = syncDir(e.path(stage)); err != nil {
		return err
	}
	return e.hit("file:" + rel)
}
func (e Engine) verifyFinal(m Manifest) error {
	if err := metadata(e.path(featurePath), true, 0700); err != nil {
		return err
	}
	b, err := e.read(featurePath+"/complete.json", 128, 0600)
	if err != nil || string(b) != e.Pin+"\n" {
		return errors.New("existing feature namespace not owned by this seed")
	}
	return e.verifyContent(m)
}

// verifyTop checks every manifest entry below one of our top-level directories.
func (e Engine) verifyTop(m Manifest, top string) error {
	for _, v := range m.Entries {
		if !strings.HasPrefix(v.Target, top+"/") {
			continue
		}
		mode := os.FileMode(v.Mode)
		// Historical installations kept this read-only library private (0600).
		// Accept that stricter mode without changing it, only for this exact target.
		if v.Target == "vr180-v1/sync.so" {
			if st, err := os.Lstat(e.path(featurePath + "/" + v.Target)); err == nil && st.Mode().Perm() == 0600 {
				mode = 0600
			}
		}
		if err := e.hash(featurePath+"/"+v.Target, v.Bytes, v.SHA256, mode); err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) verifyContent(m Manifest) error {
	if err := metadata(e.path(featurePath), true, 0700); err != nil {
		return err
	}
	for _, top := range seedTop {
		if err := e.verifyTop(m, top); err != nil {
			return err
		}
	}
	return e.verifyPreferences()
}

type namespaceKind int

const (
	// An existing installation or content this engine does not understand:
	// the adoption and migration paths decide, exactly as before.
	namespaceUnknown namespaceKind = iota
	// Only other components' directories (runtime preference helpers may create
	// the namespace before this seed publishes on a fresh install).
	namespaceForeign
	// Our directories are present, verified and unreceipted: a merge publish of
	// this seed was interrupted, or an exact installation lacks its receipt.
	namespaceMergeable
)

// classify looks at an unreceipted namespace. It never fails the run: whatever
// it cannot place is left to the adoption/migration paths.
func (e Engine) classify(m Manifest) namespaceKind {
	if metadata(e.path(featurePath), true, 0700) != nil {
		return namespaceUnknown
	}
	entries, err := os.ReadDir(e.path(featurePath))
	if err != nil {
		return namespaceUnknown
	}
	present := []string{}
	for _, d := range entries {
		name := d.Name()
		if strings.HasPrefix(name, ".") || !d.IsDir() {
			return namespaceUnknown // receipts, markers and temporary files are not ours to reinterpret
		}
		for _, top := range seedTop {
			if top == name {
				present = append(present, name)
			}
		}
	}
	if len(present) == 0 {
		return namespaceForeign
	}
	for _, top := range present {
		if e.verifyTop(m, top) != nil {
			return namespaceUnknown
		}
	}
	return namespaceMergeable
}

// removeStage deletes only this seed's own staging tree, after checking that
// nothing but staging content lives there.
func (e Engine) removeStage(stage string) error {
	if _, err := os.Lstat(e.path(stage)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := e.stageTree(stage); err != nil {
		return err
	}
	if err := os.RemoveAll(e.path(stage)); err != nil {
		return err
	}
	if err := syncDir(e.path("data")); err != nil {
		return err
	}
	return e.hit("stage-removed")
}

// mergePublish publishes into a namespace another component created first, or
// resumes an interrupted merge. Our directories move in one at a time; the
// receipt moves last, so a receipted namespace always holds every payload.
func (e Engine) mergePublish(stage string, m Manifest) error {
	for _, pref := range captureDefaults {
		if err := e.publishAbsent(stage, pref.path, featurePath+"/"+pref.path); err != nil {
			return err
		}
	}
	for _, top := range seedTop {
		dst := featurePath + "/" + top
		if _, err := os.Lstat(e.path(dst)); err == nil {
			if err = e.verifyTop(m, top); err != nil {
				return err
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(e.path(stage+"/"+top), e.path(dst)); err != nil {
			return err
		}
		if err := syncDir(e.path(featurePath)); err != nil {
			return err
		}
		if err := e.hit("merged:" + top); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(e.path(featurePath + "/complete.json")); !os.IsNotExist(err) {
		return errors.New("receipt appeared")
	}
	if err := os.Rename(e.path(stage+"/complete.json"), e.path(featurePath+"/complete.json")); err != nil {
		return err
	}
	if err := syncDir(e.path(featurePath)); err != nil {
		return err
	}
	if err := e.hit("published"); err != nil {
		return err
	}
	if err := e.removeStage(stage); err != nil {
		return err
	}
	return e.finish(m)
}

// An existing installation with exactly this payload may lack our new receipt.
// Adopt only after verifying every payload and preference; never reset them.
// Package/owner authorization is external to this filesystem engine.
func (e Engine) adoptExisting(m Manifest) error {
	if err := e.verifyContent(m); err != nil {
		return err
	}
	// Existing modified installations must not receive factory first-use defaults.
	if err := e.preserveExistingSettings(); err != nil {
		return err
	}
	if err := e.hit("adopt-settings-preserved"); err != nil {
		return err
	}
	f, err := os.CreateTemp(e.path(featurePath), ".firstboot-receipt-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.WriteString(e.Pin + "\n")
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err = e.hit("adopt-before-publish"); err != nil {
		return err
	}
	final := e.path(featurePath + "/complete.json")
	if _, x := os.Lstat(final); !os.IsNotExist(x) {
		return errors.New("receipt appeared")
	}
	if err = os.Rename(tmp, final); err != nil {
		return err
	}
	if err = syncDir(e.path(featurePath)); err != nil {
		return err
	}
	if err = e.hit("adopt-published"); err != nil {
		return err
	}
	return e.finish(m)
}

func (e Engine) preserveExistingSettings() error {
	rel := featurePath + "/preserve-native-settings"
	if b, err := e.read(rel, 128, 0600); err == nil {
		if string(b) == e.Pin+"\n" {
			return nil
		}
		return errors.New("unknown existing-settings receipt")
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(e.path(featurePath), ".settings-preserve-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.WriteString(e.Pin + "\n")
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if _, err = os.Lstat(e.path(rel)); !os.IsNotExist(err) {
		return errors.New("settings receipt appeared")
	}
	if err = os.Rename(tmp, e.path(rel)); err != nil {
		return err
	}
	return syncDir(e.path(featurePath))
}
func (e Engine) Run() error {
	if !filepath.IsAbs(e.Root) || filepath.Clean(e.Root) != e.Root {
		return errors.New("invalid root")
	}
	if err := metadata(e.Root, true, 0); err != nil {
		return err
	}
	if err := metadata(e.path("data"), true, 0); err != nil {
		return err
	}
	m, blob, err := e.load()
	if err != nil {
		return err
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if e.Migration != nil && (e.Migration.LocalHolder || len(e.Migration.Owners) > 0) {
		if err = e.overlayGuard(); err != nil {
			return err
		}
	}
	if handled, x := e.restoreStock(m, blob); handled || x != nil {
		return x
	}
	stage := "data/.x6-firstboot-stage-" + e.Pin
	if _, err = os.Lstat(e.path(featurePath)); err == nil {
		if _, x := os.Lstat(e.path(featurePath + "/complete.json")); os.IsNotExist(x) {
			kind := e.classify(m)
			_, staged := os.Lstat(e.path(stage))
			// Another component created the namespace before this seed published
			// (fresh-install race), or our own merge publish was interrupted.
			if kind == namespaceForeign || (kind == namespaceMergeable && staged == nil) {
				return e.install(m, blob, stage)
			}
			if e.verifyContent(m) == nil {
				return e.adoptExisting(m)
			}
			return e.migrate(m, blob)
		}
		if e.verifyFinal(m) == nil {
			if x := e.removeStage(stage); x != nil {
				fmt.Fprintln(os.Stderr, "OpenX6 stale staging tree kept:", x)
			}
			return e.finish(m)
		}
		return e.migrate(m, blob)
	} else if !os.IsNotExist(err) {
		return err
	}
	return e.install(m, blob, stage)
}

func (e Engine) stagePayloads(m Manifest, blob []byte, stage string) error {
	var err error
	// Check all source bytes before any feature output is created.
	for _, v := range m.Entries {
		if err = e.hash(v.Base, v.BaseBytes, v.BaseSHA256, 0); err != nil {
			return err
		}
	}
	var need uint64 = 16 << 20
	for _, v := range m.Entries {
		need += uint64(v.Bytes)
	}
	need += maxFile
	available := e.Free
	if available == nil {
		available = freeBytes
	}
	free, err := available(e.path("data"))
	if err != nil {
		return err
	}
	if free < need {
		return errors.New("insufficient staging space")
	}
	if err = e.makeDir(stage); err != nil {
		return err
	}
	if err = e.stageTree(stage); err != nil {
		return err
	}
	if err = e.hit("stage"); err != nil {
		return err
	}
	for _, d := range dirs {
		if err = e.makeDir(stage + "/" + d); err != nil {
			return err
		}
	}
	for _, v := range m.Entries {
		if _, err = os.Lstat(e.path(stage + "/" + v.Target)); err == nil {
			if err = e.hash(stage+"/"+v.Target, v.Bytes, v.SHA256, os.FileMode(v.Mode)); err != nil {
				return err
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		err = e.write(stage, v.Target, os.FileMode(v.Mode), func(out *os.File) error {
			src, x := e.open(v.Base, 0)
			if x != nil {
				return x
			}
			_, x = io.Copy(out, src)
			src.Close()
			if x != nil {
				return x
			}
			if x = out.Truncate(v.Bytes); x != nil {
				return x
			}
			if x = e.hit("base:" + v.Target); x != nil {
				return x
			}
			for i, c := range v.Chunks {
				if _, x = out.WriteAt(blob[c.BlobOffset:c.BlobOffset+c.Length], c.Offset); x != nil {
					return x
				}
				if x = e.hit(fmt.Sprintf("chunk:%s:%d", v.Target, i)); x != nil {
					return x
				}
			}
			// Verify through a separately opened descriptor before publication.
			return e.hash(stage+"/.writing", v.Bytes, v.SHA256, 0)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// install stages every payload, then publishes: by one rename when nothing
// exists at the destination, otherwise by merging into what another component
// created there (only its own directories are ever moved).
func (e Engine) install(m Manifest, blob []byte, stage string) error {
	if err := e.stagePayloads(m, blob, stage); err != nil {
		return err
	}
	var err error
	// Defaults are written only into unpublished staging, never over saved choices.
	for _, rel := range []string{"vr180-v1/sync", "raw-nr-v1/level", "vr180-v1/enabled", "raw-nr-v1/enabled"} {
		b := []byte{defaults[rel]}
		if err = e.write(stage, rel, 0600, func(f *os.File) error { _, x := f.Write(b); return x }); err != nil {
			return err
		}
	}
	for _, pref := range captureDefaults {
		if err = e.makeDir(stage + "/" + filepath.Dir(pref.path)); err != nil {
			return err
		}
		if err = e.saveBytes(stage, pref.path, []byte(pref.value), 0600); err != nil {
			return err
		}
	}
	if err = e.saveBytes(stage, "wifi-minutes", []byte{2}, 0600); err != nil {
		return err
	}
	if err = e.publishAbsent(stage, "wifi-minutes", "data/x6-wifi-idle/minutes"); err != nil {
		return err
	}
	if err = e.write(stage, "complete.json", 0600, func(f *os.File) error { _, x := f.WriteString(e.Pin + "\n"); return x }); err != nil {
		return err
	}
	// All files and child directories are durable before the single publication.
	for _, d := range []string{"vr180-v1/backup", "vr180-v1", "raw-nr-v1", ""} {
		if err = syncDir(e.path(filepath.Join(stage, d))); err != nil {
			return err
		}
	}
	if err = e.hit("before-publish"); err != nil {
		return err
	}
	if _, x := os.Lstat(e.path(featurePath)); x == nil {
		if e.classify(m) == namespaceUnknown {
			return errors.New("feature destination appeared")
		}
		return e.mergePublish(stage, m)
	} else if !os.IsNotExist(x) {
		return x
	}
	if err = os.Rename(e.path(stage), e.path(featurePath)); err != nil {
		return err
	}
	if err = e.hit("after-rename"); err != nil {
		return err
	}
	if err = syncDir(e.path("data")); err != nil {
		return err
	}
	if err = e.hit("published"); err != nil {
		return err
	}
	return e.finish(m)
}
func freeBytes(p string) (uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(p, &s); err != nil {
		return 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), nil
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "prepare" || os.Args[1] == "settings") {
		if err := targetEntry(os.Args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	root := flag.String("fixture-root", "", "marked offline fixture only; never /")
	pin := flag.String("manifest-sha256", "", "required trusted manifest digest for offline fixture")
	flag.Parse()
	if *root == "" || *root == "/" {
		fmt.Fprintln(os.Stderr, "offline fixtures only; production activation not implemented")
		os.Exit(2)
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		panic(err)
	}
	e, err := targetEngine(abs, *pin)
	if err != nil {
		panic(err)
	}
	marker, err := e.read(".x6-offline-fixture", 64, 0600)
	if err != nil || string(marker) != "OFFLINE TEST ROOT\n" {
		fmt.Fprintln(os.Stderr, "missing fixture marker")
		os.Exit(2)
	}
	if err = e.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("OFFLINE_FEATURE_INITIALIZATION_VERIFIED")
}
