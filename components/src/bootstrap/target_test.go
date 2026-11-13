package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func nativeFixture(e Engine, m *memoryConfig) NativeConfig {
	return NativeConfig{Engine: e, Call: func(b []byte) ([]byte, error) {
		var q struct {
			Cmd, Action, Key string
			Value            map[string]json.RawMessage
		}
		if err := json.Unmarshal(b, &q); err != nil {
			return nil, err
		}
		if q.Cmd == "share_state" {
			return []byte(`{"ret":"ok","data":{"host_serial_number":"TEST-DEVICE-12345"}}`), nil
		}
		if q.Cmd != "config" {
			return nil, errors.New("unknown command")
		}
		if q.Action == "set_general" {
			if err := m.Set(q.Key, q.Value); err != nil {
				return nil, err
			}
			return []byte(`{"ret":"ok"}`), nil
		}
		if q.Action != "get_general" {
			return nil, errors.New("unknown action")
		}
		d, err := m.Get(q.Key)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"ret": "ok", "data": d})
	}}
}
func TestNativeAdapterDurableAndPreserving(t *testing.T) {
	f := fixture(t)
	m := configFixture()
	n := nativeFixture(f.e, m)
	if err := n.BindDevice(); err != nil {
		t.Fatal(err)
	}
	identity, _ := os.ReadFile(f.e.path(journalDir + "/device.sha256"))
	if err := f.e.Bootstrap(n); err != nil {
		t.Fatal(err)
	}
	first, err := n.LoadJournal()
	if err != nil {
		t.Fatal(err)
	}
	m.data["kSystemAutoShutdown"]["auto_power_off_time"] = json.RawMessage(`90`)
	m.data["kDeviceActiveTime"]["is_active"] = json.RawMessage(`false`)
	if err = f.e.Bootstrap(n); err != nil {
		t.Fatal(err)
	}
	second, _ := n.LoadJournal()
	if !bytes.Equal(first, second) || string(m.data["kSystemAutoShutdown"]["auto_power_off_time"]) != "90" || string(m.data["kDeviceActiveTime"]["is_active"]) != "false" {
		t.Fatal("repeat changed user settings")
	}
	if err = n.BindDevice(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(f.e.path(journalDir + "/device.sha256"))
	if !bytes.Equal(identity, after) {
		t.Fatal("identity rotated")
	}
	untouched(t, f.e)
}
func TestNativeJournalInterruption(t *testing.T) {
	for _, point := range []string{"settings-before-rename", "settings-after-rename"} {
		t.Run(point, func(t *testing.T) {
			f := fixture(t)
			m := configFixture()
			e := f.e
			e.Fault = func(s string) error {
				if s == point {
					return errors.New("interrupted")
				}
				return nil
			}
			if InitializeSettings(nativeFixture(e, m), e.Pin, nil) == nil {
				t.Fatal("expected failure")
			}
			if err := InitializeSettings(nativeFixture(f.e, m), e.Pin, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestNativeAdapterRejects(t *testing.T) {
	for _, value := range []string{`{"ret":"failed"}`, `{"ret":"ok","data":null}`, `not json`, strings.Repeat("x", 65537)} {
		t.Run(value[:8], func(t *testing.T) {
			f := fixture(t)
			n := NativeConfig{Engine: f.e, Call: func([]byte) ([]byte, error) { return []byte(value), nil }}
			if _, err := n.Get("kNetdConfig"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	f := fixture(t)
	m := configFixture()
	n := nativeFixture(f.e, m)
	if _, err := n.Get("kUnknown"); err == nil {
		t.Fatal("unknown key")
	}
	if err := n.BindDevice(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f.e.path(journalDir+"/device.sha256"), []byte(strings.Repeat("a", 64)+"\n"), 0600)
	if err := n.BindDevice(); err == nil {
		t.Fatal("foreign identity")
	}
	if err := n.SaveJournal([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	os.Chmod(f.e.path(journalDir+"/settings.json"), 0666)
	if _, err := n.LoadJournal(); err == nil {
		t.Fatal("unsafe journal")
	}
}
