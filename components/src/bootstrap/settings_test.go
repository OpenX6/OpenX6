package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCombinedFreshResumeVersusExistingSettings(t *testing.T) {
	t.Run("fresh feature publication then config service not ready", func(t *testing.T) {
		f := fixture(t)
		m := configFixture()
		m.failGet = true
		if f.e.Bootstrap(m) == nil {
			t.Fatal("must report pending native configuration")
		}
		if err := f.e.verifyFinal(f.m); err != nil {
			t.Fatal(err)
		}
		m.failGet = false
		if err := f.e.Bootstrap(m); err != nil {
			t.Fatal(err)
		}
		if string(m.data["kNetdConfig"]["wifi_enable_strategy"]) != `"kAlwaysOn"` || string(m.data["kDeviceActiveTime"]["is_active"]) != "false" {
			t.Fatal("fresh configuration not resumed")
		}
		untouched(t, f.e)
	})
	t.Run("existing modified features preserve all native settings", func(t *testing.T) {
		f := fixture(t)
		if err := f.e.Run(); err != nil {
			t.Fatal(err)
		}
		os.Remove(f.e.path(featurePath + "/complete.json"))
		m := configFixture()
		m.failGet = true
		if err := f.e.Bootstrap(m); err != nil {
			t.Fatal(err)
		}
		if m.journal != nil {
			t.Fatal("existing native configuration touched")
		}
		if string(m.data["kSystemAutoShutdown"]["auto_power_off_time"]) != "180" {
			t.Fatal("existing preference overwritten")
		}
		untouched(t, f.e)
	})
}

type memoryConfig struct {
	data                                  map[string]map[string]json.RawMessage
	journal                               []byte
	failGet, failSet, failSave, ignoreSet bool
}

func (m *memoryConfig) Get(k string) (map[string]json.RawMessage, error) {
	if m.failGet {
		return nil, errors.New("service not ready")
	}
	return cloneObject(m.data[k]), nil
}
func (m *memoryConfig) Set(k string, v map[string]json.RawMessage) error {
	if m.failSet {
		return errors.New("write rejected")
	}
	if !m.ignoreSet {
		m.data[k] = cloneObject(v)
	}
	return nil
}
func (m *memoryConfig) LoadJournal() ([]byte, error) { return append([]byte(nil), m.journal...), nil }
func (m *memoryConfig) SaveJournal(b []byte) error {
	if m.failSave {
		return errors.New("journal storage failure")
	}
	m.journal = append([]byte(nil), b...)
	return nil
}
func configFixture() *memoryConfig {
	return &memoryConfig{data: map[string]map[string]json.RawMessage{
		"kSystemAutoShutdown": {"auto_power_off_time": json.RawMessage(`180`), "rotate_shutdown_delay": json.RawMessage(`"k3Seconds"`)},
		"kNetdConfig":         {"wifi_enable_strategy": json.RawMessage(`"kAuto"`), "wlan0_mac": json.RawMessage(`"unique-this-device"`), "ap_passwd": json.RawMessage(`"test-only"`)},
		"kDeviceActiveTime":   {"is_active": json.RawMessage(`false`), "active_time": json.RawMessage(`0`)},
	}}
}
func TestSettingsResumeAndPreserve(t *testing.T) {
	pin := strings.Repeat("a", 64)
	points := []string{}
	m := configFixture()
	if err := InitializeSettings(m, pin, func(p string) error { points = append(points, p); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			m := configFixture()
			err := InitializeSettings(m, pin, func(p string) error {
				if p == point {
					return errors.New("interrupted")
				}
				return nil
			})
			if err == nil {
				t.Fatal("fault not reached")
			}
			if err = InitializeSettings(m, pin, nil); err != nil {
				t.Fatal(err)
			}
			if string(m.data["kNetdConfig"]["wlan0_mac"]) != `"unique-this-device"` || string(m.data["kNetdConfig"]["ap_passwd"]) != `"test-only"` {
				t.Fatal("device data replaced")
			}
			var j configJournal
			json.Unmarshal(m.journal, &j)
			if !j.Complete || j.Version != 2 || j.Original["kDeviceActiveTime"] != nil || string(j.Original["kSystemAutoShutdown"]["auto_power_off_time"]) != "180" {
				t.Fatal("original snapshot lost")
			}
			m.data["kSystemAutoShutdown"]["auto_power_off_time"] = json.RawMessage(`300`)
			if err = InitializeSettings(m, pin, nil); err != nil {
				t.Fatal(err)
			}
			if string(m.data["kSystemAutoShutdown"]["auto_power_off_time"]) != "300" {
				t.Fatal("user choice reset")
			}
		})
	}
}
func TestSettingsReject(t *testing.T) {
	for _, name := range []string{"get", "set", "save", "readback", "schema", "journal", "different-seed"} {
		t.Run(name, func(t *testing.T) {
			m := configFixture()
			pin := strings.Repeat("b", 64)
			switch name {
			case "get":
				m.failGet = true
			case "set":
				m.failSet = true
			case "save":
				m.failSave = true
			case "readback":
				m.ignoreSet = true
			case "schema":
				delete(m.data["kNetdConfig"], "wifi_enable_strategy")
			case "journal":
				m.journal = []byte(`{}`)
			case "different-seed":
				if err := InitializeSettings(m, strings.Repeat("c", 64), nil); err != nil {
					t.Fatal(err)
				}
			}
			if InitializeSettings(m, pin, nil) == nil {
				t.Fatal("accepted failure")
			}
		})
	}
}

// New installs need no activation object. Previous installers journaled it;
// the actual camera may have been officially activated since that snapshot.
func TestStockActivationIsUntouched(t *testing.T) {
	f := fixture(t)
	for _, state := range []string{"false", "true", "missing"} {
		for _, journal := range []string{"fresh", "legacy-pending", "legacy-complete"} {
			t.Run(state+"/"+journal, func(t *testing.T) {
				m := configFixture()
				if journal != "fresh" {
					old := configJournal{Version: 1, Seed: f.e.Pin, Complete: journal == "legacy-complete", Original: m.data}
					m.journal, _ = json.Marshal(old)
				}
				if state == "missing" {
					delete(m.data, "kDeviceActiveTime")
				} else {
					m.data["kDeviceActiveTime"]["is_active"] = json.RawMessage(state)
					m.data["kDeviceActiveTime"]["active_time"] = json.RawMessage("1234")
				}
				before, _ := json.Marshal(m.data["kDeviceActiveTime"])
				if err := InitializeSettings(m, f.e.Pin, nil); err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(m.data["kDeviceActiveTime"])
				if string(before) != string(after) {
					t.Fatal("activation state changed")
				}
				var receipt configJournal
				if err := json.Unmarshal(m.journal, &receipt); err != nil || !receipt.Complete {
					t.Fatal("settings initialization did not finish")
				}
			})
		}
	}
	n := NativeConfig{Call: func([]byte) ([]byte, error) {
		t.Fatal("activation reached the native configuration service")
		return nil, nil
	}}
	if _, err := n.Get("kDeviceActiveTime"); err == nil {
		t.Fatal("activation read permitted")
	}
	if err := n.Set("kDeviceActiveTime", nil); err == nil {
		t.Fatal("activation write permitted")
	}
}
