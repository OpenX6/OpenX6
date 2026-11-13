package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// bootGuardAllow is written by x6-boot-guard (src/boot-guard) only after it
// counted this boot and allowed feature layers. Keep the two in sync.
const bootGuardAllow = "run/x6-boot/features-allowed"

// Bootstrap is the host-testable form of the target "settings" action with
// feature layers allowed. Production native service I/O is deliberately
// absent; tests supply a persisted mock ConfigIO.
func (e Engine) Bootstrap(io ConfigIO) error {
	e.featureLayer(true)
	return e.ApplySettings(io)
}

// BootstrapWithoutFeatures is the same action on a boot where the boot-loop
// breaker skipped feature layers: the feature seed is never touched.
func (e Engine) BootstrapWithoutFeatures(io ConfigIO) error {
	e.featureLayer(false)
	return e.ApplySettings(io)
}

// featureLayer prepares the owner feature namespace best-effort. The native
// settings that keep the camera reachable over SSH (Wi-Fi always on, no auto
// power-off, activated) must not depend on it: a failed or skipped feature
// seed is reported and the settings still proceed. The feature seed unit
// reports its own failure separately.
func (e Engine) featureLayer(allowed bool) {
	if !allowed {
		fmt.Fprintln(os.Stderr, "X6_FIRSTBOOT: feature layers skipped by the boot guard; native settings continue")
		return
	}
	if err := e.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "X6_FIRSTBOOT: feature layer not prepared (%v); native settings continue\n", err)
	}
}

func (e Engine) featuresAllowed() bool {
	_, err := os.Lstat(e.path(bootGuardAllow))
	return err == nil
}

// preserved reports whether this device adopted an existing installation
// whose native settings must never be initialized. A receipt written by any
// release counts: the statement "the owner had already configured this
// camera" does not expire when the seed changes.
func (e Engine) preserved() (bool, error) {
	// Recovery has already verified a configured source. Preserve native choices
	// even when feature migration is deferred by the boot guard on this boot.
	if e.Migration != nil && e.Migration.LocalHolder && len(e.Migration.Previous) == 1 {
		if err := e.verifyOwners(e.Migration.Previous[0]); err != nil {
			return false, err
		}
		return true, nil
	}
	b, err := e.read(featurePath+"/preserve-native-settings", 128, 0600)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s := strings.TrimSuffix(string(b), "\n")
	if !validHash(s) {
		return false, errors.New("unrecognized preservation receipt")
	}
	if s != e.Pin {
		fmt.Fprintln(os.Stderr, "X6_FIRSTBOOT: preservation receipt from another release; native settings preserved")
	}
	return true, nil
}

// ApplySettings initializes first-use native settings unless the device was
// adopted with its settings preserved.
func (e Engine) ApplySettings(io ConfigIO) error {
	keep, err := e.preserved()
	if err != nil || keep {
		return err
	}
	return InitializeSettings(io, e.Pin, e.Fault)
}

// ConfigIO is deliberately not connected to ins_bll_cli in this offline build.
// A target adapter must use native get/set calls, bounded retries, a shared lock,
// and an atomically durable private journal. No exported old camera config is an input.
type ConfigIO interface {
	Get(string) (map[string]json.RawMessage, error)
	Set(string, map[string]json.RawMessage) error
	LoadJournal() ([]byte, error) // nil,nil means not initialized
	SaveJournal([]byte) error     // durable atomic commit contract
}
type setting struct {
	key, field string
	value      json.RawMessage
}

var initialSettings = []setting{
	{"kSystemAutoShutdown", "auto_power_off_time", json.RawMessage(`0`)},
	{"kNetdConfig", "wifi_enable_strategy", json.RawMessage(`"kAlwaysOn"`)},
	{"kDeviceActiveTime", "is_active", json.RawMessage(`true`)},
}

type configJournal struct {
	Version  int
	Seed     string
	Complete bool
	Original map[string]map[string]json.RawMessage
}

func cloneObject(v map[string]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, b := range v {
		out[k] = append(json.RawMessage(nil), b...)
	}
	return out
}
func equalJSON(a, b json.RawMessage) bool {
	var aa, bb bytes.Buffer
	if json.Compact(&aa, a) != nil || json.Compact(&bb, b) != nil {
		return false
	}
	return bytes.Equal(aa.Bytes(), bb.Bytes())
}
func InitializeSettings(io ConfigIO, pin string, checkpoint func(string) error) error {
	if !validHash(pin) {
		return errors.New("invalid seed identity")
	}
	hit := func(s string) error {
		if checkpoint != nil {
			return checkpoint(s)
		}
		return nil
	}
	raw, err := io.LoadJournal()
	if err != nil {
		return err
	}
	j := configJournal{Version: 1, Seed: pin, Original: map[string]map[string]json.RawMessage{}}
	if len(raw) > 0 {
		if len(raw) > 65536 {
			return errors.New("config journal too large")
		}
		if err = json.Unmarshal(raw, &j); err != nil {
			return err
		}
		if j.Version != 1 || j.Seed != pin || len(j.Original) != 3 {
			return errors.New("unknown config initialization journal")
		}
		for _, s := range initialSettings {
			v, ok := j.Original[s.key][s.field]
			if !ok || !json.Valid(v) {
				return errors.New("incomplete originals")
			}
		}
		if j.Complete {
			return nil
		} // Later user choices are not reset on every boot.
	} else {
		for _, s := range initialSettings {
			v, e := io.Get(s.key)
			if e != nil {
				return e
			}
			if _, ok := v[s.field]; !ok {
				return fmt.Errorf("missing native config field %s", s.field)
			}
			j.Original[s.key] = cloneObject(v)
		}
		raw, err = json.Marshal(j)
		if err != nil {
			return err
		}
		if len(raw) > 65536 {
			return errors.New("config snapshot too large")
		}
		if err = io.SaveJournal(raw); err != nil {
			return err
		}
		if err = hit("config-originals-saved"); err != nil {
			return err
		}
	}
	for _, s := range initialSettings {
		current, err := io.Get(s.key)
		if err != nil {
			return err
		}
		if _, ok := current[s.field]; !ok {
			return errors.New("native config changed schema")
		}
		want := cloneObject(current)
		want[s.field] = append(json.RawMessage(nil), s.value...)
		if !equalJSON(current[s.field], s.value) {
			if err = io.Set(s.key, want); err != nil {
				return err
			}
		}
		if err = hit("config-set:" + s.key); err != nil {
			return err
		}
		got, err := io.Get(s.key)
		if err != nil {
			return err
		}
		if len(got) != len(want) {
			return errors.New("native config readback shape differs")
		}
		for k, b := range want {
			if !equalJSON(got[k], b) {
				return fmt.Errorf("native config readback differs: %s/%s", s.key, k)
			}
		}
		if err = hit("config-verified:" + s.key); err != nil {
			return err
		}
	}
	j.Complete = true
	raw, err = json.Marshal(j)
	if err != nil {
		return err
	}
	if err = io.SaveJournal(raw); err != nil {
		return err
	}
	return hit("config-complete")
}
