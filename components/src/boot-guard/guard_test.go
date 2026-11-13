package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const relA = "1111111111111111111111111111111111111111111111111111111111111111"
const relB = "2222222222222222222222222222222222222222222222222222222222222222"

type fixture struct {
	t       *testing.T
	g       *guard
	root    string
	slot    string
	boots   int
	up      time.Duration
	healthy bool
	probes  int
	out     *bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"data", "run", "usr/share"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{t: t, root: root, slot: "a", healthy: true, out: &bytes.Buffer{}}
	f.setRelease(relA)
	f.g = &guard{
		root:   root,
		bootID: func() string { return "boot-" + string(rune('0'+f.boots%10)) + "-" + time.Duration(f.boots).String() },
		slot: func() (string, error) {
			if f.slot == "" {
				return "", errors.New("running slot unknown or ambiguous")
			}
			return f.slot, nil
		},
		uptime: func() (time.Duration, error) { return f.up, nil },
		sleep:  func(d time.Duration) { f.up += d },
		now:    func() time.Time { return time.Unix(1700000000, 0) },
		probe: func(int) []check {
			f.probes++
			return []check{{Name: "ssh-ap", Required: true, OK: f.healthy}, {Name: "factory-usb-auth", OK: false}}
		},
		out:    f.out,
		errout: &bytes.Buffer{},
	}
	return f
}

func (f *fixture) setRelease(r string) {
	if err := os.WriteFile(filepath.Join(f.root, "usr/share/x6-release-id"), []byte(r+"\n"), 0644); err != nil {
		f.t.Fatal(err)
	}
}

// boot simulates a new boot: fresh /run, then the early breaker.
func (f *fixture) boot() (int, string, bool) {
	f.t.Helper()
	f.boots++
	os.RemoveAll(filepath.Join(f.root, "run/x6-boot"))
	f.up = 2 * time.Second
	f.g.enter()
	return f.level()
}

func (f *fixture) level() (int, string, bool) {
	l, r := f.g.currentLevel()
	_, err := os.Lstat(filepath.Join(f.root, "run/x6-boot", allowName))
	return l, r, err == nil
}

func (f *fixture) record(slot string) record {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, "data/x6-boot-guard", recordName(slot)))
	if err != nil {
		f.t.Fatal(err)
	}
	var r record
	if err = json.Unmarshal(b, &r); err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f *fixture) events() []event {
	b, _ := os.ReadFile(filepath.Join(f.root, "data/x6-boot-guard", eventsName))
	var es []event
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		var e event
		if json.Unmarshal(s.Bytes(), &e) == nil {
			es = append(es, e)
		}
	}
	return es
}

func want(t *testing.T, got, want int, gotReason, wantReason string, allowed, wantAllowed bool) {
	t.Helper()
	if got != want || gotReason != wantReason || allowed != wantAllowed {
		t.Fatalf("level %d/%s allowed=%v, want %d/%s allowed=%v", got, gotReason, allowed, want, wantReason, wantAllowed)
	}
}

func TestFreshBootAllowsFeaturesAfterCountIsPersisted(t *testing.T) {
	f := newFixture(t)
	l, r, a := f.boot()
	want(t, l, 0, r, reasonOK, a, true)
	if rec := f.record("a"); rec.Attempts != 1 || rec.Release != relA || rec.Level != 0 {
		t.Fatalf("count not persisted before allowing features: %+v", rec)
	}
	if st, err := os.Stat(filepath.Join(f.root, "data/x6-boot-guard")); err != nil || st.Mode().Perm() != 0700 {
		t.Fatal("state directory must be private")
	}
}

func TestRepeatedUnhealthyBootsKeepFeatures(t *testing.T) {
	f := newFixture(t)
	l, r, a := f.boot()
	want(t, l, 0, r, reasonOK, a, true)
	l, r, a = f.boot() // one interrupted boot is tolerated
	want(t, l, 0, r, reasonOK, a, true)
	l, r, a = f.boot()
	want(t, l, 0, r, reasonOK, a, true)
	if n := len(f.events()); n != 3 {
		t.Fatalf("expected 3 enter events, got %d", n)
	}
}

