package main

import (
	"fmt"
)

type InstallReadiness struct {
	CurrentVersion                                                                 string
	RootUID, GUIUID                                                                int
	SSHIdentityPinned, GUIIdentityPinned, SSHWrongKeyRejected, GUIWrongKeyRejected bool
	Idle, PowerStable, SlotsVerified, HoldPresent                                  bool
	PendingAttempt                                                                 bool
}

// BuildHandoff is a non-executing plan. It consumes the real private-file verifier
// and rejects incomplete readiness. Callers cannot obtain a plan from SD metadata
// alone. The native current-version value is supplied by the future coordinator.
// There is intentionally no default executor or boot service releasing OTA holds.
func BuildHandoff(p policy, r InstallReadiness) ([][]string, error) {
	if !versionRE.MatchString(r.CurrentVersion) || r.RootUID != 0 || r.GUIUID != 0 || !r.SSHIdentityPinned || !r.GUIIdentityPinned || !r.SSHWrongKeyRejected || !r.GUIWrongKeyRejected || !r.Idle || !r.PowerStable || !r.SlotsVerified || !r.HoldPresent || r.PendingAttempt {
		return nil, fail("install readiness incomplete")
	}
	pub, e := readSmall(p.Public, p, false, 32)
	if e != nil {
		return nil, e
	}
	a, e := readSmall(p.Package+".owner-auth", p, true, 200)
	if e != nil {
		return nil, e
	}
	m, e := authenticate(a, pub, p)
	if e != nil {
		return nil, e
	}
	if _, e = verify(p, p.Package, m.Version, true); e != nil {
		return nil, e
	}
	if compareVersion(m.Version, r.CurrentVersion) < 0 && !m.Downgrade {
		return nil, fail("downgrade not authorized for this package")
	}
	if m.Base != p.Base {
		return nil, fmt.Errorf("different maintenance base")
	}
	return [][]string{
		{"REQUEST_VALIDATED_INSTALL_WINDOW"},
		{"/bin/systemctl", "start", "ins_otad.service"},
		{"WAIT_NATIVE_INITIALIZED_IDLE"},
		{"/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kOtad", "Upgrade", "UpgradeProcessFirmwarePath", p.Package},
		{"/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kOtad", "Upgrade", "UpgradeProcessEvent", "OtaEventRecvSdFwRequest", "\"\""},
		{"WAIT_NATIVE_PREPROCESS"},
		{"/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kOtad", "Upgrade", "UpgradeProcessEvent", "OtaEventStoragePkgFind", p.Package},
	}, nil
}
