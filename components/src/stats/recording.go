package main

import (
	"math"
	"strconv"
	"strings"
)

// Width and height refer to the main encoded surface (not the sensor readout).
type geometry struct{ Width, Height int }
type recordingProfile struct {
	geometry
	Eyes, FPS int
}
type measuredEvent struct {
	event
	geometry
}
type routing struct {
	profile  recordingProfile
	waiting  []measuredEvent
	overflow bool
}

func (r *routing) accept(m *meter, e measuredEvent) {
	if r.profile.Eyes == 0 {
		if len(r.waiting) < 4096 {
			r.waiting = append(r.waiting, e)
		} else {
			r.overflow = true
		}
		return
	}
	if e.geometry == r.profile.geometry {
		m.add(e.event)
	}
}
func (r *routing) bind(m *meter, p recordingProfile) {
	r.profile = p
	m.Eyes, m.FPS = p.Eyes, p.FPS
	if r.overflow {
		m.Lost = true
		m.LossReason = "profile_wait_overflow"
	}
	for _, e := range r.waiting {
		r.accept(m, e)
	}
	r.waiting = nil
}
func pipelineProfile(info, lens, mode string) (recordingProfile, bool) {
	var p recordingProfile
	if !strings.Contains(info, "Error code: Success(0)") {
		return p, false
	}
	switch lens {
	case "kSATAngle":
		if mode != "kFreeFrameVideo" && mode != "kNormalVideo" {
			return p, false
		}
		p.Eyes = 1
	case "kPanorama":
		if mode != "kNormalVideo" {
			return p, false
		}
		p.Eyes = 2
	default:
		return p, false
	}
	for _, line := range strings.Split(info, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.Atoi(f[1])
		switch f[0] {
		case "width:":
			p.Width = v
		case "height:":
			p.Height = v
		case "fps:":
			p.FPS = v
		}
	}
	return p, p.Width >= 1280 && p.Height >= 720 && p.FPS >= 12 && p.FPS <= 240
}
func framePeriod(med float64, fps int) float64 {
	if fps == 0 { // Historical replay fixtures contain no main-stream description.
		for _, p := range []float64{20000000, 1001000000.0 / 30} {
			if math.Abs(med-p) < p*.12 {
				return p
			}
		}
		return 0
	}
	p := 1e9 / float64(fps)
	ntsc := p * 1.001
	if math.Abs(ntsc-med) < math.Abs(p-med) {
		p = ntsc
	}
	// A cadence already missing every second frame must not become the baseline.
	if math.Abs(med-p) > p*.12 {
		return 0
	}
	return p
}