func TestHealthyBootClearsCount(t *testing.T) {
	f := newFixture(t)
	f.boot()
	f.boot()
	f.g.health()
	if rec := f.record("a"); rec.Attempts != 0 || rec.Healthy == "" || rec.Sticky != 0 {
		t.Fatalf("healthy boot did not clear: %+v", rec)
	}
	l, r, a := f.boot()
	want(t, l, 0, r, reasonOK, a, true)
}

func TestUnhealthyBootKeepsCountAndRetriesWithinWindow(t *testing.T) {
	f := newFixture(t)
	f.boot()
	f.healthy = false
	f.up = 10 * time.Second
	f.g.health()
	if f.up < healthGiveUp {
		t.Fatalf("gave up early at %v", f.up)
	}
	if f.probes < 10 {
		t.Fatalf("expected repeated checks inside the window, got %d", f.probes)
	}
	if rec := f.record("a"); rec.Attempts != 1 {
		t.Fatalf("unhealthy boot must keep its count: %+v", rec)
	}
	evs := f.events()
	if last := evs[len(evs)-1]; last.Event != "unhealthy" || !strings.Contains(last.Checks, "ssh-ap=FAIL") {
		t.Fatalf("unhealthy result not logged: %+v", last)
	}
}

func TestHealthWaitsForBootToSettle(t *testing.T) {
	f := newFixture(t)
	f.boot()
	f.up = 3 * time.Second
	calledAt := time.Duration(0)
	f.g.probe = func(int) []check { calledAt = f.up; return []check{{Name: "ssh-ap", Required: true, OK: true}} }
	f.g.health()
	if calledAt < healthStart {
		t.Fatalf("health probed at %v, before settle time %v", calledAt, healthStart)
	}
}

func TestCorruptRecordIsKeptAsEvidenceAndFeaturesLoad(t *testing.T) {
	for _, kind := range []string{"garbage", "unknown-field", "out-of-range", "symlink", "loose-mode"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			dir := filepath.Join(f.root, "data/x6-boot-guard")
			os.Mkdir(dir, 0700)
			p := filepath.Join(dir, recordName("a"))
			switch kind {
			case "garbage":
				os.WriteFile(p, []byte("{not json"), 0600)
			case "unknown-field":
				os.WriteFile(p, []byte(`{"Version":1,"Slot":"a","Release":"`+relA+`","Attempts":0,"Extra":1}`), 0600)
			case "out-of-range":
				os.WriteFile(p, []byte(`{"Version":1,"Slot":"a","Release":"`+relA+`","Attempts":-4}`), 0600)
			case "symlink":
				os.WriteFile(p+".real", []byte(`{"Version":1,"Slot":"a","Release":"`+relA+`"}`), 0600)
				os.Symlink(p+".real", p)
			case "loose-mode":
				os.WriteFile(p, []byte(`{"Version":1,"Slot":"a","Release":"`+relA+`"}`), 0644)
				os.Chmod(p, 0644)
			}
			l, r, a := f.boot()
			want(t, l, 0, r, reasonOK, a, true)
			aside, _ := filepath.Glob(p + ".corrupt-*")
			if len(aside) != 1 {
				t.Fatal("corrupt record not kept as evidence")
			}
			f.g.health()
			if rec := f.record("a"); rec.Sticky != 0 {
				t.Fatal("state corruption must not quarantine features")
			}
			l, r, a = f.boot()
			want(t, l, 0, r, reasonOK, a, true)
		})
	}
}

func TestNewReleaseStartsFresh(t *testing.T) {
	f := newFixture(t)
	f.boot()
	f.boot()
	f.boot()
	f.g.health() // quarantined under release A
	f.setRelease(relB)
	l, r, a := f.boot()
	want(t, l, 0, r, reasonOK, a, true)
	if rec := f.record("a"); rec.Release != relB || rec.Sticky != 0 {
		t.Fatalf("new release did not start fresh: %+v", rec)
	}
}

func TestSlotsCountIndependently(t *testing.T) {
	f := newFixture(t)
	f.boot()
	f.boot()
	l, r, a := f.boot()
	want(t, l, 0, r, reasonOK, a, true)
	before := f.record("a")
	f.slot = "b" // the bootloader fell back to the other slot
	l, r, a = f.boot()
	want(t, l, 0, r, reasonOK, a, true)
	if after := f.record("a"); after != before {
		t.Fatalf("slot b changed slot a's record: %+v -> %+v", before, after)
	}
}

