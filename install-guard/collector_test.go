package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceCollector(t *testing.T) {
	for _, change := range []string{"valid-early", "duplicate-version", "symlink-updater", "unsafe-preserved-key"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			write := func(p string, b []byte, m os.FileMode) {
				p = filepath.Join(root, p)
				os.MkdirAll(filepath.Dir(p), 0700)
				if e := os.WriteFile(p, b, m); e != nil {
					t.Fatal(e)
				}
			}
			write("ins/etc/version.txt", []byte("Version:V0.8.825\nqcs8625_Version:V0.8.825\n"), 0644)
			write("usr/bin/ins_otad", []byte("inert fixture"), 0755)
			if change == "duplicate-version" {
				write("ins/etc/version.txt", []byte("qcs8625_Version:V0.8.825\nqcs8625_Version:V1.2.36\n"), 0644)
			}
			if change == "symlink-updater" {
				os.Remove(filepath.Join(root, "usr/bin/ins_otad"))
				os.Symlink(filepath.Join(root, "ins/etc/version.txt"), filepath.Join(root, "usr/bin/ins_otad"))
			}
			if change == "unsafe-preserved-key" {
				write("data/x6-owner-ssh/host_ed25519", []byte("not a real key"), 0600)
				os.Chmod(filepath.Join(root, "data/x6-owner-ssh/host_ed25519"), 0666)
			}
			s, e := collectSource(root)
			if (e == nil) != (change == "valid-early") {
				t.Fatal(s, e)
			}
			if e == nil && (s.Version != "0.8.825" || s.OTASHA256 != sumBytes([]byte("inert fixture")) || len(s.Preserved) != 0) {
				t.Fatal(s)
			}
		})
	}
}
