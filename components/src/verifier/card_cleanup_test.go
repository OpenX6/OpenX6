package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsumedInputRemovalPolicy(t *testing.T) {
	dig := strings.Repeat("a", 64)
	done := SDState{Version: 1, Phase: "completed", Boot: "b1", SourceSlot: "_a", Digest: dig, Release: strings.Repeat("c", 64)}
	cases := []struct {
		name, status, path string
		s                  SDState
		cleaned, pkg       string
		want               bool
	}{
		{"completed and cleaned", "left-card-suppressed", consumedCardPath, done, dig, dig, true},
		{"visible name is the owner's file", "left-card-suppressed", cardInputPath, done, dig, dig, false},
		{"stage not cleaned yet", "left-card-suppressed", consumedCardPath, done, "", dig, false},
		{"different package on the card", "left-card-suppressed", consumedCardPath, done, dig, strings.Repeat("b", 64), false},
		{"resident package", "resident-suppressed", consumedCardPath, SDState{Version: 1, Phase: "resident", Boot: "b1", SourceSlot: "_a", Digest: dig, Release: strings.Repeat("c", 64)}, dig, dig, false},
		{"still dispatched", "pending-outcome-no-retry", consumedCardPath, SDState{Version: 1, Phase: "dispatched", Boot: "b0", SourceSlot: "_a", Digest: dig, Release: strings.Repeat("c", 64)}, dig, dig, false},
		{"other status", "staged-install-locked", consumedCardPath, done, dig, dig, false},
	}
	for _, c := range cases {
		if got := shouldRemoveConsumedInput(c.status, c.path, c.s, c.cleaned, c.pkg); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestRemoveConsumedCardInput(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, filepath.Base(consumedCardPath))
	write := func(p, body string) {
		if e := os.WriteFile(p, []byte(body), 0644); e != nil {
			t.Fatal(e)
		}
	}
	open := func(p string) *os.File {
		f, e := os.Open(p)
		if e != nil {
			t.Fatal(e)
		}
		return f
	}
	t.Run("same file is removed", func(t *testing.T) {
		write(hidden, "pkg")
		f := open(hidden)
		defer f.Close()
		if e := removeConsumedCardInput(f, hidden); e != nil {
			t.Fatal(e)
		}
		if _, e := os.Lstat(hidden); !os.IsNotExist(e) {
			t.Fatal("consumed input still present")
		}
	})
	t.Run("replaced file is kept", func(t *testing.T) {
		write(hidden, "old")
		f := open(hidden)
		defer f.Close()
		other := filepath.Join(dir, "new")
		write(other, "new package")
		if e := os.Rename(other, hidden); e != nil {
			t.Fatal(e)
		}
		if e := removeConsumedCardInput(f, hidden); e == nil {
			t.Fatal("removed a file that replaced the verified one")
		}
		if b, e := os.ReadFile(hidden); e != nil || string(b) != "new package" {
			t.Fatal("replacement file was touched")
		}
		os.Remove(hidden)
	})
	t.Run("symlink is kept", func(t *testing.T) {
		target := filepath.Join(dir, "target")
		write(target, "t")
		if e := os.Symlink(target, hidden); e != nil {
			t.Fatal(e)
		}
		f := open(target)
		defer f.Close()
		if e := removeConsumedCardInput(f, hidden); e == nil {
			t.Fatal("followed a symlink")
		}
		if _, e := os.Lstat(hidden); e != nil {
			t.Fatal("symlink removed")
		}
		os.Remove(hidden)
		os.Remove(target)
	})
	t.Run("visible owner file is never removed", func(t *testing.T) {
		visible := filepath.Join(dir, filepath.Base(cardInputPath))
		write(visible, "owner package")
		f := open(visible)
		defer f.Close()
		if e := removeConsumedCardInput(f, visible); e == nil {
			t.Fatal("removed the owner's visible package")
		}
		if _, e := os.Lstat(visible); e != nil {
			t.Fatal("visible package missing")
		}
	})
}