func TestUnknownSlotSkipsHistoryButAllowsFeatures(t *testing.T) {
	f := newFixture(t)
	f.slot = ""
	l, r, a := f.boot()
	want(t, l, 0, r, reasonSlot, a, true)
	if _, err := os.Lstat(filepath.Join(f.root, "data/x6-boot-guard")); !os.IsNotExist(err) {
		t.Fatal("state written without a known slot")
	}
}

func TestUnusableHistoryAllowsNormalBoot(t *testing.T) {
	for _, kind := range []string{"dir-is-file", "dir-loose", "dir-symlink", "data-readonly"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			dir := filepath.Join(f.root, "data/x6-boot-guard")
			switch kind {
			case "dir-is-file":
				os.WriteFile(dir, []byte("x"), 0600)
			case "dir-loose":
				os.Mkdir(dir, 0755)
				os.Chmod(dir, 0755)
			case "dir-symlink":
				os.Mkdir(dir+".real", 0700)
				os.Symlink(dir+".real", dir)
			case "data-readonly":
				if os.Geteuid() == 0 {
					t.Skip("root can write a mode-0500 fixture")
				}
				// Stand-in for a shared /data that did not mount writable.
				os.Chmod(filepath.Join(f.root, "data"), 0500)
				t.Cleanup(func() { os.Chmod(filepath.Join(f.root, "data"), 0755) })
			}
			// A stale allow marker from an earlier run in the same boot.
			os.MkdirAll(filepath.Join(f.root, "run/x6-boot"), 0700)
			os.WriteFile(filepath.Join(f.root, "run/x6-boot", allowName), []byte("1\n"), 0600)
			f.boots++
			f.g.enter()
			l, r, a := f.level()
			want(t, l, 0, r, reasonState, a, true)
		})
	}
}

func TestLoosePublishDirectoryNeverAllowsFeatures(t *testing.T) {
	f := newFixture(t)
	os.Mkdir(filepath.Join(f.root, "run/x6-boot"), 0755)
	os.Chmod(filepath.Join(f.root, "run/x6-boot"), 0755)
	f.boots++
	f.g.enter()
	if _, err := os.Lstat(filepath.Join(f.root, "run/x6-boot", allowName)); err == nil {
		t.Fatal("features allowed through an unsafe runtime directory")
	}
}

func TestEventLogIsBounded(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, "data/x6-boot-guard")
	os.Mkdir(dir, 0700)
	big := bytes.Repeat([]byte(`{"t":0,"boot":"x","event":"enter","level":0,"attempts":0}`+"\n"), eventLimit/50)
	os.WriteFile(filepath.Join(dir, eventsName), big, 0600)
	f.boot()
	st, err := os.Stat(filepath.Join(dir, eventsName))
	if err != nil || st.Size() > 4096 {
		t.Fatal("event log not rotated")
	}
	if _, err := os.Stat(filepath.Join(dir, eventsName+".1")); err != nil {
		t.Fatal("previous event log not kept")
	}
}

