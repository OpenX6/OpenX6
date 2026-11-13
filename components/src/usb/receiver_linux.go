//go:build linux

package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"time"
)

type linuxReceiver struct{ procReceiver }

func (o linuxReceiver) Terminate(p receiverProcess) error {
	return o.signal(p, unix.SIGTERM, false)
}
func (o linuxReceiver) KillStoppedTransport(p receiverProcess) error {
	return o.signal(p, unix.SIGKILL, true)
}
func (o linuxReceiver) signal(p receiverProcess, sig unix.Signal, requireClosed bool) error {
	fd, e := unix.PidfdOpen(p.PID, 0)
	if errors.Is(e, unix.ESRCH) {
		return nil
	}
	if e != nil {
		return fmt.Errorf("pidfd_open (no unsafe fallback): %w", e)
	}
	defer unix.Close(fd)
	// Recheck identity after opening the pidfd. PID reuse before or after this
	// point can never redirect the signal to a new process.
	now, ok, e := o.identity(p.PID)
	if e != nil {
		return e
	}
	if !ok {
		return nil
	}
	if now != p {
		return fmt.Errorf("PID identity changed before signal")
	}
	if requireClosed {
		if e = o.closedTransport(p); e != nil {
			return e
		}
		// Recheck after inspecting descriptors and threads. The pidfd remains
		// tied to the original task even if its numeric PID is reused later.
		now, ok, e = o.identity(p.PID)
		if e != nil {
			return e
		}
		if !ok {
			return nil
		}
		if now != p {
			return fmt.Errorf("identity changed during cleanup checks")
		}
	}
	e = unix.PidfdSendSignal(fd, sig, nil, 0)
	if errors.Is(e, unix.ESRCH) {
		return nil
	}
	return e
}
func stopInsUSB() error {
	if e := verifyCandidate(); e != nil {
		return e
	}
	if e := privateRunDir(); e != nil {
		return e
	}
	fd, e := unix.Open(runtimeDir+"/receiver.lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if e = unix.Fstat(fd, &st); e != nil {
		return e
	}
	if st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 {
		return fmt.Errorf("unsafe receiver lock")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if e == nil {
			break
		}
		if !errors.Is(e, unix.EWOULDBLOCK) || !time.Now().Before(deadline) {
			return fmt.Errorf("receiver lock unavailable: %w", e)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	return stopReceiver(linuxReceiver{procReceiver{proc: "/proc"}})
}
