package main

import (
	"fmt"
	"testing"
	"time"
)

type simulatedReceiver struct {
	now                         time.Time
	age, exitAfter              time.Duration
	initial                     []receiverProcess
	term                        []receiverProcess
	kills                       []receiverProcess
	allowCleanup, ignoreKill    bool
	failScan, failTerm, replace bool
	scans                       int
}

func (f *simulatedReceiver) Now() time.Time        { return f.now.Add(f.age) }
func (f *simulatedReceiver) Pause(d time.Duration) { f.age += d }
func (f *simulatedReceiver) Snapshot() ([]receiverProcess, error) {
	f.scans++
	if f.failScan {
		return nil, fmt.Errorf("proc unavailable")
	}
	if f.replace && f.scans > 1 {
		return []receiverProcess{{42, "reused"}}, nil
	}
	if f.age >= f.exitAfter {
		return nil, nil
	}
	return f.initial, nil
}
func (f *simulatedReceiver) Terminate(p receiverProcess) error {
	if f.failTerm {
		return fmt.Errorf("pidfd unsupported/identity mismatch")
	}
	f.term = append(f.term, p)
	return nil
}
func (f *simulatedReceiver) KillStoppedTransport(p receiverProcess) error {
	if !f.allowCleanup {
		return fmt.Errorf("transport open, paused, or identity unavailable")
	}
	f.kills = append(f.kills, p)
	if !f.ignoreKill {
		f.exitAfter = f.age + 150*time.Millisecond
	}
	return nil
}
func TestReceiverBoundedClosedTransportCleanup(t *testing.T) {
	for _, stuck := range []bool{false, true} {
		f := &simulatedReceiver{now: time.Unix(1000, 0), initial: []receiverProcess{{42, "boot123"}}, exitAfter: time.Hour, allowCleanup: true, ignoreKill: stuck}
		e := stopReceiver(f)
		if (e != nil) != stuck || len(f.term) != 1 || len(f.kills) != 1 || f.age < 8250*time.Millisecond || f.age > 11*time.Second {
			t.Fatalf("cleanup did not enforce confirmed bounded exit: %v %+v", e, f)
		}
	}
}
func TestReceiverDelayedExitAndIdempotence(t *testing.T) {
	for _, d := range []time.Duration{0, 300 * time.Millisecond, 6 * time.Second} {
		f := &simulatedReceiver{now: time.Unix(1000, 0), initial: []receiverProcess{{42, "boot123"}}, exitAfter: d}
		if e := stopReceiver(f); e != nil {
			t.Fatal(e)
		}
		if f.age < d+100*time.Millisecond || f.age >= 8*time.Second {
			t.Fatal("did not wait for confirmed exit")
		}
		if (len(f.term) == 1) != (d > 0) {
			t.Fatal("non-idempotent signal")
		}
	}
}
func TestReceiverFailureNeverReportsStopped(t *testing.T) {
	for _, name := range []string{"timeout", "scan", "pidfd", "duplicate", "reuse"} {
		t.Run(name, func(t *testing.T) {
			f := &simulatedReceiver{now: time.Unix(1000, 0), initial: []receiverProcess{{42, "boot123"}}, exitAfter: time.Hour}
			switch name {
			case "scan":
				f.failScan = true
			case "pidfd":
				f.failTerm = true
			case "duplicate":
				f.initial = append(f.initial, receiverProcess{43, "boot456"})
			case "reuse":
				f.replace = true
			}
			if stopReceiver(f) == nil {
				t.Fatal("failed stop accepted")
			}
			if len(f.term) > 1 {
				t.Fatal("signaled a new/unrelated receiver")
			}
			if (name == "scan" || name == "duplicate" || name == "pidfd") && len(f.term) != 0 {
				t.Fatal("signal without validated identity")
			}
			if f.age > 8*time.Second {
				t.Fatal("unbounded wait")
			}
		})
	}
}
