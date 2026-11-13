package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type event struct {
	T                            float64
	Kind                         string
	SID, Index, PTS, Addr, Bytes uint64
}

var lineRE = regexp.MustCompile(`\s(\d+\.\d+): (?:x6stats_)?(hfi|batch_frame|input_response|output_response): (.*)$`)
var fieldRE = regexp.MustCompile(`(\w+)=(0x[0-9a-f]+|\d+)`)

func parseMeasurement(s string) (measuredEvent, bool) {
	m := lineRE.FindStringSubmatch(s)
	if m == nil {
		return measuredEvent{}, false
	}
	v := map[string]uint64{}
	for _, f := range fieldRE.FindAllStringSubmatch(m[3], -1) {
		n, e := strconv.ParseUint(f[2], 0, 64)
		if e != nil {
			return measuredEvent{}, false
		}
		v[f[1]] = n
	}
	for _, k := range []string{"sid", "index", "pts", "addr"} {
		if _, ok := v[k]; !ok {
			return measuredEvent{}, false
		}
	}
	t, _ := strconv.ParseFloat(m[1], 64)
	kind := m[2]
	if kind == "batch_frame" {
		kind = "hfi"
	}
	return measuredEvent{event{t, kind, v["sid"], v["index"], v["pts"], v["addr"], v["bytes"]}, geometry{int(v["width"]), int(v["height"])}}, true
}

func parse(s string) (event, bool) { e, ok := parseMeasurement(s); return e.event, ok }

type key struct{ SID, Index, PTS, Addr uint64 }
type point struct{ T, Value float64 }
type stream struct {
	PhaseCompensated bool

	PhasePending         bool
	PhasePTS, PhaseDelta uint64
	PhaseFollow          int
	PhaseCorrections     uint64

	RecentPTS, FirstAnomaly, AfterAnomaly []uint64
	FirstAnomalyTime                      float64

	First, Last                   float64
	LastPTS                       uint64
	Steps                         []uint64
	Period                        float64
	Gaps, Frames                  uint64
	Bad                           bool
	LastOutput, InputInvalidUntil float64
	InputFaults                   uint64
	FirstFault                    string
	Latency, Payload              []point
	SumLatency                    float64
	Returns                       uint64
}
type meter struct {
	Eyes, FPS  int
	Streams    map[uint64]*stream
	Pending    map[key]float64
	Lost       bool
	LossReason string
	First      float64
}

