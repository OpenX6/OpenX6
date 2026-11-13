package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// productionSeed is generated from the locally built feature manifest.
const journalDir = "data/x6-firstboot-state"

type NativeConfig struct {
	Engine Engine
	Call   func([]byte) ([]byte, error)
}

func (n NativeConfig) request(cmd map[string]any) (map[string]json.RawMessage, error) {
	b, e := json.Marshal(cmd)
	if e != nil {
		return nil, e
	}
	b, e = n.Call(b)
	if e != nil {
		return nil, e
	}
	if len(b) > 65536 {
		return nil, errors.New("native response too large")
	}
	var r struct {
		Ret  string                     `json:"ret"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	if r.Ret != "ok" {
		return nil, errors.New("native config rejected")
	}
	return r.Data, nil
}
func (n NativeConfig) Get(k string) (map[string]json.RawMessage, error) {
	if !initialKey(k) {
		return nil, errors.New("unapproved config key")
	}
	d, e := n.request(map[string]any{"cmd": "config", "action": "get_general", "key": k})
	if e == nil && d == nil {
		e = errors.New("missing config object")
	}
	return d, e
}
func (n NativeConfig) Set(k string, v map[string]json.RawMessage) error {
	if !initialKey(k) {
		return errors.New("unapproved config key")
	}
	_, e := n.request(map[string]any{"cmd": "config", "action": "set_general", "key": k, "value": v})
	return e
}
func initialKey(k string) bool {
	for _, s := range initialSettings {
		if s.key == k {
			return true
		}
	}
	return false
}
func (n NativeConfig) LoadJournal() ([]byte, error) {
	b, e := n.Engine.read(journalDir+"/settings.json", 65536, 0600)
	if os.IsNotExist(e) {
		return nil, nil
	}
	return b, e
}
func (n NativeConfig) SaveJournal(b []byte) error {
	if len(b) > 65536 || !json.Valid(b) {
		return errors.New("invalid journal")
	}
	e := n.Engine
	if err := e.makeDir(journalDir); err != nil {
		return err
	}
	if _, err := e.read(journalDir+"/settings.json", 65536, 0600); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(e.path(journalDir), ".settings-")
	if err != nil {
		return err
	}
	p := f.Name()
	defer os.Remove(p)
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = e.hit("settings-before-rename"); err != nil {
		return err
	}
	if err = os.Rename(p, e.path(journalDir+"/settings.json")); err != nil {
		return err
	}
	if err = syncDir(e.path(journalDir)); err != nil {
		return err
	}
	return e.hit("settings-after-rename")
}
func (n NativeConfig) BindDevice() error {
	d, err := n.request(map[string]any{"cmd": "share_state", "action": "get", "type": "bll"})
	if err != nil {
		return err
	}
	var serial string
	if json.Unmarshal(d["host_serial_number"], &serial) != nil || len(serial) < 10 || len(serial) > 128 {
		return errors.New("missing device identity")
	}
	want := []byte(digest([]byte(serial)) + "\n")
	e := n.Engine
	if err = e.makeDir(journalDir); err != nil {
		return err
	}
	old, err := e.read(journalDir+"/device.sha256", 65, 0600)
	if err == nil {
		if string(old) != string(want) {
			return errors.New("device binding mismatch")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return e.write(journalDir, "device.sha256", 0600, func(f *os.File) error { _, x := f.Write(want); return x })
}

type cappedBuffer struct{ b []byte }

func (c *cappedBuffer) Write(b []byte) (int, error) {
	if len(c.b)+len(b) > 65536 {
		return 0, errors.New("native output limit")
	}
	c.b = append(c.b, b...)
	return len(b), nil
}
func nativeCall(b []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/ins_bll_cli", string(b))
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	out := &cappedBuffer{}
	cmd.Stdout = out
	cmd.Stderr = &cappedBuffer{}
	err := cmd.Run()
	return out.b, err
}
func targetEntry(action string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 {
		return errors.New("target Linux ARM64 root required")
	}
	e, err := targetEngine("/", productionSeed)
	if err != nil {
		return err
	}
	if _, _, err := e.load(); err != nil {
		return err
	}
	// Run verifies every installed base and seed before writing the private feature
	// namespace. No root key, service, partition, OTA hold or mount is changed here.
	if action == "prepare" {
		return e.Run()
	}
	if action != "settings" {
		return errors.New("unknown target action")
	}
	if err := e.makeDir(journalDir); err != nil {
		return err
	}
	// The feature seed runs best-effort and only when the boot-loop breaker
	// allowed feature layers this boot. The SSH-reachability settings below
	// never wait on it (Run takes its own lock, so it stays outside ours).
	e.featureLayer(e.featuresAllowed())
	release, err := e.lock()
	if err != nil {
		return err
	}
	defer release()
	n := NativeConfig{Engine: e, Call: nativeCall}
	if err = n.BindDevice(); err != nil {
		return err
	}
	if err = e.ApplySettings(n); err != nil {
		return fmt.Errorf("first setup: %w", err)
	}
	return nil
}