func TestStatusReportsBothSlotsAndRecentEvents(t *testing.T) {
	f := newFixture(t)
	f.boot()
	f.slot = "b"
	f.boot()
	f.out.Reset()
	if f.g.status() != 0 {
		t.Fatal("status failed")
	}
	var s map[string]json.RawMessage
	if err := json.Unmarshal(f.out.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	var recs map[string]record
	json.Unmarshal(s["records"], &recs)
	var evs []event
	json.Unmarshal(s["events"], &evs)
	if len(recs) != 2 || len(evs) != 2 {
		t.Fatalf("status incomplete: %s", f.out.String())
	}
}

func TestSlotDetection(t *testing.T) {
	for _, tc := range []struct {
		name, mountinfo, uevent, bootconfig, want string
	}{
		{"partname-a", "22 1 259:7 / / ro,relatime - ext4 /dev/sda7 ro\n", "PARTNAME=system_a\n", "", "a"},
		{"partname-b", "22 1 259:8 / / ro,relatime - ext4 /dev/sda8 ro\n", "PARTNAME=system_b\n", "", "b"},
		{"bootconfig", "", "", `androidboot.slot_suffix = "_b"` + "\n", "b"},
		{"unknown", "22 1 259:7 / / ro - ext4 x ro\n", "PARTNAME=userdata\n", "", ""},
		{"ambiguous", "", "", `androidboot.slot_suffix = "_a"` + "\n" + `androidboot.slot_suffix = "_b"` + "\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			g := &guard{root: root}
			if tc.mountinfo != "" {
				os.MkdirAll(filepath.Join(root, "proc/self"), 0755)
				os.WriteFile(filepath.Join(root, "proc/self/mountinfo"), []byte(tc.mountinfo), 0644)
				dev := strings.Fields(tc.mountinfo)[2]
				os.MkdirAll(filepath.Join(root, "sys/dev/block", dev), 0755)
				os.WriteFile(filepath.Join(root, "sys/dev/block", dev, "uevent"), []byte(tc.uevent), 0644)
			}
			if tc.bootconfig != "" {
				os.MkdirAll(filepath.Join(root, "proc"), 0755)
				os.WriteFile(filepath.Join(root, "proc/bootconfig"), []byte(tc.bootconfig), 0644)
			}
			got, err := g.detectSlot()
			if tc.want == "" {
				if err == nil {
					t.Fatalf("expected unknown slot, got %q", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestRootReadOnlyCheckUsesVisibleRoot(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "proc/self"), 0755)
	g := &guard{root: root}
	os.WriteFile(filepath.Join(root, "proc/self/mountinfo"), []byte("1 0 8:1 / / ro,relatime - ext4 a ro\n9 1 0:5 / / rw,relatime - ext4 a rw\n"), 0644)
	if g.rootReadOnly().OK {
		t.Fatal("writable visible root reported read-only")
	}
	os.WriteFile(filepath.Join(root, "proc/self/mountinfo"), []byte("1 0 8:1 / / ro,relatime - ext4 a ro\n"), 0644)
	if !g.rootReadOnly().OK {
		t.Fatal("read-only root not recognised")
	}
}

func TestSSHBannerCheck(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("SSH-2.0-Go\r\n"))
			c.Close()
		}
	}()
	if c := dialBanner(l.Addr().String()); !c.OK {
		t.Fatalf("banner not recognised: %+v", c)
	}
	l2, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		c, err := l2.Accept()
		if err == nil {
			c.Write([]byte("HTTP/1.1 400\r\n"))
			c.Close()
		}
	}()
	defer l2.Close()
	if c := dialBanner(l2.Addr().String()); c.OK {
		t.Fatal("non-SSH service accepted as SSH")
	}
	addr := l2.Addr().String()
	l2.Close()
	if c := dialBanner(addr); c.OK {
		t.Fatal("closed port accepted as SSH")
	}
}

func TestOptionalCheckDoesNotFailHealth(t *testing.T) {
	if !passed([]check{{Name: "ssh-ap", Required: true, OK: true}, {Name: "factory-usb-auth", OK: false}}) {
		t.Fatal("observe-only check failed the boot")
	}
	if passed([]check{{Name: "ssh-ap", Required: true, OK: false}}) || passed(nil) {
		t.Fatal("missing SSH passed health")
	}
}

func (f *fixture) rescue() bool {
	_, err := os.Lstat(filepath.Join(f.root, "run/x6-boot", rescueName))
	return err == nil
}

func TestBreakerFailuresNeverPublishRescue(t *testing.T) {
	for _, kind := range []string{"unknown-slot", "corrupt-record", "data-readonly"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			dir := filepath.Join(f.root, "data/x6-boot-guard")
			switch kind {
			case "unknown-slot":
				f.slot = ""
			case "corrupt-record":
				os.Mkdir(dir, 0700)
				os.WriteFile(filepath.Join(dir, recordName("a")), []byte("{truncated"), 0600)
			case "data-readonly":
				if os.Geteuid() == 0 {
					t.Skip("root can write a mode-0500 fixture")
				}
				os.Chmod(filepath.Join(f.root, "data"), 0500)
				t.Cleanup(func() { os.Chmod(filepath.Join(f.root, "data"), 0755) })
			}
			// A stale rescue marker from an earlier run in this boot must go.
			os.MkdirAll(filepath.Join(f.root, "run/x6-boot"), 0700)
			os.WriteFile(filepath.Join(f.root, "run/x6-boot", rescueName), []byte("1\n"), 0600)
			f.boots++
			f.g.enter()
			l, _, a := f.level()
			if l != 0 || !a || f.rescue() {
				t.Fatalf("level %d allowed=%v rescue=%v", l, a, f.rescue())
			}
		})
	}
}

