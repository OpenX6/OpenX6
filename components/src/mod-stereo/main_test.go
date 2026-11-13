package main

// Host fixtures: the test's effective UID stands in for root, temporary
// directories stand in for /data and /run, and a recording runner stands in for
// ins_bll_cli and the select script. Mounts, systemd, the media lock and real
// media services are not exercised here.

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	levels   = 2
	selector = "/usr/local/sbin/x6-mod-stereo-select"
	query    = `{"cmd":"share_state","action":"get","type":"media"}`
	idle     = `{"ret":"ok","data":{"capture":{"capture_status":"CaptureIdle"},"record":{"record_status":"RecordIdle","record_time_ms":0}}}`
)

type call struct {
	timeout time.Duration
	argv    []string
}

type fake struct {
	calls    []call
	media    string
	mediaErr error
	sel      func(level string) error
}

func (f *fake) run(timeout time.Duration, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{timeout, append([]string{name}, args...)})
	switch {
	case name == "/usr/bin/ins_bll_cli" && len(args) == 1 && args[0] == query:
		return []byte(f.media), f.mediaErr
	case name == "/bin/sh" && len(args) == 2 && args[0] == selector:
		if f.sel != nil {
			if e := f.sel(args[1]); e != nil {
				return []byte("select failed"), e
			}
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected command %q %q", name, args)
}

func (f *fake) selected() []string {
	var out []string
	for _, c := range f.calls {
		if c.argv[0] == "/bin/sh" {
			out = append(out, c.argv[2])
		}
	}
	return out
}

func capture(t *testing.T) *bytes.Buffer {
	var b bytes.Buffer
	w, flags := log.Writer(), log.Flags()
	log.SetOutput(&b)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(w); log.SetFlags(flags) })
	return &b
}

func mustSave(t *testing.T, p string, v byte) {
	t.Helper()
	if e := save(p, v); e != nil {
		t.Fatal(e)
	}
}

func mustRead(t *testing.T, p string, want byte) {
	t.Helper()
	if v, e := read(p); e != nil || v != want {
		t.Fatalf("%s: got %d %v, want %d", filepath.Base(p), v, e, want)
	}
}

func TestPreferenceRoundTrip(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "pref")
	last := -1
	for i := 0; i < 256; i++ {
		if e := save(p, byte(i)); (e == nil) != (i < levels) {
			t.Fatalf("save %d: %v", i, e)
		} else if e == nil {
			last = i
		}
		mustRead(t, p, byte(last))
	}
	if s, e := os.Stat(p); e != nil || s.Mode().Perm() != 0600 {
		t.Fatalf("saved mode %v %v", s, e)
	}
	if m, _ := filepath.Glob(filepath.Join(d, ".*")); len(m) != 0 {
		t.Fatalf("temporary files left: %v", m)
	}
	for _, b := range [][]byte{{}, {0, 0}, {levels}, {'0'}, {255}} {
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := read(p); e == nil {
			t.Fatalf("content %v accepted", b)
		}
	}
}

func TestPreferenceRejectsUnsafeFiles(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "pref")
	mustSave(t, p, levels-1)
	for _, m := range []os.FileMode{0620, 0602, 0622} {
		os.Chmod(p, m)
		if _, e := read(p); e == nil {
			t.Fatalf("mode %o accepted", m)
		}
	}
	os.Chmod(p, 0644)
	mustRead(t, p, levels-1)

	link := filepath.Join(d, "link")
	os.Symlink(p, link)
	if _, e := read(link); e == nil {
		t.Fatal("symlink read")
	}
	if e := save(link, 0); e == nil {
		t.Fatal("symlink overwritten")
	}
	if l, e := os.Lstat(link); e != nil || l.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	mustRead(t, p, levels-1)

	sub := filepath.Join(d, "sub")
	os.Mkdir(sub, 0700)
	if _, e := read(sub); e == nil {
		t.Fatal("directory read")
	}
	if e := save(sub, 0); e == nil {
		t.Fatal("directory replaced")
	}
	if s, e := os.Lstat(sub); e != nil || !s.IsDir() {
		t.Fatal("directory changed")
	}
}

func TestPreferenceRejectsUnsafeDirectories(t *testing.T) {
	d := t.TempDir()
	safe := filepath.Join(d, "safe")
	os.Mkdir(safe, 0700)
	p := filepath.Join(safe, "pref")
	mustSave(t, p, 0)
	for _, m := range []os.FileMode{0770, 0757, 0777} {
		os.Chmod(safe, m)
		if _, e := read(p); e == nil {
			t.Fatalf("directory mode %o read", m)
		}
		if e := save(p, levels-1); e == nil {
			t.Fatalf("directory mode %o written", m)
		}
	}
	os.Chmod(safe, 0700)
	mustRead(t, p, 0)

	alias := filepath.Join(d, "alias")
	os.Symlink(safe, alias)
	if _, e := read(filepath.Join(alias, "pref")); e == nil {
		t.Fatal("symlinked directory read")
	}
	if e := save(filepath.Join(alias, "pref"), levels-1); e == nil {
		t.Fatal("symlinked directory written")
	}
	mustRead(t, p, 0)
	if _, e := read(filepath.Join(d, "missing", "pref")); e == nil {
		t.Fatal("missing directory read")
	}
	if e := save(filepath.Join(d, "missing", "pref"), 0); e == nil {
		t.Fatal("missing directory written")
	}
}

