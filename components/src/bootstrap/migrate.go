package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"openx6.local/components/holder"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migration-policy.json
var migrationPolicy []byte

type Snapshot struct {
	Target, SHA256 string
	Bytes          int64
	Mode           uint32
}
type OldSSH struct {
	SHA256 string
	Bytes  int64
}
type Predecessor struct {
	Version     int
	Seed        string
	Owners      map[string]string
	Entries     []Snapshot
	LegacySSH   *OldSSH
	Modified    bool `json:",omitempty"`
	Receiptless bool
}
type MigrationPolicy struct {
	LocalHolder bool
	Owners      map[string]string
	Previous    []Predecessor
}
type transaction struct {
	Version                     int
	From, To, Receipt, Preserve string
	Legacy                      bool
}
type replacement struct {
	Path     string
	Old, New Snapshot
	Source   string
}

const legacyUnit = "etc/systemd/system/x6-owner-maintenance.service"
const legacyUnitSHA = "57c9b143941ab9903b0ba957195686cc98ec57046bb1f3cea45b206c78fd34a9"
const legacyExec = "data/x6-owner-maintenance/ssh"

func targetEngine(root, pin string) (Engine, error) {
	var p MigrationPolicy
	if err := json.Unmarshal(migrationPolicy, &p); err != nil {
		return Engine{}, err
	}
	e := Engine{Root: root, Pin: pin, Migration: &p}
	if p.LocalHolder {
		if !validHash(pin) {
			return Engine{}, errors.New("invalid target seed")
		}
		raw, err := e.read("data/x6-inherit-"+pin+"/previous.json", 16384, 0600)
		if err == nil {
			var prior Predecessor
			if json.Unmarshal(raw, &prior) != nil || !validHash(prior.Seed) {
				return Engine{}, errors.New("invalid local predecessor")
			}
			p.Previous = []Predecessor{prior}
		} else if !os.IsNotExist(err) {
			return Engine{}, err
		}
	}
	return e, nil
}
func (e Engine) optional(rel string) (string, error) {
	b, err := e.read(rel, 128, 0600)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(b), err
}
func (e Engine) verifyPreferences() error {
	for rel := range defaults {
		b, err := e.read(featurePath+"/"+rel, 1, 0600)
		if os.IsNotExist(err) && strings.HasSuffix(rel, "/enabled") {
			continue
		} // Preserve disabled features.
		if err != nil || len(b) != 1 {
			return errors.New("invalid saved preference")
		}
		switch rel {
		case "vr180-v1/sync":
			if b[0] > 1 {
				return errors.New("invalid sync")
			}
		case "raw-nr-v1/level":
			if b[0] > 3 {
				return errors.New("invalid NR level")
			}
		default:
			if b[0] != '1' {
				return errors.New("invalid enabled flag")
			}
		}
	}
	return nil
}
func (e Engine) verifyOwners(p Predecessor) error {
	if e.Migration != nil && e.Migration.LocalHolder {
		if len(p.Owners) != 5 {
			return errors.New("missing previous owner roles")
		}
		for _, name := range []string{holder.SSH, holder.GUI, holder.Factory, holder.Update, holder.ADB} {
			role := "ins/etc/" + name
			if name == holder.ADB {
				role = name
			}
			b, err := holder.Read(e.Root, name)
			if err != nil || digest(b) != p.Owners[role] {
				return errors.New("retained owner mismatch")
			}
		}
		return nil
	}
	if e.Migration == nil || len(p.Owners) != 5 || len(e.Migration.Owners) != 5 {
		return errors.New("owner policy missing")
	}
	for name, h := range e.Migration.Owners {
		if !safeRel(name) || p.Owners[name] != h || !validHash(h) {
			return errors.New("holder changed")
		}
		b, err := e.read(name, 4096, 0)
		if err != nil {
			return err
		}
		if digest(b) != h {
			return errors.New("installed owner key differs")
		}
	}
	return nil
}
func (e Engine) legacyState(p Predecessor) (bool, error) {
	if _, err := os.Lstat(e.path(legacyUnit)); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	b, err := e.read(legacyUnit, 16384, 0644)
	if err != nil {
		return false, err
	}
	if digest(b) != legacyUnitSHA || p.LegacySSH == nil {
		return false, errors.New("unrecognized legacy SSH override")
	}
	b, err = e.read("data/x6-owner-maintenance/owner.pub", 4096, 0)
	if err != nil {
		return false, err
	}
	f := strings.Fields(string(b))
	if len(f) < 2 || len(f) > 3 {
		return false, errors.New("legacy SSH key format")
	}
	if digest([]byte(strings.Join(f[:2], " ")+"\n")) != p.Owners["ins/etc/x6_owner_authorized_keys"] {
		return false, errors.New("legacy SSH holder changed")
	}
	return true, nil
}
func (e Engine) migrationItems(p Predecessor, m Manifest, legacy bool) ([]replacement, error) {
	old := map[string]Snapshot{}
	items := []replacement{}
	if p.Version != 1 || !validHash(p.Seed) || len(p.Entries) != len(policies) {
		return nil, errors.New("invalid predecessor")
	}
	for _, v := range p.Entries {
		bounded := v.Bytes > 0 && v.Bytes <= maxFile
		if p.Modified {
			bounded = v.Bytes >= 0 && v.Bytes <= 1<<50
		}
		if _, ok := policies[v.Target]; !ok || old[v.Target].SHA256 != "" || !bounded || !validHash(v.SHA256) || v.Mode&^0777 != 0 || v.Mode&0022 != 0 || v.Mode&0400 == 0 {
			return nil, errors.New("invalid old entry")
		}
		old[v.Target] = v
	}
	for _, v := range m.Entries {
		o := old[v.Target]
		items = append(items, replacement{featurePath + "/" + v.Target, o, Snapshot{v.Target, v.SHA256, v.Bytes, v.Mode}, ""})
	}
	if legacy {
		b, err := e.read("usr/local/sbin/x6-owner-maintenance", 32<<20, 0755)
		if err != nil {
			return nil, err
		}
		items = append(items, replacement{legacyExec, Snapshot{"ssh", p.LegacySSH.SHA256, p.LegacySSH.Bytes, 0755}, Snapshot{"ssh", digest(b), int64(len(b)), 0755}, "usr/local/sbin/x6-owner-maintenance"})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	// Historical data copies may be stricter than image modes. Preserve the
	// original archive mode across a resume after the active file was replaced.
	for i := range items {
		mode := os.FileMode(0)
		if items[i].Path == legacyExec {
			mode = 0700
		}
		if !p.Modified && items[i].Old.Target == "vr180-v1/sync.so" {
			mode = 0600
		}
		if mode != 0 {
			for _, path := range []string{items[i].Path, fmt.Sprintf("data/.x6-firstboot-migrate-%s/old/%02d", e.Pin, i)} {
				if st, err := os.Lstat(e.path(path)); err == nil && st.Mode().Perm() == mode {
					items[i].Old.Mode = uint32(mode)
				}
			}
		}
	}

	return items, nil
}
func (e Engine) matches(path string, v Snapshot) bool {
	return e.hash(path, v.Bytes, v.SHA256, os.FileMode(v.Mode)) == nil
}
func (e Engine) checkCurrent(items []replacement, resume bool) error {
	for _, v := range items {
		if !e.matches(v.Path, v.Old) && !(resume && e.matches(v.Path, v.New)) {
			return fmt.Errorf("unknown migration input: %s", v.Path)
		}
	}
	return e.verifyPreferences()
}
func (e Engine) stageDir(rel string) error {
	parts := strings.Split(rel, "/")
	for i := 2; i <= len(parts); i++ {
		if err := e.makeDir(strings.Join(parts[:i], "/")); err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) saveBytes(dir, name string, b []byte, mode os.FileMode) error {
	return e.write(dir, name, mode, func(f *os.File) error { _, err := f.Write(b); return err })
}
func (e Engine) copyVerified(src, dir, name string, v Snapshot) error {
	if _, err := os.Lstat(e.path(dir + "/" + name)); err == nil {
		return e.hash(dir+"/"+name, v.Bytes, v.SHA256, os.FileMode(v.Mode))
	} else if !os.IsNotExist(err) {
		return err
	}
	return e.write(dir, name, os.FileMode(v.Mode), func(f *os.File) error {
		in, err := e.open(src, 0)
		if err != nil {
			return err
		}
		defer in.Close()
		if _, err = io.Copy(f, in); err != nil {
			return err
		}
		if err = e.copyLabel(src, f); err != nil {
			return err
		}
		return e.hash(dir+"/.writing", v.Bytes, v.SHA256, 0)
	})
}
func (e Engine) prepareMigration(dir string, items []replacement, m Manifest, blob []byte) error {
	var need uint64 = 32 << 20
	for _, v := range items {
		need += uint64(v.Old.Bytes + v.New.Bytes)
	}
	free := e.Free
	if free == nil {
		free = freeBytes
	}
	available, err := free(e.path("data"))
	if err != nil {
		return err
	}
	if available < need {
		return errors.New("insufficient migration space")
	}
	// Validate every official/new base before any old payload is replaced.
	for _, v := range m.Entries {
		if err = e.hash(v.Base, v.BaseBytes, v.BaseSHA256, 0); err != nil {
			return err
		}
	}
	if err = e.stageDir(dir); err != nil {
		return err
	}
	for i, v := range items {
		id := fmt.Sprintf("%02d", i)
		for _, kind := range []string{"old", "new"} {
			if err = e.makeDir(dir + "/" + kind); err != nil {
				return err
			}
		}
		if err = e.copyVerified(v.Path, dir+"/old", id, v.Old); err != nil {
			return err
		}
		if _, x := os.Lstat(e.path(dir + "/new/" + id)); x == nil {
			if err = e.hash(dir+"/new/"+id, v.New.Bytes, v.New.SHA256, os.FileMode(v.New.Mode)); err != nil {
				return err
			}
			continue
		} else if !os.IsNotExist(x) {
			return x
		}
		if v.Source != "" {
			err = e.copyVerified(v.Source, dir+"/new", id, v.New)
		} else {
			var entry Entry
			for _, x := range m.Entries {
				if x.Target == v.New.Target {
					entry = x
				}
			}
			err = e.write(dir+"/new", id, os.FileMode(v.New.Mode), func(out *os.File) error {
				in, x := e.open(entry.Base, 0)
				if x != nil {
					return x
				}
				_, x = io.Copy(out, in)
				in.Close()
				if x != nil {
					return x
				}
				if x = out.Truncate(entry.Bytes); x != nil {
					return x
				}
				for _, c := range entry.Chunks {
					if _, x = out.WriteAt(blob[c.BlobOffset:c.BlobOffset+c.Length], c.Offset); x != nil {
						return x
					}
				}
				return e.hash(dir+"/new/.writing", entry.Bytes, entry.SHA256, 0)
			})
		}
		if err != nil {
			return err
		}
		if err = e.hit("migration-staged:" + v.Path); err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) migrate(m Manifest, blob []byte) error {
	if e.Migration == nil {
		return errors.New("no predecessor policy")
	}
	dir := "data/.x6-firstboot-migrate-" + e.Pin
	var tx transaction
	raw, err := e.read(dir+"/transaction.json", 4096, 0600)
	resume := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if resume {
		if err = json.Unmarshal(raw, &tx); err != nil || tx.Version != 1 || tx.To != e.Pin {
			return errors.New("invalid migration journal")
		}
	}
	receipt, err := e.optional(featurePath + "/complete.json")
	if err != nil {
		return err
	}
	preserve, err := e.optional(featurePath + "/preserve-native-settings")
	if err != nil {
		return err
	}
	var chosen *Predecessor
	var items []replacement
	for _, p := range e.Migration.Previous {
		if resume && tx.From != p.Seed {
			continue
		}
		if !resume && receipt != p.Seed+"\n" && !(receipt == "" && p.Receiptless) {
			continue
		}
		if err = e.verifyOwners(p); err != nil {
			continue
		}
		legacy, x := e.legacyState(p)
		if x != nil {
			continue
		}
		if resume && tx.Legacy != legacy {
			continue
		}
		candidate, x := e.migrationItems(p, m, legacy)
		if x != nil {
			continue
		}
		if x = e.checkCurrent(candidate, resume); x != nil {
			continue
		}
		if chosen != nil {
			return errors.New("ambiguous predecessor")
		}
		copy := p
		chosen = &copy
		items = candidate
		if !resume {
			tx = transaction{1, p.Seed, e.Pin, receipt, preserve, legacy}
		}
	}
	if chosen == nil {
		return errors.New("unrecognized old seed/overlay; preserved without mutation")
	}
	if receipt != tx.Receipt && receipt != e.Pin+"\n" {
		return errors.New("migration receipt changed")
	}
	if preserve != "" && preserve != chosen.Seed+"\n" && preserve != e.Pin+"\n" {
		return errors.New("unknown preservation marker")
	}
	if err = e.overlayGuard(); err != nil {
		return err
	}
	if !resume {
		if err = e.prepareMigration(dir, items, m, blob); err != nil {
			return err
		}
		b, _ := json.Marshal(tx)
		if err = e.saveBytes(dir, "transaction.json", b, 0600); err != nil {
			return err
		}
	}
	// Refuse corrupted archives/staging before further writes, including on resume.
	for i, v := range items {
		for _, part := range []struct {
			kind string
			spec Snapshot
		}{{"old", v.Old}, {"new", v.New}} {
			if err = e.hash(fmt.Sprintf("%s/%s/%02d", dir, part.kind, i), part.spec.Bytes, part.spec.SHA256, os.FileMode(part.spec.Mode)); err != nil {
				return err
			}
		}
	}
	if err = e.hit("migration-ready"); err != nil {
		return err
	}
	// For a same-seed reinstall the receipt already names the target. Publish
	// preservation before the last rename can make verifyFinal succeed on reboot.
	if chosen.Modified && chosen.Seed == e.Pin && preserve == "" {
		if err = e.saveBytes(featurePath, "preserve-native-settings", []byte(e.Pin+"\n"), 0600); err != nil {
			return err
		}
	}
	for i, v := range items {
		if e.matches(v.Path, v.New) {
			continue
		}
		if err = e.hash(v.Path, v.Old.Bytes, v.Old.SHA256, os.FileMode(v.Old.Mode)); err != nil {
			return err
		}
		// Atomic replacement of one file. Dependent feature services remain ordered
		// after initialization; SSH identities, trust files and preferences are never targets.
		parent := filepath.Dir(v.Path)
		id := fmt.Sprintf("%s/new/%02d", dir, i)
		err = e.write(parent, filepath.Base(v.Path), os.FileMode(v.New.Mode), func(f *os.File) error {
			in, x := e.open(id, os.FileMode(v.New.Mode))
			if x != nil {
				return x
			}
			defer in.Close()
			_, x = io.Copy(f, in)
			if x != nil {
				return x
			}
			return e.copyLabel(v.Path, f)
		})
		if err != nil {
			return err
		}
		if err = e.hit("migration-replaced:" + v.Path); err != nil {
			return err
		}
	}
	if err = e.verifyContent(m); err != nil {
		return err
	}
	if err = e.saveBytes(featurePath, "preserve-native-settings", []byte(e.Pin+"\n"), 0600); err != nil {
		return err
	}
	if err = e.hit("migration-settings-preserved"); err != nil {
		return err
	}
	if err = e.saveBytes(featurePath, "complete.json", []byte(e.Pin+"\n"), 0600); err != nil {
		return err
	}
	if err = e.hit("migration-complete"); err != nil {
		return err
	}
	return e.finish(m)
}

// Only the exact legacy SSH unit and stock GUI copies/links are recognized.
// Unknown masks, attached units and drop-ins remain intact and refuse migration.
func (e Engine) overlayGuard() error {
	units := []string{"ins_gui.service", "x6-owner-maintenance.service", "x6-owner-late-start.service", "x6-owner-gui-auth.service", "x6-owner-gui-late-start.service", "x6-firstboot-seed.service", "x6-vr180-prepare.service", "x6-raw-nr-prepare.service"}
	dirs := []string{"service.d", "ins_.service.d", "x6-.service.d", "x6-owner-.service.d", "x6-owner-gui-.service.d", "x6-firstboot-.service.d", "x6-vr180-.service.d", "x6-raw-.service.d", "x6-raw-nr-.service.d"}
	for _, u := range units {
		dirs = append(dirs, u+".d")
	}
	bases := []string{"etc/systemd/system.control", "run/systemd/system.control", "run/systemd/transient", "run/systemd/generator.early", "etc/systemd/system", "etc/systemd/system.attached", "run/systemd/system", "run/systemd/system.attached", "run/systemd/generator", "usr/local/lib/systemd/system", "run/systemd/generator.late"}
	for _, base := range bases {
		for _, u := range units {
			rel := base + "/" + u
			st, err := os.Lstat(e.path(rel))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if err = e.parents(rel); err != nil {
				return err
			}
			if rel == legacyUnit {
				b, x := e.read(rel, 16384, 0644)
				if x == nil && digest(b) == legacyUnitSHA {
					continue
				}
			}
			if u == "ins_gui.service" {
				b, x := e.read("lib/systemd/system/ins_gui.service", 16384, 0644)
				if x != nil {
					return x
				}
				if st.Mode()&os.ModeSymlink != 0 {
					to, x := os.Readlink(e.path(rel))
					if x != nil {
						return x
					}
					if !filepath.IsAbs(to) {
						to = filepath.Clean(filepath.Join("/"+base, to))
					}
					if to == "/lib/systemd/system/ins_gui.service" || to == "/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service" {
						continue
					}
				} else {
					actual, x := e.read(rel, 16384, 0644)
					if x == nil && bytes.Equal(b, actual) {
						continue
					}
				}
			}
			return fmt.Errorf("unknown unit override: %s", rel)
		}
		for _, d := range dirs {
			matches, err := filepath.Glob(e.path(base + "/" + d + "/*.conf"))
			if err != nil {
				return err
			}
			if len(matches) > 0 {
				return errors.New("unknown overlay drop-in")
			}
		}
	}
	if _, err := os.Lstat(e.path("etc/ld.so.preload")); !os.IsNotExist(err) {
		return errors.New("global preload rejected")
	}
	return nil
}

func (e Engine) copyLabel(source string, target *os.File) error {
	old, err := e.open(source, 0)
	if err != nil {
		return err
	}
	defer old.Close()
	fn := e.ReplaceLabel
	if fn == nil {
		fn = preserveSELinux
	}
	return fn(old, target)
}
