package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRepairFailureWindow(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"failed", "failed", true}, {"inactive", "active", true}, {"active", "active", false}, {"inactive", "activating", false}, {"failed", "unknown", false}, {"failed", "error", false}, {"missing", "failed", false}} {
		load := func(unit string) ([]byte, error) {
			state := tc.a
			if unit == "x6-owner-gui-auth.service" {
				state = tc.b
			}
			if state == "error" {
				return nil, errors.New("unavailable")
			}
			if state == "missing" {
				return []byte("LoadState=not-found\nActiveState=inactive"), nil
			}
			return []byte("ActiveState=" + state + "\nLoadState=loaded\n"), nil
		}
		if accessStopped(load) != tc.want {
			t.Fatal(tc)
		}
	}
	var w failureWindow
	now := time.Now()
	for _, v := range []struct {
		sec        int
		down, want bool
	}{{0, true, false}, {20, true, false}, {29, false, false}, {30, true, false}, {59, true, false}, {60, true, true}, {61, false, false}} {
		if w.observed(now.Add(time.Duration(v.sec)*time.Second), v.down) != v.want {
			t.Fatal(v)
		}
	}
}

func TestSameCardRepair(t *testing.T) {
	for _, phase := range []string{"idle", "resident", "completed", "intent", "dispatched"} {
		t.Run(phase, func(t *testing.T) {
			c, o, n := sdFixture()
			o.CurrentRelease = o.PackageRelease
			o.Card = strings.Repeat("d", 64)
			o.Identity = strings.Repeat("e", 64)
			o.Repair = true
			o.NewPresentation = true
			o.InstallAllowed = false
			if phase != "idle" {
				c.State = SDState{Version: 1, Phase: phase, Boot: "old", SourceSlot: "_b", Digest: o.PackageDigest, Release: o.CurrentRelease, Identity: o.Identity, Card: o.Card}
			}
			var journal SDState
			c.Save = func(s SDState) error { journal = s; return nil }
			step := func(want string) {
				t.Helper()
				s, e := c.Step(o)
				if e != nil || s != want {
					t.Fatalf("%s: got %s %v", want, s, e)
				}
			}
			o.NewPresentation = false
			step("left-card-suppressed")
			o.NewPresentation = true
			step("repair-armed")
			o.NewPresentation = false
			step("staged-install-locked")
			if *n != 0 {
				t.Fatal("crossed installation lock")
			}
			// Downstream transition tests use a fake in-memory executor, never the production policy.
			o.InstallAllowed = true
			step("dispatched")
			if *n != 1 {
				t.Fatal(*n)
			}
			c = SDCoordinator{State: journal, Save: c.Save, Dispatch: c.Dispatch}
			o.Boot = "new"
			o.Slot = "_b"
			step("left-card-suppressed")
			o.Repair = false
			o.Ready = false
			step("pending-outcome-no-retry")
			o.Ready = true
			o.CompletionVerified = true
			step("completed")
			for _, boot := range []string{"new", "boot3", "boot4"} {
				o.Boot = boot
				step("left-card-suppressed")
			}
			if *n != 1 {
				t.Fatal("left card reinstalled")
			}
		})
	}
}
func TestRepairQuarantine(t *testing.T) {
	for _, bad := range []string{"same-boot", "same-slot", "wrong-id", "wrong-digest", "wrong-release", "identity-missing", "busy", "exported", "uncertain"} {
		c, o, n := sdFixture()
		o.CurrentRelease = o.PackageRelease
		o.Card = strings.Repeat("d", 64)
		o.Identity = strings.Repeat("e", 64)
		o.Repair = true
		o.NewPresentation = true
		c.State = SDState{Version: 1, Phase: "intent", SourceSlot: "_b", Boot: "old", Digest: o.PackageDigest, Release: o.CurrentRelease, Identity: o.Identity}
		switch bad {
		case "same-boot":
			o.Boot = "old"
		case "same-slot":
			o.Slot = "_b"
		case "wrong-id":
			o.Identity = strings.Repeat("f", 64)
		case "wrong-digest":
			o.PackageDigest = strings.Repeat("f", 64)
		case "wrong-release":
			o.PackageRelease = strings.Repeat("f", 64)
		case "identity-missing":
			o.Identity = ""
		case "busy":
			o.Ready = false
		case "exported":
			o.Presence = "unknown"
		case "uncertain":
			c.State.Phase = "uncertain"
		}
		before := c.State
		if _, err := c.Step(o); err != nil || c.State != before || *n != 0 {
			t.Fatal(bad, err, c.State)
		}
	}
}
