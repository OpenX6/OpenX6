package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"openx6.local/components/holder"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) view {
	t.Helper()
	v := view{root: t.TempDir(), uid: uint32(os.Getuid()), gid: uint32(os.Getgid())}
	// Real filesystem operations; only owner IDs and packaged fixture content
	// differ from the target. This does not simulate PID1 or SELinux enforcement.
	for _, p := range []string{"/data", "/run/x6-owner-gui", "/run/x6-owner-gui-bridge"} {
		if e := os.MkdirAll(v.path(p), 0700); e != nil {
			t.Fatal(e)
		}
	}
	return v
}
func write(t *testing.T, v view, p string, b []byte, mode os.FileMode) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(v.path(p)), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(v.path(p), b, mode); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(v.path(p), mode); e != nil {
		t.Fatal(e)
	}
}
func TestDeviceIdentity(t *testing.T) {
	t.Run("fresh_then_same_identity", func(t *testing.T) {
		v := fixture(t)
		if e := v.identity(); e != nil {
			t.Fatal(e)
		}
		a, _ := v.regular("/data/x6-owner-gui/server_ed25519", 0600, 32)
		pub, _ := v.regular("/data/x6-owner-gui/server_ed25519.pub", 0600, 32)
		if string(ed25519.NewKeyFromSeed(a).Public().(ed25519.PublicKey)) != string(pub) {
			t.Fatal("pub mismatch")
		}
		if e := v.identity(); e != nil {
			t.Fatal(e)
		}
		b, _ := v.regular("/data/x6-owner-gui/server_ed25519", 0600, 32)
		if string(a) != string(b) {
			t.Fatal("identity rotated")
		}
	})
	t.Run("private_only_recovers_matching_public", func(t *testing.T) {
		v := fixture(t)
		os.Mkdir(v.path("/data/x6-owner-gui"), 0700)
		seed := make([]byte, 32)
		rand.Read(seed)
		write(t, v, "/data/x6-owner-gui/server_ed25519", seed, 0600)
		if e := v.identity(); e != nil {
			t.Fatal(e)
		}
	})
	for _, name := range []string{"orphan_public", "private_wrong_size", "private_world_readable", "private_symlink", "directory_symlink", "directory_mode", "public_mismatch", "public_symlink"} {
		t.Run(name, func(t *testing.T) {
			v := fixture(t)
			seed := make([]byte, 32)
			rand.Read(seed)
			os.Mkdir(v.path("/data/x6-owner-gui"), 0700)
			switch name {
			case "orphan_public":
				write(t, v, "/data/x6-owner-gui/server_ed25519.pub", seed, 0600)
			case "private_wrong_size":
				write(t, v, "/data/x6-owner-gui/server_ed25519", []byte("partial"), 0600)
			case "private_world_readable":
				write(t, v, "/data/x6-owner-gui/server_ed25519", seed, 0644)
			case "private_symlink":
				write(t, v, "/data/unrelated", seed, 0600)
				os.Symlink(v.path("/data/unrelated"), v.path("/data/x6-owner-gui/server_ed25519"))
			case "directory_symlink":
				os.Remove(v.path("/data/x6-owner-gui"))
				os.Symlink(v.path("/run"), v.path("/data/x6-owner-gui"))
			case "directory_mode":
				os.Chmod(v.path("/data/x6-owner-gui"), 0755)
			case "public_mismatch":
				write(t, v, "/data/x6-owner-gui/server_ed25519", seed, 0600)
				write(t, v, "/data/x6-owner-gui/server_ed25519.pub", seed, 0600)
			case "public_symlink":
				write(t, v, "/data/x6-owner-gui/server_ed25519", seed, 0600)
				write(t, v, "/data/unrelated", seed, 0600)
				os.Symlink(v.path("/data/unrelated"), v.path("/data/x6-owner-gui/server_ed25519.pub"))
			}
			if e := v.identity(); e == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	t.Run("no_overwrite_existing", func(t *testing.T) {
		v := fixture(t)
		p := "/data/preserved"
		write(t, v, p, []byte("saved"), 0600)
		if e := v.create(p, []byte("replace"), 0600); e == nil {
			t.Fatal("overwrite")
		}
		b, _ := os.ReadFile(v.path(p))
		if string(b) != "saved" {
			t.Fatal("changed")
		}
	})
}

func base(t *testing.T) view {
	t.Helper()
	root := os.Getenv("X6_BUILD_TEST_ROOT")
	if root == "" {
		t.Fatal("X6_BUILD_TEST_ROOT required for generated-image tests")
	}
	overlay := filepath.Join(root, "overlay")
	v := fixture(t)
	for p, s := range expectedFiles {
		src := filepath.Join(overlay, p)
		if p == "/lib/systemd/system/ins_gui.service" || p == "/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service" {
			src = filepath.Join(root, "startup-stock/ins_gui.service")
		}
		if p == "/lib/ld-linux-aarch64.so.1" {
			src = filepath.Join(root, "startup-stock/loader")
		}
		if p == "/usr/bin/ins_gui" && os.Getenv("X6_GUI_FEATURE_TEST") == "1" {
			src = filepath.Join(root, "seed-targets/vr180-v1/gui")
		}
		b, e := os.ReadFile(src)
		if e != nil {
			t.Fatal(e)
		}
		write(t, v, p, b, s.Mode)
	}
	key, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := v.path(holder.Dir)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	pins := map[string]string{}
	for _, name := range []string{holder.SSH, holder.GUI, holder.ADB, holder.Factory, holder.Update} {
		h := sha256.Sum256(key)
		pins[name] = hex.EncodeToString(h[:])
		write(t, v, holder.Dir+"/"+name, key, 0600)
	}
	manifest, _ := json.Marshal(pins)
	write(t, v, holder.Dir+"/pins.json", manifest, 0600)
	return v
}
func TestGuardRealFilesystemBranches(t *testing.T) {
	stock, e := os.ReadFile(filepath.Join(os.Getenv("X6_BUILD_TEST_ROOT"), "startup-stock/ins_gui.service"))
	if e != nil {
		t.Fatal(e)
	}
	t.Run("packaged_files", func(t *testing.T) {
		if e := base(t).check(); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("known_stock_regular_override", func(t *testing.T) {
		v := base(t)
		write(t, v, "/etc/systemd/system/ins_gui.service", stock, 0644)
		if e := v.check(); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("known_stock_link", func(t *testing.T) {
		v := base(t)
		os.MkdirAll(v.path("/etc/systemd/system"), 0755)
		os.Symlink("/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service", v.path("/etc/systemd/system/ins_gui.service"))
		if e := v.check(); e != nil {
			t.Fatal(e)
		}
	})
	cases := []struct{ name, path, content string }{
		{"GUI_replaced", "/usr/bin/ins_gui", "bad"}, {"wrong_owner_public", holder.Dir + "/" + holder.GUI, "bad"},
		{"unknown_GUI_unit", "/etc/systemd/system/ins_gui.service", "bad"}, {"unknown_broker_unit", "/run/systemd/system/x6-owner-gui-auth.service", "bad"},
		{"generic_dropin", "/etc/systemd/system/service.d/unknown.conf", "bad"}, {"prefix_dropin", "/etc/systemd/system/x6-owner-.service.d/unknown.conf", "bad"},
		{"specific_dropin", "/etc/systemd/system/ins_gui.service.d/unknown.conf", "bad"}, {"lower_specific_extra", "/lib/systemd/system/ins_gui.service.d/other.conf", "bad"},
		{"generator_override", "/run/systemd/generator.early/ins_gui.service", "bad"}, {"global_preload", "/etc/ld.so.preload", "/bad.so"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := base(t)
			write(t, v, c.path, []byte(c.content), 0644)
			if e := v.check(); e == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	t.Run("mask", func(t *testing.T) {
		v := base(t)
		os.MkdirAll(v.path("/etc/systemd/system"), 0755)
		os.Symlink("/dev/null", v.path("/etc/systemd/system/ins_gui.service"))
		if e := v.check(); e == nil {
			t.Fatal("mask accepted")
		}
	})
	t.Run("missing_bridge", func(t *testing.T) {
		v := base(t)
		os.Remove(v.path("/usr/lib/libx6-owner-gui-bridge.so"))
		if e := v.check(); e == nil {
			t.Fatal("missing accepted")
		}
	})
	t.Run("writable_unit", func(t *testing.T) {
		v := base(t)
		os.Chmod(v.path("/lib/systemd/system/x6-owner-gui-auth.service"), 0666)
		if e := v.check(); e == nil {
			t.Fatal("writable accepted")
		}
	})
	t.Run("wrong_owner", func(t *testing.T) {
		v := base(t)
		v.uid++
		if e := v.check(); e == nil {
			t.Fatal("owner accepted")
		}
	})
	t.Run("GUI_guard_no_data_access", func(t *testing.T) {
		v := base(t)
		os.RemoveAll(v.path("/data"))
		if v.check() == nil {
			t.Fatal("broker accepted missing credentials")
		}
		if e := v.checkFiles(); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("directory_wrong_mode", func(t *testing.T) {
		v := fixture(t)
		os.Chmod(v.path("/run/x6-owner-gui-bridge"), 0755)
		if e := v.dir("/run/x6-owner-gui-bridge", false); e == nil {
			t.Fatal("dir accepted")
		}
	})
}
func TestScopedLoaderEnvironment(t *testing.T) {
	t.Setenv("LD_PRELOAD", "bad")
	t.Setenv("LD_LIBRARY_PATH", "bad")
	t.Setenv("LD_AUDIT", "bad")
	t.Setenv("GLIBC_TUNABLES", "bad")
	t.Setenv("WAYLAND_DISPLAY", "wayland-1")
	seen := false
	for _, v := range cleanEnv() {
		if strings.HasPrefix(v, "LD_") || strings.HasPrefix(v, "GLIBC_TUNABLES=") {
			t.Fatal("injection retained")
		}
		if v == "WAYLAND_DISPLAY=wayland-1" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("vendor env lost")
	}
}
func TestLocalGUITrust(t *testing.T) {
	v := base(t)
	if _, ok := expectedFiles["/ins/etc/x6_owner_gui_ed25519.pub"]; ok {
		t.Fatal("image pins builder key")
	}
	if err := os.Remove(v.path(holder.Dir + "/pins.json")); err != nil {
		t.Fatal(err)
	}
	if v.check() == nil {
		t.Fatal("missing local identity accepted")
	}
}

func TestGUIFallback(t *testing.T) {
	t.Run("healthy_bridge", func(t *testing.T) {
		p, e := base(t).guiPlan()
		if e != nil || !p.Bridge || !strings.Contains(strings.Join(p.Args, " "), "--preload") {
			t.Fatalf("%+v %v", p, e)
		}
	})
	for _, name := range []string{"missing_bridge", "bad_broker", "bad_broker_unit", "missing_bridge_runtime", "wrong_bridge_mode", "stale_socket"} {
		t.Run(name, func(t *testing.T) {
			v := base(t)
			switch name {
			case "missing_bridge":
				os.Remove(v.path("/usr/lib/libx6-owner-gui-bridge.so"))
			case "bad_broker":
				write(t, v, "/usr/local/sbin/x6-owner-gui-auth", []byte("wrong"), 0755)
			case "bad_broker_unit":
				write(t, v, "/lib/systemd/system/x6-owner-gui-auth.service", []byte("wrong"), 0644)
			case "missing_bridge_runtime":
				os.Remove(v.path("/run/x6-owner-gui-bridge"))
			case "wrong_bridge_mode":
				os.Chmod(v.path("/run/x6-owner-gui-bridge"), 0755)
			case "stale_socket":
				write(t, v, "/run/x6-owner-gui-bridge/dispatch.sock", []byte("unrelated"), 0600)
			}
			p, e := v.guiPlan()
			if e != nil || p.Bridge || p.Disabled == "" || len(p.Args) != 2 || p.Args[1] != "/usr/bin/ins_gui" {
				t.Fatalf("unsafe fallback: %+v %v", p, e)
			}
			if name == "stale_socket" {
				b, _ := os.ReadFile(v.path("/run/x6-owner-gui-bridge/dispatch.sock"))
				if string(b) != "unrelated" {
					t.Fatal("stale socket changed")
				}
			}
		})
	}
	for _, name := range []string{"GUI", "wrapper", "loader", "loader_symlink", "loader_parent_symlink", "global_preload", "empty_global_preload", "bad_gui_unit", "unknown_dropin", "bad_gui_dropin"} {
		t.Run("reject_"+name, func(t *testing.T) {
			v := base(t)
			paths := map[string]string{"GUI": "/usr/bin/ins_gui", "wrapper": "/usr/lib/libins_gui_wrap.so", "loader": "/lib/ld-linux-aarch64.so.1"}
			if p, ok := paths[name]; ok {
				write(t, v, p, []byte("bad"), 0755)
			} else {
				switch name {
				case "loader_symlink":
					os.Remove(v.path("/lib/ld-linux-aarch64.so.1"))
					os.Symlink("/somewhere", v.path("/lib/ld-linux-aarch64.so.1"))
				case "loader_parent_symlink":
					os.Rename(v.path("/lib"), v.path("/lib-real"))
					os.Symlink(v.path("/lib-real"), v.path("/lib"))
				case "global_preload":
					write(t, v, "/etc/ld.so.preload", []byte("/bad.so"), 0644)
				case "bad_gui_unit":
					write(t, v, "/etc/systemd/system/ins_gui.service", []byte("wrong"), 0644)
				case "unknown_dropin":
					write(t, v, "/etc/systemd/system/service.d/untrusted.conf", []byte("wrong"), 0644)
				case "bad_gui_dropin":
					write(t, v, "/lib/systemd/system/ins_gui.service.d/50-owner-gui.conf", []byte("wrong"), 0644)
				case "empty_global_preload":
					write(t, v, "/etc/ld.so.preload", nil, 0644)
				}
			}
			if p, e := v.guiPlan(); e == nil {
				t.Fatalf("untrusted core accepted: %+v", p)
			}
		})
	}
}

func TestMigratedGeneratedGUIStartup(t *testing.T) {
	root := os.Getenv("X6_REAL_MIGRATION_ROOT")
	if root == "" {
		t.Skip("explicit migrated fixture required")
	}
	if b, e := os.ReadFile(filepath.Join(root, "migration-test-complete")); e != nil || string(b) != "PASS\n" {
		t.Fatal("migration did not finish")
	}
	v := view{root: root, uid: uint32(os.Getuid()), gid: uint32(os.Getgid())}
	if e := v.check(); e != nil {
		t.Fatal("new base GUI startup", e)
	}
	before, e := os.ReadFile(v.path("/data/x6-owner-gui/server_ed25519"))
	if e != nil {
		t.Fatal(e)
	}
	if e = v.identity(); e != nil {
		t.Fatal("GUI identity reload", e)
	}
	after, _ := os.ReadFile(v.path("/data/x6-owner-gui/server_ed25519"))
	if string(before) != string(after) {
		t.Fatal("GUI identity rotated")
	}
}
