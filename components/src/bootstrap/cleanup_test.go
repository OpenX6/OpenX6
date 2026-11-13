package main

import (
	"errors"
	"os"
	"testing"
)

func TestCompletedCleanupAndResume(t *testing.T) {
	for _, point := range []string{"", "cleanup-ready", "cleanup-removed:old0", "cleanup-removed:new0"} {
		t.Run(point, func(t *testing.T) {
			f, _ := migrationFixture(t)
			f.e.Fault = func(p string) error {
				if point != "" && p == point {
					return errors.New("interrupted")
				}
				return nil
			}
			if e := f.e.Run(); e != nil {
				t.Fatal(e)
			}
			checkMigration(t, f)
			f.e.Fault = nil
			if e := f.e.Run(); e != nil {
				t.Fatal(e)
			}
			checkMigration(t, f)
			for _, kind := range []string{"old", "new"} {
				if _, e := os.Lstat(f.e.path("data/.x6-firstboot-migrate-" + f.e.Pin + "/" + kind)); !os.IsNotExist(e) {
					t.Fatal("copies retained", kind, e)
				}
			}
		})
	}
}
func TestCleanupRefusesUnknownAndCorruption(t *testing.T) {
	for _, kind := range []string{"unknown", "symlink", "hardlink", "corrupt", "wrong-marker", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := migrationFixture(t)
			f.e.Fault = func(p string) error {
				if p == "migration-complete" {
					return errors.New("cut")
				}
				return nil
			}
			if f.e.Run() == nil {
				t.Fatal("fault not reached")
			}
			f.e.Fault = nil
			d := "data/.x6-firstboot-migrate-" + f.e.Pin
			switch kind {
			case "unknown":
				put(t, f.e.path(d+"/keep"), []byte("unknown"), 0600)
			case "symlink":
				os.Remove(f.e.path(d + "/old/00"))
				os.Symlink(f.e.path(featurePath+"/vr180-v1/gui"), f.e.path(d+"/old/00"))
			case "hardlink":
				os.Link(f.e.path(d+"/old/00"), f.e.path(d+"/link"))
			case "corrupt":
				put(t, f.e.path(d+"/old/00"), []byte("bad"), 0600)
			case "wrong-marker":
				put(t, f.e.path(d+"/cleanup.ready"), []byte("bad"), 0600)
			case "incomplete":
				os.Remove(f.e.path(featurePath + "/complete.json"))
			}
			if f.e.cleanupCompleted(f.m) == nil {
				t.Fatal("unsafe cleanup allowed")
			}
			if _, e := os.Lstat(f.e.path(d + "/new/00")); e != nil {
				t.Fatal("removed before validation", e)
			}
		})
	}
}
