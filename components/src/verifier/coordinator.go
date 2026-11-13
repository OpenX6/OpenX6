package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// State is durable before any OTA handoff. Unknown outcomes are quarantined,
// never automatically retried. An observed removal must follow presence in this
// boot; boot-time enumeration gaps and UDisk unmounts do not re-arm a card.
type SDState struct {
	Version                                                  int
	Phase, Boot, SourceSlot, Digest, Release, Card, Identity string
	Native                                                   bool `json:",omitempty"`
	Armed, Repair                                            bool
}
type SDObservation struct {
	Boot, Slot, CurrentRelease, Card, Identity string
	Presence                                   string // present, absent, unknown. Not merely mounted/unmounted.
	PackageDigest, PackageRelease              string // authenticated, not raw SD metadata
	Ready, InstallAllowed, CompletionVerified  bool
	Factory                                    bool
	Native, NativeIdle                         bool
	Repair, NewPresentation                    bool
}
type SDCoordinator struct {
	State          SDState
	SeenPresent    bool
	SessionBoot    string
	Save           func(SDState) error
	ConfirmFactory func() bool
	Stage          func() error
	Restage        func() error
	Prepare        func() error
	Dispatch       func() error
	Fault          func(string) error
}

func validID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func validState(s SDState) bool {
	if s.Version != 1 {
		return false
	}
	switch s.Phase {
	case "idle":
		return s.Digest == ""
	case "resident", "staged", "intent", "dispatched", "completed", "uncertain":
	default:
		return false
	}
	return validID(s.Digest) && validID(s.Release) && s.Boot != "" && (s.SourceSlot == "_a" || s.SourceSlot == "_b")
}
func (c *SDCoordinator) put(s SDState) error {
	if !validState(s) {
		return fail("invalid coordinator state")
	}
	if err := c.Save(s); err != nil {
		return err
	}
	c.State = s
	return nil
}
func (c *SDCoordinator) hit(s string) error {
	if c.Fault != nil {
		return c.Fault(s)
	}
	return nil
}
func (c *SDCoordinator) Step(o SDObservation) (string, error) {
	if o.Boot == "" || (o.Slot != "_a" && o.Slot != "_b") || !validID(o.CurrentRelease) {
		return "", fail("invalid boot observation")
	}
	if c.State.Version == 0 {
		c.State = SDState{Version: 1, Phase: "idle"}
	}
	if !validState(c.State) {
		return "", fail("invalid durable state")
	}
	if c.SessionBoot != o.Boot {
		c.SessionBoot = o.Boot
		c.SeenPresent = false
	}
	s := c.State
	if o.Repair {
		resume := s.Repair && (s.Phase == "staged" || (s.Phase == "resident" && s.Armed))
		if !resume {
			return c.startRepair(o)
		}
		if !o.Ready || o.Presence != "present" || o.CurrentRelease != o.PackageRelease || o.PackageDigest != s.Digest || o.Card != s.Card || o.Identity != s.Identity || o.Slot != s.SourceSlot || o.PackageRelease != s.Release {
			return "repair-context-changed", nil
		}
	}
	if s.Phase == "intent" || s.Phase == "dispatched" {
		if s.Native && s.Boot != o.Boot && o.NativeIdle {
			// The next boot is idle. No assertion about the installed contents or slot.
			s.Phase = "completed"
			s.Armed = false
			if err := c.put(s); err != nil {
				return "", err
			}
			return "native-session-ended", nil
		}
		if !s.Native && s.Boot != o.Boot && o.Slot != s.SourceSlot && o.CurrentRelease == s.Release && (s.Identity == "" || s.Identity == o.Identity) && o.CompletionVerified && o.Ready {
			s.Phase = "completed"
			s.Armed = false
			if err := c.put(s); err != nil {
				return "", err
			}
			return "completed", nil
		}
		// A healthy new target may be late to initialize. Remain quarantined; a later
		// observation can verify completion. Neither removal nor another boot retries.
		return "pending-outcome-no-retry", nil
	}
	if s.Phase == "uncertain" {
		return "quarantined", nil
	}
	if o.Presence == "present" {
		c.SeenPresent = true
	} else if o.Presence == "absent" {
		if c.SeenPresent && !s.Armed {
			s.Armed = true
			if err := c.put(s); err != nil {
				return "", err
			}
		}
		return "no-card", nil
	} else {
		return "presence-unknown", nil
	}
	if o.PackageDigest == "" {
		return "no-authenticated-package", nil
	}
	if !validID(o.PackageDigest) || !validID(o.PackageRelease) {
		return "", fail("invalid package identity")
	}
	if !o.Ready {
		return "not-ready", nil
	}
	same := s.Digest == o.PackageDigest
	if s.Phase == "idle" && !s.Armed && o.PackageRelease == o.CurrentRelease {
		s = SDState{Version: 1, Phase: "resident", Boot: o.Boot, SourceSlot: o.Slot, Digest: o.PackageDigest, Release: o.PackageRelease, Card: o.Card, Identity: o.Identity}
		if err := c.put(s); err != nil {
			return "", err
		}
		return "resident-suppressed", nil
	}
	if same && !s.Armed && (s.Phase == "completed" || s.Phase == "resident") {
		return "left-card-suppressed", nil
	}
	if s.Phase == "staged" && !same {
		return "different-package-pending", nil
	}
	if !o.Ready {
		return "not-ready", nil
	}
	if o.Factory && (!o.InstallAllowed || c.ConfirmFactory == nil || !c.ConfirmFactory()) {
		return "factory-confirmation-required", nil
	}
	if s.Phase != "staged" {
		if err := c.Stage(); err != nil {
			return "", err
		}
		if err := c.hit("staged-before-save"); err != nil {
			return "", err
		}
		s = SDState{Version: 1, Phase: "staged", Native: o.Native, Repair: o.Repair, Boot: o.Boot, SourceSlot: o.Slot, Digest: o.PackageDigest, Release: o.PackageRelease, Card: o.Card, Identity: o.Identity}
		if err := c.put(s); err != nil {
			return "", err
		}
		if err := c.hit("staged"); err != nil {
			return "", err
		}
	}
	if c.Restage != nil {
		if err := c.Restage(); err != nil {
			return "restage-refused", err
		}
	}
	if s.SourceSlot != o.Slot || (s.Identity != "" && s.Identity != o.Identity) {
		return "source-identity-changed", fail("source state changed before handoff")
	}
	s.Boot = o.Boot
	if !o.InstallAllowed {
		return "staged-install-locked", nil
	}
	if c.Prepare != nil {
		if err := c.Prepare(); err != nil {
			return "prepare-refused", err
		}
	}
	s.Boot = o.Boot
	s.SourceSlot = o.Slot
	s.Phase = "intent"
	s.Armed = false
	if err := c.put(s); err != nil {
		return "", err
	}
	if err := c.hit("intent"); err != nil {
		return "", err
	}
	// Intent is committed BEFORE dispatch. Dispatch failure is ambiguous unless
	// the native updater independently proves no work occurred; never guess.
	if err := c.Dispatch(); err != nil {
		// This refusal is raised before the native updater receives a path or event.
		if errors.Is(err, errHandoffSpace) {
			s.Phase = "staged"
			if e := c.put(s); e != nil {
				return "", e
			}
			return "prepare-refused", err
		}
		return "dispatch-outcome-unknown", err
	}
	if err := c.hit("dispatched-before-save"); err != nil {
		return "", err
	}
	s.Phase = "dispatched"
	if err := c.put(s); err != nil {
		return "", err
	}
	return "dispatched", c.hit("dispatched")
}
func decodeState(b []byte) (SDState, error) {
	var s SDState
	if len(b) > 8192 {
		return s, fail("oversize state")
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if !validState(s) {
		return s, fmt.Errorf("invalid journal")
	}
	return s, nil
}
