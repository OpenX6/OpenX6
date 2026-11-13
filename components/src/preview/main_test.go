package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sample(state string, preview int, ms int64) media {
	var m media
	m.Record.Status = state
	m.Record.Time = ms
	m.Capture.Status = "CaptureIdle"
	m.Pipeline.Status.States = []int{preview, 3, 3, 3, 1, 1, 0, 0, 3}
	return m
}
func fixture() (*policy, *[]bool, *[]byte) {
	calls := []bool{}
	marks := []byte{}
	p := &policy{call: func(start bool) error { calls = append(calls, start); return nil }, mark: func(v byte) error { marks = append(marks, v); return nil }}
	return p, &calls, &marks
}
func TestIdleAndDisabled(t *testing.T) {
	p, c, _ := fixture()
	for _, m := range []media{sample("RecordIdle", 3, 0), sample("RecordRunning", 3, 5000)} {
		if e := p.step(false, m); e != nil {
			t.Fatal(e)
		}
	}
	p.step(true, sample("RecordIdle", 3, 0))
	if len(*c) != 0 {
		t.Fatal(*c)
	}
}
func TestStopThenRestore(t *testing.T) {
	p, c, k := fixture()
	p.step(true, sample("RecordRunning", 3, 5000))
	p.step(true, sample("RecordRunning", 1, 1000))
	p.step(true, sample("RecordStopping", 1, 2000))
	p.step(true, sample("RecordIdle", 1, 0))
	if !reflect.DeepEqual(*c, []bool{false, true}) || !reflect.DeepEqual(*k, []byte{1, 0}) || p.owned {
		t.Fatal(*c, *k)
	}
}
func TestOffDuringRecording(t *testing.T) {
	p, c, _ := fixture()
	p.step(true, sample("RecordRunning", 3, 5000))
	p.step(false, sample("RecordRunning", 1, 1000))
	if !reflect.DeepEqual(*c, []bool{false, true}) {
		t.Fatal(*c)
	}
}
func TestDontRestoreSomeoneElsesStop(t *testing.T) {
	p, c, _ := fixture()
	p.step(true, sample("RecordRunning", 1, 1))
	p.step(true, sample("RecordIdle", 1, 0))
	p.restore()
	if len(*c) != 0 {
		t.Fatal(*c)
	}
}
func TestCrashOwnership(t *testing.T) {
	p, c, k := fixture()
	p.owned = true
	p.step(true, sample("RecordIdle", 1, 0))
	if !reflect.DeepEqual(*c, []bool{true}) || !reflect.DeepEqual(*k, []byte{0}) {
		t.Fatal(*c, *k)
	}
}
func TestRetryAndFailure(t *testing.T) {
	p, c, _ := fixture()
	p.call = func(start bool) error { *c = append(*c, start); return errors.New("failed") }
	for i := int64(0); i < 8; i++ {
		p.step(true, sample("RecordRunning", 3, 5000+i))
	}
	if len(*c) != 3 || !p.owned {
		t.Fatal(*c)
	}
	if p.restore() == nil || !p.owned {
		t.Fatal("must retain recovery ownership")
	}
}
func TestCaptureAndMissingState(t *testing.T) {
	p, c, _ := fixture()
	m := sample("RecordRunning", 3, 5000)
	m.Capture.Status = "CaptureRunning"
	p.step(true, m)
	m.Capture.Status = "CaptureIdle"
	m.Pipeline.Status.States = nil
	p.step(true, m)
	if len(*c) != 0 {
		t.Fatal(*c)
	}
}
func TestPreferenceValidation(t *testing.T) {
	d := t.TempDir()
	os.Chmod(d, 0700)
	p := filepath.Join(d, "state")
	for _, v := range []byte{0, 1} {
		if e := save(p, v); e != nil {
			t.Fatal(e)
		}
		r, e := read(p)
		if e != nil || r != v {
			t.Fatal(r, e)
		}
	}
	for _, b := range [][]byte{{}, {2}, {255}, {1, 0}} {
		os.WriteFile(p, b, 0600)
		if _, e := read(p); e == nil {
			t.Fatal(b)
		}
	}
	os.Remove(p)
	os.Symlink(filepath.Join(d, "target"), p)
	if _, e := read(p); e == nil {
		t.Fatal("symlink read")
	}
	if e := save(p, 1); e == nil {
		t.Fatal("symlink write")
	}
	if e := save(filepath.Join(d, "other"), 2); e == nil {
		t.Fatal("bad value")
	}
}
func TestNativeScope(t *testing.T) {
	old := runCommand
	defer func() { runCommand = old }()
	var a [][]string
	runCommand = func(n string, args ...string) ([]byte, error) {
		a = append(a, append([]string{n}, args...))
		return []byte("Error code: Success(0)"), nil
	}
	stream(false)
	stream(true)
	for i, x := range a {
		joined := strings.Join(x, " ")
		if !strings.HasSuffix(joined, "StreamTypePreview") || strings.Contains(joined, "systemctl") {
			t.Fatal(x)
		}
		method := "PipelineStopStream"
		if i == 1 {
			method = "PipelineStartStream"
		}
		if !strings.Contains(joined, method) {
			t.Fatal(x)
		}
	}
}

func TestStartupGuard(t *testing.T) {
	p, c, _ := fixture()
	for _, ms := range []int64{0, 1, 1000, 4999} {
		p.step(true, sample("RecordRunning", 3, ms))
	}
	if len(*c) != 0 {
		t.Fatal(*c)
	}
	for _, idx := range []int{1, 2, 8} {
		m := sample("RecordRunning", 3, 5000)
		m.Pipeline.Status.States[idx] = 2
		p.step(true, m)
	}
	if len(*c) != 0 {
		t.Fatal(*c)
	}
	p.step(true, sample("RecordRunning", 3, 5000))
	if !reflect.DeepEqual(*c, []bool{false}) {
		t.Fatal(*c)
	}
}
