package main

import (
	"strings"
	"testing"
)

func TestInputFaultDoesNotPoisonOutput(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, -1)
	m.add(event{12, "input_response", 1, 9, 99999999, 999, 0})
	s, _ := m.display(12, 60)
	if !strings.Contains(s, "Gaps 0/0") || !strings.Contains(s, "Return --/25") {
		t.Fatal(s)
	}
	if m.Streams[1].FirstFault != "input_unpaired" {
		t.Fatal("reason missing")
	}
}
func TestActualGapSurvivesInputFault(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, 50)
	m.add(event{12, "input_response", 1, 9, 99999999, 999, 0})
	s, _ := m.display(12, 60)
	if !strings.Contains(s, "Gaps 1/1") {
		t.Fatal(s)
	}
}
func TestOutputBadStaysUnknown(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, -1)
	m.add(event{12, "output_response", 1, 9, 100, 999, 120})
	s, _ := m.display(12, 60)
	if !strings.Contains(s, " ?\nSoC") {
		t.Fatal(s)
	}
}
func TestLossCounterIsolation(t *testing.T) {
	base := map[string]uint64{"input:hfi": 8, "output:fbd": 3, "trace:cpu": 4}
	now := map[string]uint64{"input:hfi": 8, "output:fbd": 3, "trace:cpu": 4}
	o, i, _ := lossSince(base, now)
	if o || i {
		t.Fatal("historical counters poisoned new recording")
	}
	now["input:hfi"]++
	o, i, _ = lossSince(base, now)
	if o || !i {
		t.Fatal(o, i)
	}
	now["output:fbd"]++
	o, _, _ = lossSince(base, now)
	if !o {
		t.Fatal("output loss hidden")
	}
	now["trace:cpu"] = 3
	o, _, _ = lossSince(base, now)
	if !o {
		t.Fatal("counter reset accepted")
	}
}
func TestNewMeterHasNoPriorLoss(t *testing.T) {
	m := fresh()
	m.Lost = true
	m = fresh()
	fixture(m, 100, .02, .025, -1)
	s, _ := m.display(12, 60)
	if !strings.Contains(s, "Gaps 0/0") {
		t.Fatal(s)
	}
}

func phaseFixture(m *meter, short1, short2 uint64, withGap bool) {
	fixture(m, 100, .02, .025, -1)
	for i := 0; i < 40; i++ {
		for sid := uint64(1); sid <= 2; sid++ {
			d := uint64(20000000)
			if i == 0 {
				d = short1
				if sid == 2 {
					d = short2
				}
			}
			if i == 20 && withGap {
				d = 40000000
			}
			s := m.Streams[sid]
			m.add(event{12 + float64(i)*.02, "output_response", sid, 0, s.LastPTS + d, sid * 100, 350000})
		}
	}
}
func TestPairedPhaseCorrection(t *testing.T) {
	m := fresh()
	phaseFixture(m, 13344000, 13344000, false)
	s, _ := m.display(12.78, 60)
	if !strings.Contains(s, "Gaps 0/0") {
		t.Fatal(s)
	}
	for _, x := range m.Streams {
		if x.PhaseCorrections != 1 || x.Bad {
			t.Fatal(x)
		}
	}
}
func TestPhaseDoesNotHideRealGap(t *testing.T) {
	m := fresh()
	phaseFixture(m, 13344000, 13344000, true)
	s, _ := m.display(12.78, 60)
	if !strings.Contains(s, "Gaps 1/1") {
		t.Fatal(s)
	}
}
func TestOneEyeOrDifferentPhaseUnknown(t *testing.T) {
	for _, v := range []uint64{20000000, 14000000} {
		m := fresh()
		phaseFixture(m, 13344000, v, false)
		s, _ := m.display(12.78, 60)
		if !strings.Contains(s, " ?\nSoC") {
			t.Fatal(s)
		}
	}
}
func TestLongOrLossyPhaseUnknown(t *testing.T) {
	m := fresh()
	phaseFixture(m, 33344000, 33344000, false)
	s, _ := m.display(12.78, 60)
	if !strings.Contains(s, " ?\nSoC") {
		t.Fatal(s)
	}
	m = fresh()
	phaseFixture(m, 13344000, 13344000, false)
	m.Lost = true
	s, _ = m.display(12.78, 60)
	if !strings.Contains(s, " ?\nSoC") {
		t.Fatal(s)
	}
}

func TestPairedPhaseNeedsSettledFollowup(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, -1)
	for sid := uint64(1); sid <= 2; sid++ {
		s := m.Streams[sid]
		m.add(event{12, "output_response", sid, 0, s.LastPTS + 13344000, sid * 100, 350000})
	}
	s, _ := m.display(12, 60)
	if !strings.Contains(s, " ?\nSoC") {
		t.Fatal("accepted before confirmation", s)
	}
}