func TestRequiredChecksByLevel(t *testing.T) {
	for _, tc := range []struct {
		level int
		name  string
		want  bool
	}{
		{0, "ssh-ap", true}, {1, "ssh-ap", true}, {2, "ssh-ap", true},
		{0, "gui-broker", true}, {1, "gui-broker", true}, {2, "gui-broker", false},
		{0, "core-services", true}, {2, "core-services", false},
		{0, "root-readonly", true}, {2, "root-readonly", false},
		{0, "factory-usb-auth", false}, {2, "factory-usb-auth", false},
	} {
		if got := requiredAt(tc.level, tc.name); got != tc.want {
			t.Errorf("level %d %s: required=%v want %v", tc.level, tc.name, got, tc.want)
		}
	}
}

func TestStatusReportsRescue(t *testing.T) {
	f := newFixture(t)
	if f.g.armCard(strings.Repeat("a", 32), "2") != 0 {
		t.Fatal("arm failed")
	}
	f.boot()
	f.out.Reset()
	f.g.status()
	var s map[string]json.RawMessage
	if err := json.Unmarshal(f.out.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if string(s["rescue"]) != "true" || string(s["level"]) != "2" {
		t.Fatalf("status: %s", f.out.String())
	}
}

// The owner can request a level for exactly one boot from a working camera.
// This is how the degraded levels get exercised on the only unit we have.
func TestOwnerForcedLevelIsOneBootAndNotSticky(t *testing.T) {
	for _, lvl := range []int{1, 2} {
		f := newFixture(t)
		f.boot()
		f.g.health()
		f.out.Reset()
		if f.g.force(strconv.Itoa(lvl)) != 0 {
			t.Fatal("force rejected")
		}
		if !strings.Contains(f.out.String(), "once") {
			t.Fatal("force did not say it is one boot only")
		}
		l, r, a := f.boot()
		want(t, l, lvl, r, reasonForced, a, false)
		if f.rescue() != (lvl == 2) {
			t.Fatalf("rescue marker=%v at forced level %d", f.rescue(), lvl)
		}
		if _, err := os.Lstat(filepath.Join(f.root, "data/x6-boot-guard", forceName)); !os.IsNotExist(err) {
			t.Fatal("request not consumed on the boot it applied to")
		}
		f.g.health() // healthy at the forced level
		if rec := f.record("a"); rec.Sticky != 0 || rec.Attempts != 0 {
			t.Fatalf("forced level became sticky: %+v", rec)
		}
		l, r, a = f.boot()
		want(t, l, 0, r, reasonOK, a, true)
		if f.rescue() {
			t.Fatal("rescue marker survived the forced boot")
		}
	}
}

func TestForcedBootStillCountsIfItWedges(t *testing.T) {
	f := newFixture(t)
	f.g.force("1")
	f.boot() // forced boot that never reaches health
	f.boot()
	l, r, a := f.boot() // third consecutive boot without health
	want(t, l, 0, r, reasonOK, a, true)
}

func TestForceRejectsBadLevelsAndIsUndoneByClear(t *testing.T) {
	f := newFixture(t)
	f.boot()
	for _, bad := range []string{"3", "-1", "x", ""} {
		if f.g.force(bad) == 0 {
			t.Fatalf("accepted level %q", bad)
		}
	}
	if _, err := os.Lstat(filepath.Join(f.root, "data/x6-boot-guard", forceName)); !os.IsNotExist(err) {
		t.Fatal("rejected request left a file")
	}
	f.g.force("2")
	f.out.Reset()
	f.g.status()
	if !strings.Contains(f.out.String(), `"force_pending": "2"`) {
		t.Fatalf("status hides the pending request: %s", f.out.String())
	}
	if f.g.clear() != 0 {
		t.Fatal("clear failed")
	}
	l, r, a := f.boot()
	want(t, l, 0, r, reasonOK, a, true)
}

func TestUnsafeForceRequestIsRemovedAndIgnored(t *testing.T) {
	for _, kind := range []string{"garbage", "out-of-range", "symlink", "loose-mode"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			dir := filepath.Join(f.root, "data/x6-boot-guard")
			os.Mkdir(dir, 0700)
			p := filepath.Join(dir, forceName)
			switch kind {
			case "garbage":
				os.WriteFile(p, []byte("two\n"), 0600)
			case "out-of-range":
				os.WriteFile(p, []byte("7\n"), 0600)
			case "symlink":
				os.WriteFile(p+".real", []byte("2\n"), 0600)
				os.Symlink(p+".real", p)
			case "loose-mode":
				os.WriteFile(p, []byte("2\n"), 0644)
				os.Chmod(p, 0644)
			}
			l, r, a := f.boot()
			want(t, l, 0, r, reasonOK, a, true)
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Fatal("unsafe request not removed")
			}
		})
	}
}

