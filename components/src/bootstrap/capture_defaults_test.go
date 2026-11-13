package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFreshCaptureDefaultsAndSavedOff(t *testing.T) {
	f := fixture(t)
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"data/x6-wifi-idle/minutes":                   "\x02",
		featurePath + "/vr180-v1/sync":                "\x01",
		featurePath + "/mod-controls-v1/stereo.pref":  "\x01",
		featurePath + "/stats-v1/enabled.pref":        "\x01",
		featurePath + "/interval-controls-v1/tl.pref": "\x01",
		featurePath + "/proxy-v1/disabled":            "\x01",
		featurePath + "/flat-mirror-v1/enabled":       "",
	}
	check := func() {
		t.Helper()
		for rel, value := range want {
			got, err := f.e.read(rel, 1, 0600)
			if err != nil || string(got) != value {
				t.Fatalf("%s = %v, %v", rel, got, err)
			}
		}
	}
	check()
	mirror := featurePath + "/flat-mirror-v1/enabled"
	if err := os.Remove(f.e.path(mirror)); err != nil {
		t.Fatal(err)
	}
	delete(want, mirror)
	for rel := range want {
		put(t, f.e.path(rel), []byte{0}, 0600)
		want[rel] = "\x00"
	}
	if err := f.e.Run(); err != nil {
		t.Fatal(err)
	}
	check()
	if _, err := os.Lstat(f.e.path(mirror)); !os.IsNotExist(err) {
		t.Fatal("disabled mirror recreated", err)
	}
}
func TestFirstbootPreferenceMergeRejectsSymlink(t *testing.T) {
	f := fixture(t)
	rel := featurePath + "/stats-v1/enabled.pref"
	dest := f.e.path(rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.e.path("data/stock-calibration"), dest); err != nil {
		t.Fatal(err)
	}
	if f.e.Run() == nil {
		t.Fatal("unsafe preference accepted")
	}
	untouched(t, f.e)
}
