package main

import (
	"math"
	"sort"
	"strings"
	"testing"
)

func fixture(m *meter, count uint64, dt, lat float64, skip int) {
	var evs []event
	for i := uint64(0); i < count; i++ {
		if int(i) == skip {
			continue
		}
		for s := uint64(1); s < 3; s++ {
			t := 10 + float64(i)*dt
			pts := 1000000000 + uint64(float64(i)*dt*1e9)
			evs = append(evs, event{t, "hfi", s, i % 16, pts, s * 100, 0}, event{t + lat, "input_response", s, i % 16, pts, s * 100, 0}, event{t + lat + .001, "output_response", s, i % 16, pts, s * 100, 350000})
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].T < evs[j].T })
	for _, e := range evs {
		m.add(e)
	}
}
func TestBasic(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, -1)
	text, sev := m.display(12, 72)
	if sev != 0 || !strings.Contains(text, "Return 25/25 ms") || !strings.Contains(text, "Gaps 0/0") {
		t.Fatal(text, sev)
	}
}
func TestGapAndSticky(t *testing.T) {
	m := fresh()
	fixture(m, 400, .02, .025, 50)
	text, sev := m.display(18, 72)
	if sev != 2 || !strings.Contains(text, "Gaps 1/1") {
		t.Fatal(text, sev)
	}
}
func TestUnknown(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, -1)
	m.Lost = true
	text, sev := m.display(12, 72)
	if sev != 3 || !strings.Contains(text, " ?\nSoC") {
		t.Fatal(text, sev)
	}
}
func TestThirty(t *testing.T) {
	m := fresh()
	fixture(m, 100, 1.001/30, .025, -1)
	for _, s := range m.Streams {
		if s.Bad || s.Gaps != 0 || math.Abs(s.Period-1001000000.0/30) > 1 {
			t.Fatal(s)
		}
	}
}
func TestStale(t *testing.T) {
	m := fresh()
	fixture(m, 100, .02, .025, -1)
	text, sev := m.display(20, 72)
	if sev != 3 || !strings.Contains(text, "Return --/--") {
		t.Fatal(text, sev)
	}
}
func TestWrongPair(t *testing.T) {
	m := fresh()
	m.add(event{1, "hfi", 1, 0, 100, 8, 0})
	m.add(event{1.1, "input_response", 1, 0, 100, 9, 0})
	if m.Streams[1].InputFaults != 1 || m.Lost {
		t.Fatal("input/output validity not separated")
	}
}

func TestDrainQueuedWindow(t *testing.T) {
	ch := make(chan event, 600)
	for i := 0; i < 300; i++ {
		ch <- event{T: float64(i)}
	}
	var n int
	drain(ch, func(e event) { n++ })
	if n != 300 || len(ch) != 0 {
		t.Fatal(n, len(ch))
	}
	drain(ch, func(e event) { t.Fatal("empty queue produced event") })
}
