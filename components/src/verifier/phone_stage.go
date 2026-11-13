package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func phoneIdle(cfg SDPolicy) bool {
	if !cfg.AutomaticEnabled || !mediaIdle() || !otaHeld() || noUSBExport() != nil {
		return false
	}
	for _, path := range []string{cardInputPath, consumedCardPath, "/data/ins/Insta360X6FW.bin"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// The receipt binds the exact HTTP file part to the independent readback of
// the copied file. The anonymous input has no pathname that a card can replace.
func (u phoneFile) inspect(f *os.File, pub []byte, p policy, progress digestProgress) (manifest, []byte, int64, error) {
	if f != u.f {
		return inspectCardObserved(f, pub, p, progress)
	}
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() != u.size {
		return manifest{}, nil, -1, fail("mobile upload size changed")
	}
	b := make([]byte, embeddedSize)
	copy(b, nativeCopyMagic)
	binary.LittleEndian.PutUint64(b[8:16], uint64(u.size))
	copy(b[16:48], u.digest[:])
	m, e := copyManifest(b)
	m.Factory = true
	return m, b, -1, e
}

func (u phoneFile) stage(p policy, card, boot string, notice *stageNotice) error {
	digest := fmt.Sprintf("%x", u.digest)
	if _, e := os.Lstat(p.Package); e == nil {
		m, e := stagedRecord(p)
		if e != nil || !m.Native || m.Digest != u.digest {
			return fail("different private package staged")
		}
		_, e = verify(p, p.Package, "", false)
		return e
	} else if !os.IsNotExist(e) {
		return e
	}
	avail, e := freeBytes("/data")
	if e != nil {
		return e
	}
	var sd uint64
	if card != "" {
		sd, _ = freeBytes("/mnt/sdcard")
	}
	plan, e := planInputStage(uint64(u.size), avail, sd, true, importMargin)
	if e != nil {
		return e
	}
	if plan.Backend == "data" {
		return stageInput(u.f, p, nil, u.inspect, notice)
	}
	if e = preparePrivateSD(plan, card, digest, boot, notice); e != nil {
		return e
	}
	q := p
	q.Package = sdStagePackage
	if e = stageInput(u.f, q, nil, u.inspect, notice); e != nil {
		return e
	}
	return publishPrivateSD()
}

// Called on the SD coordinator's goroutine. Both transports use the same
// journal, durable intent and recovery ownership of the staged bytes.
func installPhone(u phoneFile, c *SDCoordinator, cfg SDPolicy, p policy, id, boot, slot string) error {
	digest := fmt.Sprintf("%x", u.digest)
	if !phoneIdle(cfg) || !stagingSpace(uint64(u.size)) {
		return fail("camera no longer ready")
	}
	_, card, _ := cardObservation("")
	var notice stageNotice
	notice.begin("/run/x6-sd-update-notice")
	defer notice.close()
	step := *c
	step.ConfirmFactory = func() bool { return true } // both on-body answers were required for this request
	step.Stage = func() error {
		if e := retireVerifiedStage(p, c.State, digest); e != nil {
			return e
		}
		return u.stage(p, card, boot, &notice)
	}
	step.Restage = func() error { return u.stage(p, card, boot, &notice) }
	step.Prepare = func() error {
		if !phoneIdle(cfg) {
			return fail("mobile handoff readiness changed")
		}
		if err := checkPrivateSD(); err != nil {
			return err
		}
		return checkHandoffSpace(p, freeBytes)
	}
	step.Dispatch = func() error {
		m, e := stagedRecord(p)
		if e != nil || !m.Native || m.Digest != u.digest {
			return fail("mobile staged receipt mismatch")
		}
		return dispatchNativeObserved(cfg, func() bool { return noUSBExport() == nil }, &notice, notice.close)
	}
	obs := SDObservation{Boot: boot, Slot: slot, CurrentRelease: id, Presence: "present", Card: card, PackageDigest: digest, PackageRelease: digest, Ready: true, InstallAllowed: cfg.AutomaticEnabled, Native: true, Factory: true}
	status, e := step.Step(obs)
	c.State = step.State
	if e != nil {
		return e
	}
	if status != "dispatched" {
		return fmt.Errorf("mobile update deferred: %s", status)
	}
	return nil
}
