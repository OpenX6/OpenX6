package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

type disappearingExe struct {
	simulatedReceiver
	mode string
}

func (f *disappearingExe) Snapshot() ([]receiverProcess, error) {
	f.scans++
	p := receiverProcess{42, "boot123"}
	if f.mode == "initial" || f.scans > 1 && (f.mode != "exits" || f.age < 350*time.Millisecond) {
		if f.mode == "wrong-exe" {
			return nil, fmt.Errorf("unexpected receiver executable")
		}
		if f.mode == "reused-pid" {
			p.Start = "boot456"
		}
		return nil, &receiverExitPending{p, "R", os.ErrNotExist}
	}
	if f.scans == 1 {
		return []receiverProcess{p}, nil
	}
	return nil, nil
}

func TestReceiverUnresolvedExeIsNeverExitAcknowledgement(t *testing.T) {
	for _, name := range []string{"initial", "exits", "stuck", "reused-pid", "wrong-exe"} {
		t.Run(name, func(t *testing.T) {
			f := &disappearingExe{simulatedReceiver: simulatedReceiver{now: time.Unix(1000, 0)}, mode: name}
			e := stopReceiver(f)
			if (e == nil) != (name == "exits") {
				t.Fatalf("unexpected acceptance/rejection: %v", e)
			}
			if name == "initial" && len(f.term) != 0 {
				t.Fatal("signalled without initial executable identity")
			}
			if name != "initial" && len(f.term) != 1 {
				t.Fatal("expected precisely one identity-checked TERM")
			}
			if name == "exits" && f.age < 450*time.Millisecond {
				t.Fatal("reported stopped before disappearance and quiet interval")
			}
			if name == "stuck" && f.age != 8*time.Second {
				t.Fatal("unresolved exit did not reach bounded refusal")
			}
			if (name == "reused-pid" || name == "wrong-exe") && f.age != 0 {
				t.Fatal("unexpected identity incorrectly treated as an exit transition")
			}
		})
	}
}
