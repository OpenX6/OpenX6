package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMuxerProof(t *testing.T) {
	for _, c := range []struct {
		name, main, lrv, other, flags, thread string
		sz                                    int
		want                                  bool
	}{
		{"positive", "PRO_VID_20261107_202351_00_173.mp4", "PRO_LRV_20261107_202351_01_173.lrv", "", "0100001", "dio-lrv-d580", 1, true},
		{"empty", "VID_20261107_202351_00_173.mp4", "LRV_20261107_202351_01_173.lrv", "", "0100001", "dio-lrv-d580", 0, false},
		{"stale index", "VID_20261107_202351_00_173.mp4", "LRV_20261107_202351_01_172.lrv", "", "0100001", "dio-lrv-d580", 1, false},
		{"initial read", "VID_20261107_202351_00_173.mp4", "LRV_20261107_202351_01_173.lrv", "", "0100001", "dio-oth-d580", 1, false},
		{"reader", "VID_20261107_202351_00_173.mp4", "LRV_20261107_202351_01_173.lrv", "", "0100000", "dio-lrv-d580", 1, false},
		{"unrelated session", "VID_20261107_202351_00_173.mp4", "LRV_20261107_202351_01_173.lrv", "LRV_20261107_202420_01_174.lrv", "0100001", "dio-lrv-d580", 1, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir()
			for _, sub := range []string{"fd", "fdinfo", "task/1"} {
				if e := os.MkdirAll(filepath.Join(d, sub), 0700); e != nil {
					t.Fatal(e)
				}
			}
			add := func(fd, n string, sz int) {
				f := filepath.Join(d, n)
				if e := os.WriteFile(f, make([]byte, sz), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(f, filepath.Join(d, "fd", fd)); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(filepath.Join(d, "fdinfo", fd), []byte("flags:\t"+c.flags+"\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			add("3", c.main, 1)
			add("4", c.lrv, c.sz)
			if c.other != "" {
				add("5", c.other, 0)
			}
			os.WriteFile(filepath.Join(d, "task/1/comm"), []byte(c.thread), 0600)
			if pairedWriters(d) != c.want {
				t.Fatal(c.name)
			}
		})
	}
}
func TestSafeGate(t *testing.T) {
	m := sample("RecordRunning", 5000)
	m.Pipeline.Status.States = []int{3, 3, 0, 3, 0, 0, 0, 0, 3}
	p := &policy{}
	bad := func() (bool, error) { return false, errors.New("timeout") }
	yes := func() (bool, error) { return true, nil }
	r := func() bool { return true }
	if p.canStop(m, bad, r) || p.known {
		t.Fatal("failed profile")
	}
	if p.canStop(m, yes, r) || !p.known {
		t.Fatal("stale snapshot")
	}
	if p.canStop(m, yes, func() bool { return false }) || !p.canStop(m, yes, r) {
		t.Fatal("writer gate")
	}
	p.attempts = 1
	if p.canStop(m, yes, r) {
		t.Fatal("duplicate")
	}
	p.want(true, sample("RecordIdle", 0))
	p.want(true, m)
	if p.known {
		t.Fatal("stale profile")
	}
	m.Pipeline.Status.States[2] = 3
	if !p.canStop(m, bad, func() bool { return false }) {
		t.Fatal("dual changed")
	}
}

func TestInitializedSecondaryStillRequiresSquareWriter(t *testing.T) {
	m := sample("RecordRunning", 5000)
	m.Pipeline.Status.States = []int{3, 3, 1, 3, 0, 0, 0, 0, 3}
	p := &policy{known: true, square: true}
	profile := func() (bool, error) { return true, nil }
	for _, secondary := range []int{0, 1, 2, 4, 5, 6} {
		m.Pipeline.Status.States[2] = secondary
		positive := secondary == 0 || secondary == 1
		if p.canStop(m, profile, func() bool { return false }) || p.canStop(m, profile, func() bool { return true }) != positive {
			t.Errorf("secondary %d", secondary)
		}
	}
}