func TestApplyCommands(t *testing.T) {
	for v := 0; v < levels; v++ {
		f := &fake{media: idle}
		if e := apply(f.run, byte(v)); e != nil {
			t.Fatal(v, e)
		}
		want := []call{
			{5 * time.Second, []string{"/usr/bin/ins_bll_cli", query}},
			{90 * time.Second, []string{"/bin/sh", selector, fmt.Sprint(v)}},
		}
		if !reflect.DeepEqual(f.calls, want) {
			t.Fatalf("level %d: %+v", v, f.calls)
		}
	}
	for v := levels; v < 256; v++ {
		f := &fake{media: idle}
		if apply(f.run, byte(v)) == nil || len(f.calls) != 0 {
			t.Fatalf("level %d reached media", v)
		}
	}
}

func TestApplyRefusesBusyOrUnknownMedia(t *testing.T) {
	cases := map[string]*fake{
		"recording":  {media: `{"ret":"ok","data":{"capture":{"capture_status":"CaptureIdle"},"record":{"record_status":"RecordRunning"}}}`},
		"starting":   {media: `{"ret":"ok","data":{"capture":{"capture_status":"CaptureIdle"},"record":{"record_status":"RecordStarting"}}}`},
		"capturing":  {media: `{"ret":"ok","data":{"capture":{"capture_status":"CaptureRunning"},"record":{"record_status":"RecordIdle"}}}`},
		"no capture": {media: `{"ret":"ok","data":{"record":{"record_status":"RecordIdle"}}}`},
		"no record":  {media: `{"ret":"ok","data":{"capture":{"capture_status":"CaptureIdle"}}}`},
		"not ok":     {media: strings.Replace(idle, `"ok"`, `"error"`, 1)},
		"not json":   {media: "Error code: Timeout"},
		"cli failed": {media: idle, mediaErr: errors.New("exit status 1")},
	}
	for name, f := range cases {
		if e := apply(f.run, 0); e == nil {
			t.Fatalf("%s accepted", name)
		}
		if s := f.selected(); len(s) != 0 || len(f.calls) != 1 {
			t.Fatalf("%s: %+v", name, f.calls)
		}
	}
	f := &fake{media: idle, sel: func(string) error { return errors.New("exit status 65") }}
	if e := apply(f.run, levels-1); e == nil || !strings.Contains(e.Error(), "select failed") {
		t.Fatalf("selector failure: %v", e)
	}
}

type fixture struct {
	data, rt, pref string
	f              *fake
	log            *bytes.Buffer
}

