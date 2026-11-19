package main

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"time"
)

// A failed command, an unknown unit, or an activating service is not evidence
// that a known access service stopped. Healthy services use the normal path.
func accessStopped(show func(string) ([]byte, error)) bool {
	stopped := false
	for _, unit := range []string{"x6-owner-maintenance.service", "x6-owner-gui-auth.service"} {
		b, err := show(unit)
		if err != nil {
			return false
		}
		loaded, state := false, ""
		restarting := false
		for _, line := range strings.Split(string(bytes.TrimSpace(b)), "\n") {
			switch {
			case line == "SubState=auto-restart":
				restarting = true
			case line == "LoadState=loaded":
				loaded = true
			case strings.HasPrefix(line, "ActiveState="):
				state = strings.TrimPrefix(line, "ActiveState=")
			}
		}
		if !loaded {
			return false
		}
		switch state {
		case "inactive", "failed":
			stopped = true
		case "activating":
			if !restarting {
				return false
			}
			stopped = true
		case "active":
		default:
			return false
		}
	}
	return stopped
}
func stoppedOnCamera() bool {
	return accessStopped(func(unit string) ([]byte, error) {
		return localCommand(4, "/bin/systemctl", "show", unit, "-p", "LoadState", "-p", "ActiveState", "-p", "SubState")
	})
}

type failureWindow struct{ since time.Time }

func (w *failureWindow) observed(now time.Time, stopped bool) bool {
	if !stopped {
		w.since = time.Time{}
		return false
	}
	if w.since.IsZero() {
		w.since = now
	}
	return now.Sub(w.since) >= 30*time.Second
}

// Before retrying a previous handoff, require independent evidence that its
// target actually booted. Never reinterpret an ambiguous same-slot outcome.
func landed(s SDState, o SDObservation) bool {
	return s.Boot != o.Boot && o.Slot != s.SourceSlot && o.CurrentRelease == s.Release &&
		validID(s.Identity) && o.Identity == s.Identity
}

func (c *SDCoordinator) startRepair(o SDObservation) (string, error) {
	s := c.State
	if !o.Ready || o.Presence != "present" || !validID(o.Identity) || !validID(o.Card) ||
		!validID(o.PackageDigest) || o.PackageRelease != o.CurrentRelease {
		return "repair-not-ready", nil
	}
	if s.Identity != "" && s.Identity != o.Identity {
		return "repair-identity-changed", nil
	}
	if s.Phase == "intent" || s.Phase == "dispatched" {
		if !landed(s, o) || s.Digest != o.PackageDigest {
			return "pending-outcome-no-retry", nil
		}
	} else if s.Phase == "staged" {
		if s.Digest != o.PackageDigest || s.SourceSlot != o.Slot || s.Card != o.Card {
			return "different-package-pending", nil
		}
	} else if s.Phase == "uncertain" {
		return "quarantined", nil
	}
	// A visible user-supplied file is consumed before intent. The hidden copy
	// left by a previous attempt needs a witnessed physical remove/insert.
	if !o.NewPresentation {
		return "left-card-suppressed", nil
	}
	s = SDState{Version: 1, Phase: "resident", Armed: true, Repair: true, Boot: o.Boot, SourceSlot: o.Slot,
		Digest: o.PackageDigest, Release: o.PackageRelease, Card: o.Card, Identity: o.Identity}
	if err := c.put(s); err != nil {
		return "", err
	}
	return "repair-armed", nil
}

// A validated consumed firmware can be replaced at the later consume step,
// after the new input was verified and staged. No deletion happens here.
// This applies to healthy upgrades as well as failed-service repair.
func validatedConsumed(path string, pub []byte, p policy) (os.FileInfo, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fail("ambiguous SD packages")
	}
	_, _, _, err = inspectCard(f, pub, p)
	if err != nil {
		return nil, fail("conflicting consumed package")
	}
	return st, nil
}
