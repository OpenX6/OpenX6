package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSyntheticUnknownRetains36(t *testing.T) {
	b, e := os.ReadFile("testdata/synthetic-unknown.json")
	if e != nil {
		t.Fatal(e)
	}
	var d struct {
		Streams map[uint64]*stream `json:"streams"`
	}
	if e = json.Unmarshal(b, &d); e != nil {
		t.Fatal(e)
	}
	m := fresh()
	m.Streams = d.Streams
	var now float64
	for _, s := range d.Streams {
		if s.Last > now {
			now = s.Last
		}
		if !s.Bad || s.Gaps != 36 {
			t.Fatal(s.Bad, s.Gaps)
		}
	}
	text, sev := m.display(now, 65)
	if !strings.Contains(text, "Gaps 36/36 ?\n") || sev != 2 {
		t.Fatal(text, sev)
	}
	m.Lost = true
	text, _ = m.display(now, 65)
	if !strings.Contains(text, "Gaps 36/36 ?\n") {
		t.Fatal(text)
	}
	for _, s := range m.Streams {
		if !s.Bad || s.Gaps != 36 {
			t.Fatal("display modified validity/counts")
		}
	}
}
func TestNoObservationsNotZero(t *testing.T) {
	m := fresh()
	text, sev := m.display(1, 60)
	if !strings.Contains(text, "Gaps --/-- ?\n") || sev != 3 {
		t.Fatal(text, sev)
	}
}
