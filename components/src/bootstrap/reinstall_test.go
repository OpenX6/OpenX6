package main

import (
	"errors"
	"os"
	"testing"
)

func TestAdoptExactExistingModifiedFeatures(t *testing.T) {
	for _, point := range []string{"", "adopt-settings-preserved", "adopt-before-publish", "adopt-published"} {
		t.Run(point, func(t *testing.T) {
			f := fixture(t)
			if err := f.e.Run(); err != nil {
				t.Fatal(err)
			}
			os.Remove(f.e.path(featurePath + "/complete.json"))
			put(t, f.e.path(featurePath+"/raw-nr-v1/level"), []byte{3}, 0600)
			put(t, f.e.path(featurePath+"/unrelated-existing-feature"), []byte("keep"), 0600)
			if point != "" {
				f.e.Fault = func(p string) error {
					if p == point {
						return errors.New("interrupted adoption")
					}
					return nil
				}
				if f.e.Run() == nil {
					t.Fatal("fault missing")
				}
				f.e.Fault = nil
			}
			if err := f.e.Run(); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(f.e.path(featurePath + "/raw-nr-v1/level"))
			if len(b) != 1 || b[0] != 3 {
				t.Fatal("selection reset")
			}
			b, _ = os.ReadFile(f.e.path(featurePath + "/unrelated-existing-feature"))
			if string(b) != "keep" {
				t.Fatal("unrelated data removed")
			}
			untouched(t, f.e)
		})
	}
}
func TestAdoptionRejectsPartialOrUnknownPayload(t *testing.T) {
	for _, kind := range []string{"missing-payload", "different-payload", "missing-preference", "bad-preference", "foreign-receipt"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			if err := f.e.Run(); err != nil {
				t.Fatal(err)
			}
			os.Remove(f.e.path(featurePath + "/complete.json"))
			switch kind {
			case "missing-payload":
				os.Remove(f.e.path(featurePath + "/vr180-v1/gui"))
			case "different-payload":
				put(t, f.e.path(featurePath+"/vr180-v1/gui"), []byte("unknown"), 0755)
			case "missing-preference":
				os.Remove(f.e.path(featurePath + "/raw-nr-v1/level"))
			case "bad-preference":
				put(t, f.e.path(featurePath+"/raw-nr-v1/level"), []byte{255}, 0600)
			case "foreign-receipt":
				put(t, f.e.path(featurePath+"/complete.json"), []byte("foreign"), 0600)
			}
			if f.e.Run() == nil {
				t.Fatal("accepted unknown state")
			}
			untouched(t, f.e)
		})
	}
}
