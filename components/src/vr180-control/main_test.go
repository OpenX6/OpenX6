package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStateAndAtomic(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "sync")
	for _, b := range [][]byte{{0}, {1}, {2}, {255}, {}, {1, 0}} {
		if e := atomic(p, b, 0600); e != nil {
			t.Fatal(e)
		}
		v, e := readState(p)
		valid := len(b) == 1 && b[0] < 2
		if valid && (e != nil || v != b[0]) {
			t.Fatal(v, e)
		}
		if !valid && e == nil {
			t.Fatal("invalid accepted")
		}
	}
	os.Remove(p)
	os.Symlink(filepath.Join(d, "unrelated"), p)
	if atomic(p, []byte{1}, 0600) == nil {
		t.Fatal("symlink destination")
	}
	if _, e := readState(p); e == nil {
		t.Fatal("symlink read")
	}
	if _, e := os.Stat(filepath.Join(d, "unrelated")); !os.IsNotExist(e) {
		t.Fatal("followed symlink")
	}
	os.Remove(p)
	os.WriteFile(p, []byte{1}, 0666)
	os.Chmod(p, 0666)
	if _, e := readState(p); e == nil {
		t.Fatal("writable state")
	}
}
func TestTransactions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		want  byte
		fail  string
		calls []byte
		state byte
	}{{"on", 1, "", []byte{1}, 1}, {"off", 0, "", []byte{0}, 0}, {"invalid", 2, "", nil, 0}, {"busy", 1, "guard", nil, 0}, {"unsupported", 1, "guard", nil, 0}, {"apply_fail", 1, "apply", []byte{1}, 0}, {"disk_fail", 1, "disk", []byte{1, 0}, 0}, {"live_fail", 1, "live", []byte{1, 0}, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			disk := filepath.Join(d, "disk")
			live := filepath.Join(d, "live")
			initial := byte(0)
			if tc.name == "off" {
				initial = 1
			}
			atomic(disk, []byte{initial}, 0600)
			atomic(live, []byte{initial}, 0600)
			var calls []byte
			e := engine{disk, live, func(byte) error {
				if tc.fail == "guard" {
					return errors.New("refused")
				}
				return nil
			}, func(v byte) error {
				calls = append(calls, v)
				if tc.fail == "apply" {
					return errors.New("backend")
				}
				return nil
			}, func(p string, b []byte, m os.FileMode) error {
				if (tc.fail == "disk" && p == disk) || (tc.fail == "live" && p == live) {
					return errors.New("write")
				}
				return atomic(p, b, m)
			}}
			err := e.set(tc.want)
			if (err == nil) != (tc.fail == "" && tc.want < 2) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tc.calls) {
				t.Fatal(calls, tc.calls)
			}
			v, err := readState(live)
			if err != nil || v != tc.state {
				t.Fatal(v, err)
			}
			v, err = readState(disk)
			if err != nil || v != tc.state {
				t.Fatal(v, err)
			}
		})
	}
}
func TestRestartReadsSavedPreference(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "sync")
	for _, v := range []byte{0, 1, 0, 1} {
		if e := atomic(p, []byte{v}, 0600); e != nil {
			t.Fatal(e)
		}
		got, e := readState(p)
		if e != nil || got != v {
			t.Fatal(got, e)
		}
	}
}