func TestSSHCheckAcceptsFallbackPortAndSaysSo(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("SSH-2.0-Go\r\n"))
			c.Close()
		}
	}()
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := closed.Addr().String()
	closed.Close()
	c := sshCheck([]string{dead, l.Addr().String()})
	if !c.OK || !strings.Contains(c.Detail, "via "+l.Addr().String()) {
		t.Fatalf("fallback port not accepted/recorded: %+v", c)
	}
	if c = sshCheck([]string{l.Addr().String(), dead}); !c.OK || c.Detail != "" {
		t.Fatalf("normal port should be silent: %+v", c)
	}
	if c = sshCheck([]string{dead}); c.OK || !strings.Contains(c.Detail, dead) {
		t.Fatalf("closed port accepted: %+v", c)
	}
}

func TestSharedEtcAuditObservesWithoutDecidingHealth(t *testing.T) {
	root := t.TempDir()
	g := &guard{root: root}
	if c := g.sharedEtcAudit(); !c.OK || c.Required {
		t.Fatalf("missing write layer must be a non-failing observation: %+v", c)
	}
	etc := filepath.Join(root, "overlay/etc")
	os.MkdirAll(filepath.Join(etc, "usb"), 0755)
	os.WriteFile(filepath.Join(etc, "usb/boot_hsusb_comp"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(etc, "adb_devid"), []byte("x"), 0644)
	if c := g.sharedEtcAudit(); !c.OK || !strings.Contains(c.Detail, "none ours") {
		t.Fatalf("vendor runtime files flagged as ours: %+v", c)
	}
	os.MkdirAll(filepath.Join(etc, "systemd/system"), 0755)
	os.WriteFile(filepath.Join(etc, "systemd/system/x6-owner-maintenance.service"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(etc, "openx6-note"), []byte("x"), 0644)
	c := g.sharedEtcAudit()
	if c.OK || c.Required || !strings.Contains(c.Detail, "x6-owner-maintenance.service") || !strings.Contains(c.Detail, "openx6-note") {
		t.Fatalf("our files in the shared layer not reported: %+v", c)
	}
	// Still observe-only: health passes at every level with this check failing.
	for lvl := 0; lvl <= 2; lvl++ {
		if requiredAt(lvl, c.Name) {
			t.Fatalf("shared-etc-audit must never be required (level %d)", lvl)
		}
	}
}

func TestManualCardLifecycle(t *testing.T) {
	f := newFixture(t)
	f.boot()
	rec := f.record("a")
	rec.Sticky = 2
	rec.Attempts = 9
	dir := f.g.p(stateDir)
	if err := f.g.save(dir, rec); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		l, _, a := f.boot()
		if l != 0 || !a {
			t.Fatal("historic quarantine or short boot disabled features")
		}
	}
	for _, level := range []int{1, 2, 0} {
		id := strings.Repeat(strconv.Itoa(level), 32)
		if f.g.armCard(id, strconv.Itoa(level)) != 0 {
			t.Fatal("arm")
		}
		l, _, a := f.boot()
		f.g.enter()
		again, _, _ := f.level()
		if again != level {
			t.Fatal("service restart changed active level")
		}
		if l != level || a != (level == 0) || f.rescue() != (level == 2) {
			t.Fatal(l)
		}
		if f.g.armCard(id, strconv.Itoa(level)) != 0 {
			t.Fatal("retry")
		}
		l, _, a = f.boot()
		if l != 0 || !a || f.rescue() {
			t.Fatal("replayed card request")
		}
	}
	f.g.armCard(strings.Repeat("c", 32), "2")
	f.g.armCard(strings.Repeat("d", 32), "0")
	l, _, a := f.boot()
	if l != 0 || !a {
		t.Fatal("normal failed to cancel pending rescue")
	}
}