func newFixture(t *testing.T, saved byte) *fixture {
	d := t.TempDir()
	x := &fixture{data: filepath.Join(d, "data"), rt: filepath.Join(d, "run"), f: &fake{media: idle}, log: capture(t)}
	for _, p := range []string{x.data, x.rt} {
		if e := os.Mkdir(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	x.pref = filepath.Join(x.data, "pref")
	mustSave(t, x.pref, saved)
	mustSave(t, filepath.Join(x.rt, "state"), saved)
	return x
}

func (x *fixture) request(t *testing.T, b []byte) {
	if e := os.WriteFile(filepath.Join(x.rt, "request"), b, 0600); e != nil {
		t.Fatal(e)
	}
}

func (x *fixture) expect(t *testing.T, pref, state byte, selected ...string) {
	t.Helper()
	mustRead(t, x.pref, pref)
	mustRead(t, filepath.Join(x.rt, "state"), state)
	if _, e := os.Lstat(filepath.Join(x.rt, "request")); !os.IsNotExist(e) {
		t.Fatalf("request not consumed: %v", e)
	}
	if s := x.f.selected(); len(s)+len(selected) != 0 && !reflect.DeepEqual(s, selected) {
		t.Fatalf("selector calls %v, want %v", s, selected)
	}
}

func TestHandleAppliesAndPersists(t *testing.T) {
	for v := 0; v < levels; v++ {
		x := newFixture(t, byte((v+1)%levels))
		x.request(t, []byte{byte(v)})
		if e := handle(x.pref, x.rt, x.f.run); e != nil {
			t.Fatal(v, e)
		}
		x.expect(t, byte(v), byte(v), fmt.Sprint(v))
		if !strings.Contains(x.log.String(), fmt.Sprintf("APPLIED level=%d", v)) {
			t.Fatal(x.log.String())
		}
	}
}

func TestHandleRejectsWithoutChange(t *testing.T) {
	for _, b := range [][]byte{{levels}, {}, {0, 0}, {'1'}} {
		x := newFixture(t, levels-1)
		x.request(t, b)
		if e := handle(x.pref, x.rt, x.f.run); e == nil {
			t.Fatalf("request %v accepted", b)
		}
		if len(x.f.calls) != 0 {
			t.Fatalf("request %v reached media", b)
		}
		x.expect(t, levels-1, levels-1)
		if !strings.Contains(x.log.String(), "REJECT") {
			t.Fatal(x.log.String())
		}
	}

	x := newFixture(t, 0)
	target := filepath.Join(x.data, "elsewhere")
	mustSave(t, target, levels-1)
	os.Symlink(target, filepath.Join(x.rt, "request"))
	if handle(x.pref, x.rt, x.f.run) == nil || len(x.f.calls) != 0 {
		t.Fatal("symlinked request followed")
	}
	x.expect(t, 0, 0)
	mustRead(t, target, levels-1)

	x = newFixture(t, 0)
	x.f.media = strings.Replace(idle, "RecordIdle", "RecordRunning", 1)
	x.request(t, []byte{levels - 1})
	if handle(x.pref, x.rt, x.f.run) == nil {
		t.Fatal("busy accepted")
	}
	x.expect(t, 0, 0)

	x = newFixture(t, 0)
	x.f.sel = func(string) error { return errors.New("exit status 65") }
	x.request(t, []byte{levels - 1})
	if handle(x.pref, x.rt, x.f.run) == nil {
		t.Fatal("selector failure accepted")
	}
	x.expect(t, 0, 0, fmt.Sprint(levels-1))

	x = newFixture(t, 0)
	os.Remove(x.pref)
	x.request(t, []byte{levels - 1})
	if handle(x.pref, x.rt, x.f.run) == nil || len(x.f.calls) != 0 {
		t.Fatal("missing preference accepted")
	}
	if _, e := os.Lstat(x.pref); !os.IsNotExist(e) {
		t.Fatal("preference created on rejection")
	}
	mustRead(t, filepath.Join(x.rt, "state"), 0)
	if _, e := os.Lstat(filepath.Join(x.rt, "request")); !os.IsNotExist(e) {
		t.Fatal("request not consumed")
	}
}

// A persistence failure after a successful selector run puts the previous level
// back through the selector and rewrites both files to it.
func TestHandleRestoresPreviousOnSaveFailure(t *testing.T) {
	v, previous := byte(levels-1), byte(0)
	for _, broken := range []string{"preference", "state"} {
		x := newFixture(t, previous)
		dir := map[string]string{"preference": x.data, "state": x.rt}[broken]
		x.f.sel = func(level string) error {
			if level == fmt.Sprint(v) {
				return os.Chmod(dir, 0777)
			}
			return os.Chmod(dir, 0700)
		}
		x.request(t, []byte{v})
		if handle(x.pref, x.rt, x.f.run) == nil {
			t.Fatalf("%s failure accepted", broken)
		}
		x.expect(t, previous, previous, fmt.Sprint(v), fmt.Sprint(previous))
		if !strings.Contains(x.log.String(), "REJECT") {
			t.Fatal(x.log.String())
		}
	}

	// If putting the previous level back also fails, nothing persisted changes
	// and the failure is logged; the runtime level is then unknown.
	x := newFixture(t, previous)
	x.f.sel = func(level string) error {
		if level == fmt.Sprint(v) {
			return os.Chmod(x.data, 0777)
		}
		return errors.New("exit status 65")
	}
	x.request(t, []byte{v})
	if handle(x.pref, x.rt, x.f.run) == nil {
		t.Fatal("double failure accepted")
	}
	os.Chmod(x.data, 0700)
	x.expect(t, previous, previous, fmt.Sprint(v), fmt.Sprint(previous))
	if !strings.Contains(x.log.String(), "restore previous failed") {
		t.Fatal(x.log.String())
	}
}

func TestInitialize(t *testing.T) {
	for v := 0; v < levels; v++ {
		d := t.TempDir()
		pref, state := filepath.Join(d, "pref"), filepath.Join(d, "state")
		mustSave(t, pref, byte(v))
		if e := initialize(pref, state); e != nil {
			t.Fatal(e)
		}
		mustRead(t, state, byte(v))
	}
	d := t.TempDir()
	pref, state := filepath.Join(d, "pref"), filepath.Join(d, "state")
	if initialize(pref, state) == nil {
		t.Fatal("missing preference initialized")
	}
	os.WriteFile(pref, []byte{levels}, 0600)
	if initialize(pref, state) == nil {
		t.Fatal("invalid preference initialized")
	}
	if _, e := os.Lstat(state); !os.IsNotExist(e) {
		t.Fatal("state written after rejection")
	}
	mustSave(t, pref, 0)
	target := filepath.Join(d, "target")
	mustSave(t, target, levels-1)
	os.Symlink(target, state)
	if initialize(pref, state) == nil {
		t.Fatal("state symlink replaced")
	}
	mustRead(t, target, levels-1)
}

// The preparer seeds the same preference and state files with the same range.
func TestPreparerTableAgrees(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "mod-prepare", "main.go"))
	if e != nil {
		t.Fatal(e)
	}
	entry := fmt.Sprintf("{%q, %q, %d, ", preference, runtime+"/state", levels-1)
	if !strings.Contains(string(b), entry) {
		t.Fatalf("mod-prepare has no %s", entry)
	}
}
