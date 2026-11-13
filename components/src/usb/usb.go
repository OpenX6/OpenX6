package main

import (
	"errors"
	"fmt"
)

// Transport orchestration is separated from host operations so failures can
// be injected at every boundary without accessing a real gadget or camera.
type usbOps interface {
	Preflight(string) error
	Lock() error
	Unlock()
	Mark() error
	Unmark() error
	Unbind() error
	StopInsUSB() error
	StopADB() error
	Configure(string) error
	StartAuth() error
	InvalidateAuth() error
	StartADB() error
	WaitReady() error
	Bind() error
}

func switchUSB(o usbOps, mode string) (err error) {
	if mode != "C001" && mode != "901D" {
		return fmt.Errorf("unsupported maintenance composition")
	}
	// Preflight must succeed before disturbing a working USB interface.
	if err = o.Preflight(mode); err != nil {
		return err
	}
	if err = o.Lock(); err != nil {
		return err
	}
	defer o.Unlock()
	if err = o.Mark(); err != nil {
		return err
	}
	// Do not disturb the old ADB process if the first unbind fails. No new
	// composition has been configured yet; remove only our transition markers.
	if err = o.Unbind(); err != nil {
		return errors.Join(err, o.Unmark())
	}
	// Later failure attempts unbind and stops ADB, keeping the inhibit marker.
	// Cleanup errors are reported: a kernel refusing UDC writes cannot be
	// claimed to be unbound. A separately verified root route is still required.
	defer func() {
		if err != nil {
			err = errors.Join(err, o.Unbind(), o.StopADB())
		}
	}()
	for _, step := range []func() error{o.StopInsUSB, o.StopADB, func() error { return o.Configure(mode) }, o.StartAuth, o.InvalidateAuth, o.StartADB, o.WaitReady, o.Bind, o.Unmark} {
		if err = step(); err != nil {
			return err
		}
	}
	return nil
}
