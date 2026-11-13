package main

import "testing"

func TestSingleRawPortSwitchIndependence(t *testing.T) {
	for _, v := range []struct {
		disableProxy   byte
		disablePreview bool
		sub, raw       int
		want           int
	}{
		{1, true, 6, 3, 1}, {1, true, 0, 3, 1}, {1, false, 6, 3, 0}, {1, true, 6, 1, 0},
		{1, true, 4, 3, 0}, {1, true, 3, 3, 0}, {0, true, 3, 0, 1}, {0, false, 3, 0, 0},
	} {
		p, c, _ := fixture()
		p.singleProfile = func() (bool, error) { return true, nil }
		p.proxyPreference = func() (byte, error) { return v.disableProxy, nil }
		for ms := int64(3000); ms <= 8000; ms += 1000 {
			m := singleSample(ms, 1, 3)
			m.Pipeline.Status.States = append(m.Pipeline.Status.States, make([]int, 20-len(m.Pipeline.Status.States))...)
			m.Pipeline.Status.States[3] = v.sub
			m.Pipeline.Status.States[19] = v.raw
			if err := p.step(v.disablePreview, m); err != nil {
				t.Fatal(v, err)
			}
			if ms < 5000 && len(*c) > 0 {
				t.Fatal("early preview stop", v)
			}
		}
		if len(*c) != v.want {
			t.Fatal(v, *c)
		}
	}
}
