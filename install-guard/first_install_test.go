package main

import (
	"strings"
	"testing"
)

func TestFactoryFirstInstall(t *testing.T) {
	const updater825 = "ba6001f68643c5c2bf4d9c86c72b3925e71583c9a123b112186b05b9e4a7688e"
	const updater827 = "d2ef51479725950986b9ad45a82ed2a7c60a8accafeef58a46f795b8f770531b"
	for _, slot := range []string{"_a", "_b"} {
		for _, tc := range []struct {
			version, updater string
			accepted         bool
		}{
			{"0.8.825", updater825, true}, {"0.8.827", updater827, true},
			{"0.8.827", updater825, false}, {"0.8.825", updater827, false},
			{"0.8.827", strings.Repeat("a", 64), false},
			{"0.8.828", updater827, false}, {"0.8.833", updater827, false},
			{"1.2.36", updater827, false}, {"1.3.59", updater827, false},
		} {
			t.Run(slot+"/"+tc.version+"/"+tc.updater[:8], func(t *testing.T) {
				c, e, s := fixture(slot)
				e.PublicSHA256 = ""
				e.Source = SourceEvidence{Version: tc.version, OTASHA256: tc.updater, Preserved: map[string]string{}}
				s.SourceID = sourceID(e)
				for _, phase := range []string{"inspect", "pre", "post"} {
					err := validate(c, e, phase, s)
					if tc.accepted && err != nil {
						t.Fatalf("%s: %v", phase, err)
					}
					if !tc.accepted && (err == nil || err.Error() != "unsupported source build") {
						t.Fatalf("%s: expected unsupported source build, got %v", phase, err)
					}
				}
				if tc.accepted {
					e.Sizes["system_a"]--
					if err := validate(c, e, "pre", nil); err == nil {
						t.Fatal("unexpected partition size accepted")
					}
				}
			})
		}
	}
}
