package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func singleSample(ms int64, secondary, audio int) media {
	m := sample("RecordRunning", 3, ms)
	m.Record.Data = ms
	m.Pipeline.Status.States[2] = secondary
	m.Pipeline.Status.States[8] = audio
	return m
}
func TestSquareFreeFrameReadiness(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		secondary, audio           int
		profile, amb, enable, want bool
	}{
		{"single", 0, 3, true, false, true, true}, {"off", 0, 3, true, false, false, false}, {"dual missing", 0, 3, false, false, true, false},
		{"single initialized", 1, 3, true, false, true, true},
		{"initialized disabled", 1, 3, true, false, false, false},
		{"unrecognized initialized", 1, 3, false, false, true, false},
		{"main2 starting", 2, 3, true, false, true, false}, {"main2 stopped", 6, 3, true, false, true, false}, {"audio starting", 0, 2, true, false, true, false}, {"ambisonic", 0, 2, true, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, c, _ := fixture()
			p.singleProfile = func() (bool, error) { return tc.profile, nil }
			p.ambisonic = func() bool { return tc.amb }
			p.step(tc.enable, singleSample(4000, tc.secondary, tc.audio))
			p.step(tc.enable, singleSample(4900, tc.secondary, tc.audio))
			if len(*c) != 0 {
				t.Fatal(*c)
			}
			p.step(tc.enable, singleSample(5100, tc.secondary, tc.audio))
			if (len(*c) > 0) != tc.want {
				t.Fatal(*c)
			}
			p.step(tc.enable, sample("RecordIdle", 1, 0))
			if tc.want && !reflect.DeepEqual(*c, []bool{false, true}) {
				t.Fatal(*c)
			}
		})
	}
}
func TestSingleMustAdvance(t *testing.T) {
	for _, data := range [][]int64{{0, 0}, {5000, 5000}, {2000, 3000}, {11000, 12000}} {
		p, c, _ := fixture()
		p.singleProfile = func() (bool, error) { return true, nil }
		for i, ms := range []int64{6000, 7000} {
			m := singleSample(ms, 0, 3)
			m.Record.Data = data[i]
			p.step(true, m)
		}
		if len(*c) != 0 {
			t.Errorf("data %v: %v", data, *c)
		}
	}
}
func TestSingleProbeAcrossTakes(t *testing.T) {
	p, c, _ := fixture()
	n := 0
	p.singleProfile = func() (bool, error) {
		n++
		switch n {
		case 1:
			return false, errors.New("timeout")
		case 2:
			return false, nil
		}
		return true, nil
	}
	for _, ms := range []int64{4000, 5000, 6000, 10000, 11000, 12000} {
		p.step(true, singleSample(ms, 0, 3))
	}
	if n != 2 || len(*c) != 0 {
		t.Fatal(n, *c)
	}
	p.step(true, sample("RecordIdle", 3, 0))
	p.step(true, singleSample(4000, 0, 3))
	p.step(true, singleSample(5000, 0, 3))
	if n != 3 || len(*c) != 1 {
		t.Fatal(n, *c)
	}
}
func TestRecordingFreeFrame(t *testing.T) {
	original := runCommand
	defer func() { runCommand = original }()
	for _, tc := range []struct {
		mode, lens string
		want       bool
	}{
		{"kFreeFrameVideo", "kSATAngle", true}, {"kNormalVideo", "kSATAngle", false},
		{"kNormalVideo", "kPanorama", false}, {"kFreeFrameVideo", "kPanorama", false}, {"", "", false},
	} {
		runCommand = func(n string, args ...string) ([]byte, error) {
			if len(args) != 1 || !strings.Contains(args[0], "kMediaMode") {
				t.Fatal(n, args)
			}
			return []byte(`{"ret":"ok","data":{"shoot_mode":"` + tc.mode + `","lens_mode":"` + tc.lens + `"}}`), nil
		}
		v, _ := recordingFreeFrame()
		if v != tc.want {
			t.Fatal(tc, v)
		}
	}
}
func TestSlowProbeRequiresFreshRecordingSample(t *testing.T) {
	p, c, _ := fixture()
	p.singleProfile = func() (bool, error) { return true, nil }
	// First eligible single-lens observation comes late, with progress already known.
	p.step(true, singleSample(7000, 2, 3))
	p.step(true, singleSample(8000, 0, 3))
	p.step(true, sample("RecordIdle", 3, 0))
	if len(*c) != 0 {
		t.Fatal("command acted on stale state", *c)
	}
}

func TestProxyTransitionAndOutstandingStop(t *testing.T) {
	for _, disabled := range []byte{0, 1} {
		p, c, _ := fixture()
		p.singleProfile = func() (bool, error) { return true, nil }
		p.proxyPreference = func() (byte, error) { return disabled, nil }
		attempts := 0
		p.call = func(start bool) error {
			if !start {
				attempts++
				return errors.New("RPC timeout")
			}
			return nil
		}
		for i, sub := range []int{2, 4, 4, 6, 4} {
			m := singleSample(6000+int64(i)*1000, 0, 3)
			m.Pipeline.Status.States[3] = sub
			p.step(true, m)
		}
		if attempts != 0 {
			t.Fatal("premature request")
		}
		for i := 0; i < 4; i++ {
			m := singleSample(15000+int64(i)*1000, 0, 3)
			if disabled == 1 {
				m.Pipeline.Status.States[3] = 1
			}
			p.step(true, m)
		}
		if attempts != 1 || len(*c) != 0 {
			t.Fatalf("duplicate stop after timeout: %d", attempts)
		}
	}
}
