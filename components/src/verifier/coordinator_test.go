package main

import (
	"errors"
	"strings"
	"testing"
)

func sdFixture() (SDCoordinator, SDObservation, *int) {
	calls := new(int)
	o := SDObservation{Boot: "boot1", Slot: "_a", CurrentRelease: strings.Repeat("a", 64), PackageRelease: strings.Repeat("b", 64), PackageDigest: strings.Repeat("c", 64), Card: "card1", Presence: "present", Ready: true, InstallAllowed: true}
	c := SDCoordinator{Save: func(SDState) error { return nil }, Stage: func() error { return nil }, Dispatch: func() error { *calls++; return nil }}
	return c, o, calls
}
func TestSDHappyAndLeftCard(t *testing.T) {
	c, o, n := sdFixture()
	if s, e := c.Step(o); e != nil || s != "dispatched" || *n != 1 {
		t.Fatal(s, e, *n)
	}
	for i := 0; i < 5; i++ {
		c.Step(o)
	}
	if *n != 1 {
		t.Fatal("repeated")
	}
	o.Boot = "boot2"
	o.Slot = "_b"
	o.CurrentRelease = o.PackageRelease
	o.CompletionVerified = true
	if s, e := c.Step(o); e != nil || s != "completed" {
		t.Fatal(s, e)
	}
	for i := 0; i < 5; i++ {
		c.Step(o)
	}
	if *n != 1 {
		t.Fatal("left-card reinstall")
	}
	o.Presence = "unknown"
	c.Step(o)
	o.Presence = "present"
	c.Step(o)
	if *n != 1 {
		t.Fatal("UDisk rearmed")
	}
	o.Presence = "absent"
	c.Step(o)
	o.Presence = "present"
	if s, e := c.Step(o); e != nil || s != "dispatched" || *n != 2 {
		t.Fatal(s, e, *n)
	}
}
func TestSDFreshBootResident(t *testing.T) {
	c, o, n := sdFixture()
	o.CurrentRelease = o.PackageRelease
	for i := 0; i < 5; i++ {
		c.Step(o)
	}
	if *n != 0 || c.State.Phase != "resident" {
		t.Fatal(c.State)
	}
	c.SessionBoot = ""
	c.SeenPresent = false
	o.Presence = "absent"
	c.Step(o)
	o.Presence = "present"
	c.Step(o)
	if *n != 0 {
		t.Fatal("startup absence rearmed")
	}
	o.Presence = "absent"
	c.Step(o)
	o.Presence = "present"
	c.Step(o)
	if *n != 1 {
		t.Fatal("explicit reinsertion rejected")
	}
}
func TestSDCrashBoundaries(t *testing.T) {
	for _, point := range []string{"staged-before-save", "staged", "intent", "dispatched-before-save", "dispatched"} {
		t.Run(point, func(t *testing.T) {
			c, o, n := sdFixture()
			var disk SDState
			c.Save = func(s SDState) error { disk = s; return nil }
			c.Fault = func(s string) error {
				if s == point {
					return errors.New("crash")
				}
				return nil
			}
			if _, e := c.Step(o); e == nil {
				t.Fatal("missing fault")
			}
			c.State = disk
			c.Fault = nil
			c.SessionBoot = ""
			c.Step(o)
			if *n != 1 && point != "intent" {
				t.Fatal(*n)
			}
			if point == "intent" && *n != 0 {
				t.Fatal("unknown intent retried")
			}
			o.Boot = "boot2"
			o.Slot = "_b"
			o.CurrentRelease = o.PackageRelease
			o.CompletionVerified = true
			c.Step(o)
			if c.State.Phase != "completed" {
				t.Fatal(c.State)
			}
		})
	}
}
func TestSDRejectsAndQuarantine(t *testing.T) {
	for _, name := range []string{"not-ready", "locked", "stage-error", "save-error", "dispatch-error", "rollback", "same-slot", "no-root", "bad-journal"} {
		t.Run(name, func(t *testing.T) {
			c, o, n := sdFixture()
			switch name {
			case "not-ready":
				o.Ready = false
			case "locked":
				o.InstallAllowed = false
			case "stage-error":
				c.Stage = func() error { return errors.New("bad auth") }
			case "save-error":
				c.Save = func(SDState) error { return errors.New("disk full") }
			case "dispatch-error":
				c.Dispatch = func() error { return errors.New("unknown") }
			case "bad-journal":
				c.State = SDState{Version: 99}
			default:
			}
			c.Step(o)
			if name == "rollback" || name == "same-slot" || name == "no-root" {
				o.Boot = "boot2"
				o.CompletionVerified = true
				if name != "same-slot" {
					o.Slot = "_b"
				}
				if name != "rollback" {
					o.CurrentRelease = o.PackageRelease
				}
				if name == "no-root" {
					o.Ready = false
				}
				for i := 0; i < 4; i++ {
					c.Step(o)
				}
				if *n != 1 || c.State.Phase == "completed" {
					t.Fatal(c.State, *n)
				}
			} else if *n != 0 {
				t.Fatal("unexpected dispatch", *n)
			}
		})
	}
}
