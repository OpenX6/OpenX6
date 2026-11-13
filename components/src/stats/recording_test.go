package main

import (
	"strings"
	"testing"
)

func TestSingleMainRouting(t *testing.T) {
	r := routing{}
	m := fresh()
	for i := 0; i < 200; i++ {
		if i == 35 {
			continue
		}
		ts := 10 + float64(i)/120
		for _, g := range []geometry{{1080, 1080}, {3840, 3008}} {
			e := event{T: ts, Kind: "output_response", SID: uint64(g.Width), PTS: uint64(ts * 1e9), Bytes: 250000}
			r.accept(m, measuredEvent{e, g})
		}
		if i == 90 {
			r.bind(m, recordingProfile{geometry{3840, 3008}, 1, 120})
		}
	}
	s, _ := m.display(11.65, 45)
	if len(m.Streams) != 1 || m.Streams[3840].Gaps != 1 || !strings.Contains(s, "Gaps 1\n") {
		t.Fatal(s)
	}
	r.accept(m, measuredEvent{event{SID: 99, PTS: 10}, geometry{3840, 3008}})
	if s, _ := m.display(11.65, 45); !strings.Contains(s, "Gaps -- ?") {
		t.Fatal(s)
	}
}
func TestActualPeriod(t *testing.T) {
	for _, fps := range []int{24, 25, 30, 50, 60, 90, 100, 120} {
		p := 1.001e9 / float64(fps)
		if diff := framePeriod(p, fps) - p; diff > 0.001 || diff < -0.001 {
			t.Fatal(fps)
		}
		if framePeriod(p*2, fps) != 0 {
			t.Fatal(fps)
		}
	}
}
func TestThermalReadings(t *testing.T) {
	h := parseCameraHeat([]byte(`{"ret":"ok","data":{"battery_is_present":false,"battery_temperature_c":25,"dev_temperature":{"main_sensor_temperature_c":47,"secondary_camera_sensor_temp_c":0}}}`))
	if h != (cameraHeat{"47", "--", "--"}) {
		t.Fatal(h)
	}
	if got := heatText("Gaps 2\nSoC 45 C", true, h); got != "Gaps 2\nSoC 45 C\nS1 47  S2 --  Batt -- C" {
		t.Fatal(got)
	}
	if heatText("Stats unavailable", true, h) != "Stats unavailable" {
		t.Fatal("invalid readout changed")
	}
}

func TestBatchPicturesShareBuffer(t *testing.T) {
	r := routing{}
	m := fresh()
	r.bind(m, recordingProfile{geometry{3840, 3008}, 1, 120})
	for _, line := range []string{
		" 1.000000: x6stats_batch_frame: width=3840 height=3008 sid=11 index=7 pts=100 addr=0x40",
		" 1.000100: x6stats_batch_frame: width=3840 height=3008 sid=11 index=7 pts=200 addr=0x40",
		" 1.012000: x6stats_input_response: width=3840 height=3008 sid=11 index=7 pts=100 addr=0x40",
		" 1.018100: x6stats_input_response: width=3840 height=3008 sid=11 index=7 pts=200 addr=0x40",
	} {
		ev, ok := parseMeasurement(line)
		if !ok {
			t.Fatal(line)
		}
		r.accept(m, ev)
	}
	s := m.Streams[11]
	if s.Returns != 2 || len(m.Pending) != 0 || s.InputFaults != 0 || s.SumLatency < 29.99 || s.SumLatency > 30.01 {
		t.Fatal(s)
	}
}
