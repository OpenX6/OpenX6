package main

import (
	"os"
	"path/filepath"
	"testing"
)

func ready() InstallReadiness {
	return InstallReadiness{CurrentVersion: "1.2.36", RootUID: 0, GUIUID: 0, SSHIdentityPinned: true, GUIIdentityPinned: true, SSHWrongKeyRejected: true, GUIWrongKeyRejected: true, Idle: true, PowerStable: true, SlotsVerified: true, HoldPresent: true}
}
func TestSingleFileToSameVersionHandoff(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	f := inputFile(t, s, embedTest(t, s))
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	if e := StageSingle(f, p, nil); e != nil {
		t.Fatal(e)
	}
	r := ready()
	plan, e := BuildHandoff(p, r)
	if e != nil || len(plan) != 7 || plan[6][len(plan[6])-1] != p.Package {
		t.Fatal(plan, e)
	}
	for name, change := range map[string]func(*InstallReadiness){"SSH absent": func(r *InstallReadiness) { r.RootUID = 1000 }, "GUI absent": func(r *InstallReadiness) { r.GUIUID = 1000 }, "identity unchecked": func(r *InstallReadiness) { r.GUIIdentityPinned = false }, "wrong key not rejected": func(r *InstallReadiness) { r.SSHWrongKeyRejected = false }, "busy": func(r *InstallReadiness) { r.Idle = false }, "battery": func(r *InstallReadiness) { r.PowerStable = false }, "slot mismatch": func(r *InstallReadiness) { r.SlotsVerified = false }, "hold lost": func(r *InstallReadiness) { r.HoldPresent = false }, "incomplete prior attempt": func(r *InstallReadiness) { r.PendingAttempt = true }, "unsigned downgrade": func(r *InstallReadiness) { r.CurrentVersion = "1.2.37" }} {
		t.Run(name, func(t *testing.T) {
			r := ready()
			change(&r)
			if p, e := BuildHandoff(p, r); e == nil || p != nil {
				t.Fatal("unsafe handoff")
			}
		})
	}
	os.Remove(p.Package + ".owner-auth")
	if _, e := BuildHandoff(p, ready()); e == nil {
		t.Fatal("missing signature")
	}
}
func TestAuthorizedDowngradeHandoff(t *testing.T) {
	s := makeSetup(t, "1.2.36", true)
	r := ready()
	r.CurrentVersion = "1.2.37"
	if _, e := BuildHandoff(s.p, r); e != nil {
		t.Fatal(e)
	}
}
