package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func migrationFixture(t *testing.T) (testFixture, Predecessor) {
	t.Helper()
	f := fixture(t)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	p := Predecessor{Version: 1, Seed: f.e.Pin, Owners: map[string]string{}, Receiptless: true}
	for _, v := range f.m.Entries {
		p.Entries = append(p.Entries, Snapshot{v.Target, v.SHA256, v.Bytes, v.Mode})
	}
	for _, n := range []string{"adb_keys", "ins/etc/x6_owner_authorized_keys", "ins/etc/x6_owner_gui_ed25519.pub", "ins/etc/x6_owner_factory_ed25519.pub", "ins/etc/x6_owner_update_ed25519.pub"} {
		b := []byte("holder-key:" + n)
		put(t, f.e.path(n), b, 0644)
		p.Owners[n] = digest(b)
	}
	// Produce genuinely different new bytes, base hashes and manifest, not just a new receipt.
	for i := range f.m.Entries {
		v := &f.m.Entries[i]
		base := bytes.Repeat([]byte{99}, 8192)
		put(t, f.e.path(v.Base), base, 0644)
		target := append([]byte{}, base...)
		v.BaseSHA256 = digest(base)
		if !strings.Contains(v.Target, "/backup/") {
			target[4100] = byte(i + 30)
			off := v.Chunks[0].BlobOffset
			copy(f.blob[off:off+4096], target[4096:])
		}
		v.SHA256 = digest(target)
	}
	f.m.BlobSHA256 = digest(f.blob)
	f.save(t)
	f.e.Migration = &MigrationPolicy{Owners: p.Owners, Previous: []Predecessor{p}}
	put(t, f.e.path(featurePath+"/raw-nr-v1/level"), []byte{2}, 0600)
	put(t, f.e.path(featurePath+"/vr180-v1/sync"), []byte{1}, 0600)
	put(t, f.e.path(featurePath+"/old-feature-backup/keep"), []byte("retain old recovery"), 0600)
	put(t, f.e.path(journalDir+"/settings.json"), []byte("old complete settings journal"), 0600)
	return f, p
}
func checkMigration(t *testing.T, f testFixture) {
	t.Helper()
	if err := f.e.verifyFinal(f.m); err != nil {
		t.Fatal(err)
	}
	untouched(t, f.e)
	for n, w := range map[string][]byte{featurePath + "/raw-nr-v1/level": {2}, featurePath + "/vr180-v1/sync": {1}, featurePath + "/old-feature-backup/keep": []byte("retain old recovery"), journalDir + "/settings.json": []byte("old complete settings journal")} {
		b, err := os.ReadFile(f.e.path(n))
		if err != nil || !bytes.Equal(b, w) {
			t.Fatal("preservation failed", n)
		}
	}
	m := configFixture()
	m.failGet = true
	m.failSet = true
	m.failSave = true
	before, _ := json.Marshal(m.data)
	if err := f.e.Bootstrap(m); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(m.data)
	if !bytes.Equal(before, after) {
		t.Fatal("native settings modified")
	}
}
func TestMigrationUpgradeReinstallAndReceiptless(t *testing.T) {
	for _, kind := range []string{"old-receipt", "receiptless", "disabled-feature", "old-preserve-marker", "private-sync-library"} {
		t.Run(kind, func(t *testing.T) {
			f, p := migrationFixture(t)
			switch kind {
			case "receiptless":
				os.Remove(f.e.path(featurePath + "/complete.json"))
			case "disabled-feature":
				os.Remove(f.e.path(featurePath + "/vr180-v1/enabled"))
			case "private-sync-library":
				os.Chmod(f.e.path(featurePath+"/vr180-v1/sync.so"), 0600)
			case "old-preserve-marker":
				put(t, f.e.path(featurePath+"/preserve-native-settings"), []byte(p.Seed+"\n"), 0600)
			}
			if err := f.e.Run(); err != nil {
				t.Fatal(err)
			}
			checkMigration(t, f)
			if err := f.e.Run(); err != nil {
				t.Fatal("same-version reinstall", err)
			}
			checkMigration(t, f)
			if kind == "disabled-feature" {
				if _, err := os.Lstat(f.e.path(featurePath + "/vr180-v1/enabled")); !os.IsNotExist(err) {
					t.Fatal("disabled setting reset")
				}
			}
		})
	}
}
func TestMigrationFaultsResume(t *testing.T) {
	points := []string{}
	f, _ := migrationFixture(t)
	f.e.Fault = func(p string) error { points = append(points, p); return nil }
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			f, _ := migrationFixture(t)
			f.e.Fault = func(p string) error {
				if p == point {
					return errors.New("injected interruption")
				}
				return nil
			}
			err := f.e.Run()
			cleanup := strings.Contains(point, "cleanup")
			if !cleanup && err == nil {
				t.Fatal("fault absent")
			}
			if cleanup && (err != nil || f.e.verifyFinal(f.m) != nil) {
				t.Fatal("cleanup interrupted completed installation", err)
			}
			f.e.Fault = nil
			if err := f.e.Run(); err != nil {
				t.Fatal("resume", err)
			}
			checkMigration(t, f)
			for _, kind := range []string{"old", "new"} {
				if _, e := os.Lstat(f.e.path("data/.x6-firstboot-migrate-" + f.e.Pin + "/" + kind)); !os.IsNotExist(e) {
					t.Fatal("resume retained transaction copies", kind, e)
				}
			}
		})
	}
}
func TestMigrationRejectWithoutReplacingPayload(t *testing.T) {
	for _, kind := range []string{"wrong-owner-policy", "wrong-owner-file", "unknown-seed", "tampered-old", "symlink-old", "hardlink-old", "no-space", "unknown-overlay", "attached-mask", "bad-native-marker"} {
		t.Run(kind, func(t *testing.T) {
			f, p := migrationFixture(t)
			before, _ := os.ReadFile(f.e.path(featurePath + "/vr180-v1/gui"))
			switch kind {
			case "wrong-owner-policy":
				f.e.Migration.Owners = map[string]string{}
			case "wrong-owner-file":
				put(t, f.e.path("adb_keys"), []byte("other holder"), 0644)
			case "unknown-seed":
				put(t, f.e.path(featurePath+"/complete.json"), []byte(strings.Repeat("f", 64)+"\n"), 0600)
			case "tampered-old":
				put(t, f.e.path(featurePath+"/raw-nr-v1/low.so"), []byte("bad"), 0644)
			case "symlink-old":
				os.Remove(f.e.path(featurePath + "/raw-nr-v1/low.so"))
				os.Symlink(f.e.path("data/stock-calibration"), f.e.path(featurePath+"/raw-nr-v1/low.so"))
			case "hardlink-old":
				os.Link(f.e.path(featurePath+"/raw-nr-v1/low.so"), f.e.path("data/duplicate"))
			case "no-space":
				f.e.Free = func(string) (uint64, error) { return 1, nil }
			case "unknown-overlay":
				put(t, f.e.path("etc/systemd/system/x6-owner-gui-auth.service"), []byte("unknown"), 0644)
			case "attached-mask":
				os.MkdirAll(f.e.path("etc/systemd/system.attached"), 0700)
				os.Symlink("/dev/null", f.e.path("etc/systemd/system.attached/ins_gui.service"))
			case "bad-native-marker":
				put(t, f.e.path(featurePath+"/preserve-native-settings"), []byte("unknown"), 0600)
			}
			if f.e.Run() == nil {
				t.Fatal("accepted", kind, p.Seed)
			}
			after, _ := os.ReadFile(f.e.path(featurePath + "/vr180-v1/gui"))
			if !bytes.Equal(before, after) {
				t.Fatal("payload replaced on rejection")
			}
			untouched(t, f.e)
		})
	}
}
func TestMigrationArchiveTamperRefused(t *testing.T) {
	f, _ := migrationFixture(t)
	f.e.Fault = func(p string) error {
		if p == "migration-ready" {
			return errors.New("stop")
		}
		return nil
	}
	if f.e.Run() == nil {
		t.Fatal("fault")
	}
	f.e.Fault = nil
	dir := "data/.x6-firstboot-migrate-" + f.e.Pin
	put(t, f.e.path(dir+"/old/00"), []byte("corrupt archive"), 0600)
	if f.e.Run() == nil {
		t.Fatal("corrupt archive accepted")
	}
	untouched(t, f.e)
}
func TestMigrationKillWorker(t *testing.T) {
	if os.Getenv("X6_MIGRATION_WORKER") == "" {
		return
	}
	root := os.Getenv("X6_MIGRATION_ROOT")
	b, err := os.ReadFile(filepath.Join(root, "test-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy MigrationPolicy
	json.Unmarshal(b, &policy)
	e := Engine{Root: root, Pin: os.Getenv("X6_MIGRATION_PIN"), Migration: &policy}
	e.Fault = func(p string) error {
		if p == os.Getenv("X6_MIGRATION_POINT") {
			proc, _ := os.FindProcess(os.Getpid())
			proc.Kill()
			select {}
		}
		return nil
	}
	if err = e.Run(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("kill not reached")
}
func TestMigrationActualProcessKill(t *testing.T) {
	exe, _ := os.Executable()
	for _, point := range []string{"migration-ready", "migration-replaced:data/x6-owner-features/vr180-v1/gui", "migration-settings-preserved", "migration-complete"} {
		t.Run(point, func(t *testing.T) {
			f, _ := migrationFixture(t)
			b, _ := json.Marshal(f.e.Migration)
			put(t, f.e.path("test-policy.json"), b, 0600)
			c := exec.Command(exe, "-test.run=^TestMigrationKillWorker$")
			c.Env = append(os.Environ(), "X6_MIGRATION_WORKER=1", "X6_MIGRATION_ROOT="+f.e.Root, "X6_MIGRATION_PIN="+f.e.Pin, "X6_MIGRATION_POINT="+point)
			output, err := c.CombinedOutput()
			if err == nil || !strings.Contains(err.Error(), "signal: killed") {
				t.Fatalf("expected SIGKILL %v %s", err, output)
			}
			if err = f.e.Run(); err != nil {
				t.Fatal(err)
			}
			checkMigration(t, f)
		})
	}
}

func TestKnownLegacySSHOverlay(t *testing.T) {
	for _, kind := range []string{"normal", "after-SSH-replace", "wrong-unit", "wrong-key", "wrong-binary", "missing-review", "private-executable"} {
		t.Run(kind, func(t *testing.T) {
			f, p := migrationFixture(t)
			pub := []byte("ssh-ed25519 TEST-PUBLIC-ONLY\n")
			p.Owners["ins/etc/x6_owner_authorized_keys"] = digest(pub)
			put(t, f.e.path("ins/etc/x6_owner_authorized_keys"), pub, 0644)
			unit, err := os.ReadFile("testdata/legacy-ssh.service")
			if err != nil || digest(unit) != legacyUnitSHA {
				t.Fatal("unit fixture", err)
			}
			put(t, f.e.path(legacyUnit), unit, 0644)
			old := []byte("known old ssh executable")
			new := []byte("new source-built ssh executable")
			p.LegacySSH = &OldSSH{digest(old), int64(len(old))}
			f.e.Migration.Previous = []Predecessor{p}
			put(t, f.e.path(legacyExec), old, 0755)
			put(t, f.e.path("usr/local/sbin/x6-owner-maintenance"), new, 0755)
			put(t, f.e.path("data/x6-owner-maintenance/owner.pub"), []byte("ssh-ed25519 TEST-PUBLIC-ONLY retained-comment\n"), 0600)
			switch kind {
			case "private-executable":
				os.Chmod(f.e.path(legacyExec), 0700)
			case "wrong-unit":
				put(t, f.e.path(legacyUnit), []byte("unknown unit"), 0644)
			case "wrong-key":
				put(t, f.e.path("data/x6-owner-maintenance/owner.pub"), []byte("ssh-ed25519 OTHER\n"), 0600)
			case "wrong-binary":
				put(t, f.e.path(legacyExec), []byte("unreviewed"), 0755)
			case "missing-review":
				f.e.Migration.Previous[0].LegacySSH = nil
			case "after-SSH-replace":
				f.e.Fault = func(p string) error {
					if p == "migration-replaced:"+legacyExec {
						return errors.New("interrupted after legacy SSH")
					}
					return nil
				}
				if f.e.Run() == nil {
					t.Fatal("fault")
				}
				f.e.Fault = nil
			}
			err = f.e.Run()
			if kind == "normal" || kind == "after-SSH-replace" || kind == "private-executable" {
				if err != nil {
					t.Fatal(err)
				}
				checkMigration(t, f)
				b, _ := os.ReadFile(f.e.path(legacyExec))
				if !bytes.Equal(b, new) {
					t.Fatal("old executable retained")
				}
				b, _ = os.ReadFile(f.e.path(legacyUnit))
				if !bytes.Equal(b, unit) {
					t.Fatal("unit changed")
				}
				b, _ = os.ReadFile(f.e.path("data/x6-owner-maintenance/owner.pub"))
				if !bytes.Contains(b, []byte("retained-comment")) {
					t.Fatal("key/comment rewritten")
				}
			} else if err == nil {
				t.Fatal("unknown overlay accepted")
			}
			untouched(t, f.e)
		})
	}
}

func TestRealGeneratedMigration(t *testing.T) {
	root := os.Getenv("X6_REAL_MIGRATION_ROOT")
	if root == "" {
		t.Skip("explicit generated old/new fixture required")
	}
	e, err := targetEngine(root, productionSeed)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, n := range []string{"data/x6-owner-ssh/host_ed25519", "data/x6-owner-gui/server_ed25519", "data/x6-owner-gui/server_ed25519.pub", "etc/stock-settings", journalDir + "/settings.json", featurePath + "/vr180-v1/sync", featurePath + "/raw-nr-v1/level", "data/x6-owner-maintenance/owner.pub"} {
		b, x := os.ReadFile(e.path(n))
		if x != nil {
			t.Fatal(x)
		}
		before[n] = b
	}
	if err = e.Run(); err != nil {
		t.Fatal("actual generated old seed migration", err)
	}
	if err = e.Run(); err != nil {
		t.Fatal("actual same-version reinstall", err)
	}
	for n, b := range before {
		after, x := os.ReadFile(e.path(n))
		if x != nil || !bytes.Equal(b, after) {
			t.Fatal("real fixture identity/setting changed", n)
		}
	}
	put(t, e.path("migration-test-complete"), []byte("PASS\n"), 0600)
}

func TestMigrationLabelFailurePreservesActiveFiles(t *testing.T) {
	f, _ := migrationFixture(t)
	before, _ := os.ReadFile(f.e.path(featurePath + "/vr180-v1/gui"))
	f.e.ReplaceLabel = func(old, new *os.File) error { return errors.New("label copy denied") }
	if f.e.Run() == nil {
		t.Fatal("label failure ignored")
	}
	after, _ := os.ReadFile(f.e.path(featurePath + "/vr180-v1/gui"))
	if !bytes.Equal(before, after) {
		t.Fatal("active file changed")
	}
	untouched(t, f.e)
	f.e.ReplaceLabel = nil
	if err := f.e.Run(); err != nil {
		t.Fatal("retry", err)
	}
	checkMigration(t, f)
}