func fresh() *meter {
	return &meter{Eyes: 2, Streams: map[uint64]*stream{}, Pending: map[key]float64{}}
}
func (m *meter) add(e event) {
	if e.PTS == 0 {
		return
	}
	s := m.Streams[e.SID]
	if s == nil {
		if len(m.Streams) >= m.Eyes {
			m.LossReason = "ambiguous_main_streams"
			m.Lost = true
			return
		}
		s = &stream{First: e.T}
		m.Streams[e.SID] = s
		if m.First == 0 {
			m.First = e.T
		}
	}
	if e.T > s.Last {
		s.Last = e.T
	}
	k := key{e.SID, e.Index, e.PTS, e.Addr}
	switch e.Kind {
	case "hfi":
		if _, ok := m.Pending[k]; ok {
			s.inputFault(e.T, "duplicate_input")
		}
		if len(m.Pending) > 128 {
			s.inputFault(e.T, "input_capacity")
			clear(m.Pending)
		}
		m.Pending[k] = e.T
	case "input_response":
		if t, ok := m.Pending[k]; ok {
			ms := (e.T - t) * 1000
			if ms < 0 || ms > 10000 {
				s.inputFault(e.T, "input_latency_range")
			} else {
				s.Latency = append(s.Latency, point{e.T, ms})
				s.SumLatency += ms
				s.Returns++
			}
			delete(m.Pending, k)
		} else {
			s.inputFault(e.T, "input_unpaired")
		}
	case "output_response":
		wasBad := s.Bad
		if e.Bytes == 0 {
			return
		}
		s.LastOutput = e.T
		s.Payload = append(s.Payload, point{e.T, float64(e.Bytes)})
		s.Frames++
		if s.LastPTS != 0 {
			if e.PTS <= s.LastPTS {
				s.Bad = true
			} else {
				d := e.PTS - s.LastPTS
				if s.Period == 0 {
					if len(s.Steps) >= 16 {
						s.Steps = s.Steps[1:]
					}
					s.Steps = append(s.Steps, d)
					if len(s.Steps) >= 16 {
						ds := append([]uint64{}, s.Steps...)
						sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
						med := float64(ds[len(ds)/2])
						s.Period = framePeriod(med, m.FPS)
						if s.Period == 0 {
							s.Bad = true
						} else {
							for _, x := range s.Steps {
								s.step(x)
							}
						}
					}
				} else {
					if m.Eyes == 1 {
						s.step(d)
					} else {
						s.observeStep(d, e.PTS)
					}
				}
			}
		}

		if len(s.FirstAnomaly) > 0 && len(s.AfterAnomaly) < 16 {
			s.AfterAnomaly = append(s.AfterAnomaly, e.PTS)
		}
		if !wasBad && s.Bad {
			s.FirstAnomaly = append(append([]uint64{}, s.RecentPTS...), e.PTS)
			s.FirstAnomalyTime = e.T
		}
		s.RecentPTS = append(s.RecentPTS, e.PTS)
		if len(s.RecentPTS) > 16 {
			s.RecentPTS = s.RecentPTS[len(s.RecentPTS)-16:]
		}
		s.LastPTS = e.PTS
		m.resolvePhases()
	}
	s.Latency = trim(s.Latency, e.T-2)
	s.Payload = trim(s.Payload, e.T-2)
}
func (s *stream) step(d uint64) {
	n := math.Round(float64(d) / s.Period)
	if n < 1 || n > 500 || math.Abs(float64(d)-n*s.Period) > s.Period*.25 {
		s.Bad = true
		return
	}
	s.Gaps += uint64(n - 1)
}
func trim(a []point, t float64) []point {
	i := 0
	for i < len(a) && a[i].T < t {
		i++
	}
	if i > 0 {
		copy(a, a[i:])
		a = a[:len(a)-i]
	}
	return a
}
func mean(a []point, lo float64) (float64, int) {
	var sum float64
	n := 0
	for _, p := range a {
		if p.T >= lo {
			sum += p.Value
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}
func (m *meter) display(t, temp float64) (string, uint32) {
	ids := []uint64{}
	for id := range m.Streams {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	ts := socDegrees(temp)
	if len(ids) != m.Eyes || m.LossReason == "ambiguous_main_streams" {
		if m.Eyes == 1 {
			return "Return sampling\nRate -- Mb/s\nGaps -- ?\nSoC " + ts + " C", 3
		}
		return "Return sampling\nRate --/-- Mb/s\nGaps --/-- ?\nSoC " + ts + " C", 3
	}
	lat, rate, gaps := []string{}, []string{}, []string{}
	sev := uint32(0)
	unknown := m.Lost
	for _, id := range ids {
		s := m.Streams[id]
		v, n := mean(s.Latency, t-1)
		if n < 5 || t-s.Last > 2 || t < s.InputInvalidUntil {
			lat = append(lat, "--")
		} else {
			lat = append(lat, fmt.Sprintf("%.0f", v))
			if v >= 80 && t-m.First >= 5 && sev < 1 {
				sev = 1
			}
		}
		_, pn := mean(s.Payload, t-1)
		var bytes float64
		for _, p := range s.Payload {
			if p.T >= t-1 {
				bytes += p.Value
			}
		}
		if pn < 5 || t-s.LastOutput > 2 {
			rate = append(rate, "--")
			unknown = true
		} else {
			rate = append(rate, fmt.Sprintf("%.0f", bytes*8/1e6))
		}
		if s.Bad || s.PhasePending || s.Period == 0 {
			unknown = true
		}
		if s.Period == 0 {
			gaps = append(gaps, "--")
		} else {
			gaps = append(gaps, strconv.FormatUint(s.Gaps, 10))
		}
		if s.Gaps > 0 {
			sev = 2
		}
	}
	if unknown {
		if sev != 2 {
			sev = 3
		}
		// Preserve observed counts without claiming complete/precise coverage.
		gaps[len(gaps)-1] += " ?"
	}

	return fmt.Sprintf("Return %s ms\nRate %s Mb/s\nGaps %s\nSoC %s C", strings.Join(lat, "/"), strings.Join(rate, "/"), strings.Join(gaps, "/"), ts), sev
}

// Drain bounded queued measurements after blocking media RPC, before taking a current-time window.
func drain[T any](ch <-chan T, consume func(T)) {
	for i := 0; i < 8192; i++ {
		select {
		case e := <-ch:
			consume(e)
		default:
			return
		}
	}
}

func (s *stream) inputFault(t float64, reason string) {
	s.InputFaults++
	if t+1 > s.InputInvalidUntil {
		s.InputInvalidUntil = t + 1
	}
	if s.FirstFault == "" {
		s.FirstFault = reason
	}
}

// A single sub-period, common-mode timestamp phase adjustment is not a missing frame.
// Accept only paired eyes with the same timestamp and delta, followed by eight
// normal intervals. All one-sided, long, backwards or lossy cases stay unknown.
func (s *stream) observeStep(d, pts uint64) {
	if s.PhasePending {
		if s.PhaseFollow == 0 && math.Abs(float64(d+s.PhaseDelta)-2*s.Period) <= 2000 {
			s.PhaseCompensated = true
			s.PhaseFollow = 1
			return
		}
		if math.Abs(float64(d)-s.Period) > s.Period*.1 {
			s.Bad = true
			s.PhasePending = false
		} else {
			s.PhaseFollow++
			if s.PhaseFollow > 50 {
				s.Bad = true
				s.PhasePending = false
			}
		}
		s.step(d)
		return
	}
	if !s.Bad && float64(d) > s.Period*.5 && float64(d) < s.Period*.75 {
		s.PhasePending = true
		s.PhaseCompensated = false
		s.PhasePTS = pts
		s.PhaseDelta = d
		s.PhaseFollow = 0
		return
	}
	s.step(d)
}
func (m *meter) resolvePhases() {
	if m.Lost || len(m.Streams) != 2 {
		return
	}
	a := []*stream{}
	for _, s := range m.Streams {
		a = append(a, s)
	}
	x, y := a[0], a[1]
	if !x.PhasePending || !y.PhasePending || x.PhaseFollow < 8 || y.PhaseFollow < 8 || x.Bad || y.Bad {
		return
	}
	close := func(a, b uint64) bool {
		if a > b {
			return a-b <= 2000
		}
		return b-a <= 2000
	}
	if x.PhaseCompensated != y.PhaseCompensated || x.Period != y.Period || !close(x.PhasePTS, y.PhasePTS) || !close(x.PhaseDelta, y.PhaseDelta) {
		x.Bad = true
		y.Bad = true
		return
	}
	x.PhasePending = false
	y.PhasePending = false
	x.PhaseCorrections++
	y.PhaseCorrections++
}
