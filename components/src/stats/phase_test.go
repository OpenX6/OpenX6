package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestSyntheticPaired13344usStep(t *testing.T) {
	b, e := os.ReadFile("testdata/synthetic-phase.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows map[string]struct {
		Before []uint64 `json:"before"`
		After  []uint64 `json:"after"`
	}
	if e = json.Unmarshal(b, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 2 {
		t.Fatal("expected two synthetic streams")
	}
	m := fresh()
	evs := []event{}
	sid := uint64(1)
	for _, r := range rows {
		pts := []uint64{}
		for j := 32; j > 0; j-- {
			pts = append(pts, r.Before[0]-uint64(j)*20000000)
		}
		pts = append(pts, r.Before...)
		pts = append(pts, r.After...)
		for i, p := range pts {
			evs = append(evs, event{10 + float64(i)*.02, "output_response", sid, 0, p, sid * 100, 350000})
		}
		sid++
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].T < evs[j].T })
	for _, e := range evs {
		m.add(e)
	}
	text, _ := m.display(evs[len(evs)-1].T, 60)
	if !strings.Contains(text, "Gaps 0/0") {
		t.Fatal(text)
	}
	for _, s := range m.Streams {
		if s.PhaseCorrections != 1 {
			t.Fatal("synthetic correction missing")
		}
	}
}
