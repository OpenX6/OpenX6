package main

import (
	"errors"
	"fmt"
	"log"
	"time"
)

type receiverProcess struct {
	PID   int
	Start string
}

// /proc/PID/exe can disappear before /proc/PID/stat reports a zombie.
// This is NOT proof of exit and must never authorize a signal or restart.
type receiverExitPending struct {
	Process receiverProcess
	State   string
	Err     error
}

func (e *receiverExitPending) Error() string {
	return fmt.Sprintf("receiver pid %d start %s state %s has no executable yet: %v", e.Process.PID, e.Process.Start, e.State, e.Err)
}

type receiverOps interface {
	Snapshot() ([]receiverProcess, error)
	Terminate(receiverProcess) error
	KillStoppedTransport(receiverProcess) error
	Now() time.Time
	Pause(time.Duration)
}

// A successful TERM is not an exit acknowledgement. Wait for the exact
// receiver to disappear and for a quiet interval before HAL may restart it.
// After the grace interval, only an identity-checked receiver whose USB
// transport is already closed may be killed. Exit is still mandatory.
func stopReceiver(o receiverOps) error {
	deadline := o.Now().Add(8 * time.Second)
	forced := false
	initial, e := o.Snapshot()
	if e != nil {
		return e
	}
	if len(initial) > 1 {
		return fmt.Errorf("multiple USB receivers; refusing ambiguous stop")
	}
	if len(initial) == 1 {
		if e = o.Terminate(initial[0]); e != nil {
			return e
		}
	}
	var absentSince time.Time
	pendingLogged := false
	for {
		current, e := o.Snapshot()
		if e != nil {
			var pending *receiverExitPending
			// Only an already-validated receiver to which we requested TERM
			// may be observed again. Keep waiting; do not call it stopped.
			if errors.As(e, &pending) && len(initial) == 1 && pending.Process == initial[0] {
				if !o.Now().Before(deadline) {
					return fmt.Errorf("receiver exit unresolved; no restart permitted: %w", e)
				}
				absentSince = time.Time{}
				if !pendingLogged {
					log.Printf("waiting for confirmed receiver exit: %v", e)
					pendingLogged = true
				}
				o.Pause(50 * time.Millisecond)
				continue
			}
			return e
		}
		if len(current) > 1 {
			return fmt.Errorf("multiple receivers appeared during stop")
		}
		if len(current) == 0 {
			if absentSince.IsZero() {
				absentSince = o.Now()
			}
			if o.Now().Sub(absentSince) >= 100*time.Millisecond {
				return nil
			}
		} else {
			if len(initial) != 1 || current[0] != initial[0] {
				return fmt.Errorf("receiver identity changed during stop")
			}
			absentSince = time.Time{}
		}
		if !o.Now().Before(deadline) {
			if forced || len(initial) != 1 || len(current) != 1 || current[0] != initial[0] {
				return fmt.Errorf("USB receiver exit not confirmed; no restart permitted")
			}
			if e = o.KillStoppedTransport(initial[0]); e != nil {
				return fmt.Errorf("receiver did not exit within eight seconds; bounded cleanup refused: %w", e)
			}
			log.Printf("graceful exit timed out; verified closed USB transport, waiting for pidfd cleanup of pid %d start %s", initial[0].PID, initial[0].Start)
			forced = true
			deadline = o.Now().Add(3 * time.Second)
		}
		o.Pause(50 * time.Millisecond)
	}
}
