package main

import (
	"errors"
	"testing"
)

func TestAmbisonicReadiness(t *testing.T) {
	for _, c := range []struct {
		name    string
		mode    bool
		audio   int
		advance bool
		lag     int64
		want    bool
	}{
		{"ambisonic advancing", true, 2, true, 0, true}, {"standard starting", false, 2, true, 0, false}, {"stalled data", true, 2, false, 0, false}, {"null audio", true, 0, true, 0, false}, {"init audio", true, 1, true, 0, false}, {"stopping audio", true, 4, true, 0, false}, {"data behind", true, 2, true, 3000, false}, {"data future", true, 2, true, -3000, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, cs, _ := fixture()
			p.ambisonic = func() bool { return c.mode }
			for _, ms := range []int64{6000, 7000} {
				m := sample("RecordRunning", 3, ms)
				m.Pipeline.Status.States[8] = c.audio
				m.Record.Data = ms - c.lag
				if !c.advance {
					m.Record.Data = 0
				}
				p.step(true, m)
			}
			if (len(*cs) > 0) != c.want {
				t.Fatal(*cs)
			}
		})
	}
}
func TestAmbisonicResetAndRestore(t *testing.T) {
	p, cs, _ := fixture()
	p.ambisonic = func() bool { return true }
	for _, ms := range []int64{6000, 7000} {
		m := sample("RecordRunning", 3, ms)
		m.Record.Data = ms
		m.Pipeline.Status.States[8] = 2
		p.step(true, m)
	}
	if len(*cs) != 1 {
		t.Fatal(*cs)
	}
	p.step(true, sample("RecordIdle", 1, 0))
	if len(*cs) != 2 || !(*cs)[1] {
		t.Fatal(*cs)
	}
	m := sample("RecordRunning", 3, 6000)
	m.Record.Data = 6000
	m.Pipeline.Status.States[8] = 2
	p.step(true, m)
	if len(*cs) != 2 {
		t.Fatal("stale progress")
	}
}
func TestAmbisonicConfigValidation(t *testing.T) {
	old := runCommand
	defer func() { runCommand = old }()
	for _, c := range []struct {
		s    string
		err  bool
		want bool
	}{{`{"ret":"ok","data":{"algo_mode":"kAmbisonic"}}`, false, true}, {`{"ret":"ok","data":{"algo_mode":"kWindNoiseReduction"}}`, false, false}, {`{"ret":"error","data":{"algo_mode":"kAmbisonic"}}`, false, false}, {`{}`, false, false}, {`broken`, false, false}, {`{"ret":"ok","data":{"algo_mode":"kAmbisonic"}}`, true, false}} {
		runCommand = func(n string, a ...string) ([]byte, error) {
			if n != "/usr/bin/ins_bll_cli" || len(a) != 1 {
				t.Fatal(n, a)
			}
			if c.err {
				return nil, errors.New("timeout")
			}
			return []byte(c.s), nil
		}
		if isAmbisonic() != c.want {
			t.Fatal(c)
		}
	}
}
