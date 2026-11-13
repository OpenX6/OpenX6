package main

import "testing"

func TestExact827FirstInstall(t *testing.T) {
	const hash = "d2ef51479725950986b9ad45a82ed2a7c60a8accafeef58a46f795b8f770531b"
	for _, slot := range []string{"_a", "_b"} {
		c, e, snapshot := fixture(slot)
		e.PublicSHA256 = ""
		e.Source = SourceEvidence{Version: "0.8.827", OTASHA256: hash, Preserved: map[string]string{}}
		snapshot.SourceID = sourceID(e)
		for _, phase := range []string{"inspect", "pre", "post"} {
			if err := validate(c, e, phase, snapshot); err != nil {
				t.Fatalf("%s %s: %v", slot, phase, err)
			}
		}
		for _, pair := range [][2]string{{"0.8.827", c.Sources[0].OTASHA256}, {"0.8.825", hash}, {"0.8.828", hash}, {"1.3.59", hash}} {
			e.Source.Version, e.Source.OTASHA256 = pair[0], pair[1]
			if err := validate(c, e, "pre", nil); err == nil || err.Error() != "unsupported source build" {
				t.Fatalf("wrong pair %s: %v", pair, err)
			}
		}
	}
}
