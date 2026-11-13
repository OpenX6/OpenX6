package main

import (
	"fmt"
	"reflect"
	"testing"
)

type fakeUSB struct {
	calls  []string
	fail   string
	bound  bool
	marked bool
}

func (f *fakeUSB) step(s string) error {
	f.calls = append(f.calls, s)
	if s == f.fail {
		return fmt.Errorf("injected %s", s)
	}
	return nil
}
func (f *fakeUSB) Preflight(s string) error { return f.step("preflight") }
func (f *fakeUSB) Lock() error              { return f.step("lock") }
func (f *fakeUSB) Unlock()                  { f.step("unlock") }
func (f *fakeUSB) Mark() error {
	e := f.step("mark")
	if e == nil {
		f.marked = true
	}
	return e
}
func (f *fakeUSB) Unmark() error {
	e := f.step("unmark")
	if e == nil {
		f.marked = false
	}
	return e
}
func (f *fakeUSB) Unbind() error {
	e := f.step("unbind")
	if e == nil {
		f.bound = false
	}
	return e
}
func (f *fakeUSB) StopADB() error           { return f.step("stop-adb") }
func (f *fakeUSB) StopInsUSB() error        { return f.step("stop-ins-usb") }
func (f *fakeUSB) Configure(s string) error { return f.step("configure") }
func (f *fakeUSB) StartAuth() error         { return f.step("start-auth") }
func (f *fakeUSB) InvalidateAuth() error    { return f.step("invalidate-auth") }
func (f *fakeUSB) StartADB() error          { return f.step("start-adb") }
func (f *fakeUSB) WaitReady() error         { return f.step("wait-ready") }
func (f *fakeUSB) Bind() error {
	e := f.step("bind")
	if e == nil {
		if !f.marked {
			panic("binding without inhibit")
		}
		f.bound = true
	}
	return e
}

var successOrder = []string{"preflight", "lock", "mark", "unbind", "stop-ins-usb", "stop-adb", "configure", "start-auth", "invalidate-auth", "start-adb", "wait-ready", "bind", "unmark", "unlock"}

func TestUSBOrderAndFailureInjection(t *testing.T) {
	for _, mode := range []string{"C001", "901D"} {
		f := &fakeUSB{}
		if e := switchUSB(f, mode); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(f.calls, successOrder) || !f.bound || f.marked {
			t.Fatalf("invalid sequence: %v", f.calls)
		}
	}
	for _, step := range successOrder[:len(successOrder)-1] {
		t.Run(step, func(t *testing.T) {
			f := &fakeUSB{fail: step, bound: true}
			if e := switchUSB(f, "C001"); e == nil {
				t.Fatal("injected failure ignored")
			}
			preserveOld := step == "preflight" || step == "lock" || step == "mark" || step == "unbind"
			if f.bound != preserveOld {
				t.Fatal("early failure must preserve the old binding; later failure must attempt unbind")
			}
			if step == "unbind" {
				for _, call := range f.calls {
					if call == "stop-adb" || call == "configure" {
						t.Fatal("old interface disturbed after unbind failed")
					}
				}
			}
			if step == "preflight" && len(f.calls) != 1 {
				t.Fatal("USB modified despite preflight failure")
			}
			if step == "wait-ready" || step == "stop-ins-usb" {
				for _, c := range f.calls {
					if c == "bind" {
						t.Fatal("bound before readiness")
					}
				}
			}
		})
	}
	f := &fakeUSB{}
	if switchUSB(f, "../arbitrary") == nil || len(f.calls) != 0 {
		t.Fatal("invalid mode reached operations")
	}
}
