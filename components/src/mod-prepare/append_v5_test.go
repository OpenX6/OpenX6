package main

import "testing"

func TestMultipleAppendedRanges(t *testing.T) {
	b := []byte("base")
	want := []byte("baseAA\x00\x00BB")
	v := Variant{BaseSHA: sum(b), SHA: sum(want), Size: len(want), Edits: []Edit{{4, sum(nil), []byte("AA")}, {8, sum(nil), []byte("BB")}}}
	out, e := reconstruct(b, v)
	if e != nil || string(out) != string(want) {
		t.Fatalf("appended %x %v", out, e)
	}
	for _, kind := range []string{"negative", "overlap", "beyond", "before", "target"} {
		x := v
		x.Edits = append([]Edit(nil), v.Edits...)
		switch kind {
		case "negative":
			x.Edits[0].Offset = -1
		case "overlap":
			x.Edits[1].Offset = 5
		case "beyond":
			x.Edits[1].Offset = 9
		case "before":
			x.Edits[1].Before = sum([]byte("x"))
		case "target":
			x.SHA = sum(b)
		}
		if _, e := reconstruct(b, x); e == nil {
			t.Fatal("accepted", kind)
		}
	}
}
