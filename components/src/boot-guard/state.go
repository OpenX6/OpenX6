package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	stateDir    = "/data/x6-boot-guard"
	runDir      = "/run/x6-boot"
	releaseFile = "/usr/share/x6-release-id"
	allowName   = "features-allowed"
	rescueName  = "rescue"
	levelName   = "level"
	eventsName  = "events.jsonl"
	maxLevel    = 2
	eventLimit  = 64 << 10
	recordMax   = 4096
)

// Reasons recorded in boot diagnostics.
const (
	reasonOK = "ok"

	reasonState  = "state-error"
	reasonSlot   = "slot-unknown"
	reasonForced = "owner-forced"
	reasonCard   = "sd-request"
)

// forceName holds an owner request made over SSH: "enter this level on the
// next boot, once". It lets the degraded levels be exercised on a working
// camera without breaking anything, and it self-clears.
const forceName = "force-level"

// takeForce consumes a pending one-boot level request. Anything unreadable
// or out of range is removed and ignored; it is never evidence for a level.
func (g *guard) takeForce(dir string) (int, bool) {
	p := filepath.Join(dir, forceName)
	b, err := readPrivate(p, 16)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false
	}
	if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
		g.logf("cannot consume level request (ignored): %v", rmErr)
		return 0, false
	}
	if err != nil {
		g.logf("unsafe level request removed and ignored: %v", err)
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 0 || n > maxLevel {
		g.logf("invalid level request %q removed and ignored", strings.TrimSpace(string(b)))
		return 0, false
	}
	g.logf("owner requested level %d for this boot only", n)
	return n, true
}

// force records the owner's one-boot level request. It writes nothing else
// and changes nothing about the running system.
func (g *guard) force(arg string) int {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 0 || n > maxLevel {
		fmt.Fprintf(g.errout, "level must be 0..%d\n", maxLevel)
		return 64
	}
	dir := g.p(stateDir)
	if err = secureDir(dir, true); err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	unlock, err := lockDir(dir)
	if err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	defer unlock()
	if err = writeAtomic(dir, forceName, []byte(strconv.Itoa(n)+"\n")); err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	slot, _ := g.slot()
	_ = appendEvent(dir, event{Time: g.now().Unix(), Boot: g.bootID(), Slot: slot, Release: g.release(), Event: "owner-force", Level: n})
	fmt.Fprintf(g.out, "next boot enters level %d once; it is not kept afterwards. Undo with: x6-boot-guard clear\n", n)
	return 0
}

type record struct {
	Version  int
	Slot     string
	Release  string
	Attempts int    // consecutive boots that entered without a later healthy check
	Sticky   int    // legacy field, cleared when a record is updated
	Level    int    // level decided for the latest counted boot
	Reason   string // why that level was chosen
	Boot     string // boot id of the latest counted boot
	Healthy  string // boot id of the latest healthy check
}

type guard struct {
	root   string
	bootID func() string
	slot   func() (string, error)
	uptime func() (time.Duration, error)
	sleep  func(time.Duration)
	now    func() time.Time
	probe  func(level int) []check
	out    io.Writer
	errout io.Writer
}

func production() *guard {
	g := &guard{root: "/", sleep: time.Sleep, now: time.Now, out: os.Stdout, errout: os.Stderr}
	g.bootID = g.readBootID
	g.slot = g.detectSlot
	g.uptime = g.readUptime
	g.probe = g.liveChecks
	return g
}

func (g *guard) p(name string) string { return filepath.Join(g.root, name) }

func (g *guard) logf(format string, args ...any) {
	fmt.Fprintf(g.errout, "X6_BOOT_GUARD: "+format+"\n", args...)
}

func (g *guard) readBootID() string {
	b, err := os.ReadFile(g.p("/proc/sys/kernel/random/boot_id"))
	if s := strings.TrimSpace(string(b)); err == nil && s != "" {
		return s
	}
	return "unknown-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func (g *guard) readUptime() (time.Duration, error) {
	b, err := os.ReadFile(g.p("/proc/uptime"))
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, errors.New("empty uptime")
	}
	s, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(s * float64(time.Second)), nil
}

var releasePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (g *guard) release() string {
	b, err := os.ReadFile(g.p(releaseFile))
	if s := strings.TrimSpace(string(b)); err == nil && releasePattern.MatchString(s) {
		return s
	}
	return "unknown"
}

// The running slot comes from the root device's partition name, the method
// already used on the target by x6-vr180-control; boot arguments are a
// fallback. Disagreement or absence leaves the slot history unavailable.
func (g *guard) detectSlot() (string, error) {
	found := map[string]bool{}
	if b, err := os.ReadFile(g.p("/proc/self/mountinfo")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 7 || f[4] != "/" {
				continue
			}
			u, err := os.ReadFile(g.p("/sys/dev/block/" + f[2] + "/uevent"))
			if err != nil {
				continue
			}
			for _, l := range strings.Split(string(u), "\n") {
				switch strings.TrimSpace(l) {
				case "PARTNAME=system_a":
					found["a"] = true
				case "PARTNAME=system_b":
					found["b"] = true
				}
			}
		}
	}
	if len(found) == 0 {
		for _, name := range []string{"/proc/bootconfig", "/proc/cmdline"} {
			b, err := os.ReadFile(g.p(name))
			if err != nil {
				continue
			}
			for _, m := range regexp.MustCompile(`androidboot\.slot_suffix\s*=\s*"?_([ab])"?`).FindAllSubmatch(b, -1) {
				found[string(m[1])] = true
			}
			if len(found) > 0 {
				break
			}
		}
	}
	if len(found) != 1 {
		return "", errors.New("running slot unknown or ambiguous")
	}
	for s := range found {
		return s, nil
	}
	return "", errors.New("unreachable")
}

func owned(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid())
}

func secureDir(p string, create bool) error {
	if create {
		if err := os.Mkdir(p, 0700); err != nil && !os.IsExist(err) {
			return err
		}
	}
	st, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 || !owned(st) {
		return fmt.Errorf("unsafe directory: %s", p)
	}
	return nil
}

// readPrivate refuses links, foreign owners, multiple links and loose modes.
func readPrivate(p string, max int64) ([]byte, error) {
	fd, err := syscall.Open(p, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.Mode().IsRegular() || !owned(st) || s.Nlink != 1 || st.Mode().Perm()&0077 != 0 || st.Size() > max {
		return nil, fmt.Errorf("unsafe file: %s", p)
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// writeAtomic publishes a complete, synced file and syncs the directory, so
// the count is on storage before anything is allowed to load.
func writeAtomic(dir, name string, b []byte) error {
	dest := filepath.Join(dir, name)
	if st, err := os.Lstat(dest); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("unsafe destination: %s", dest)
	}
	f, err := os.CreateTemp(dir, "."+name+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, dest); err != nil {
		return err
	}
	return syncDir(dir)
}

func lockDir(dir string) (func(), error) {
	p := filepath.Join(dir, ".lock")
	fd, err := syscall.Open(p, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	var st syscall.Stat_t
	if err = syscall.Fstat(fd, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Uid != uint32(os.Geteuid()) {
		syscall.Close(fd)
		return nil, fmt.Errorf("unsafe lock: %s", p)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { syscall.Flock(fd, syscall.LOCK_UN); syscall.Close(fd) }, nil
		}
		if err != syscall.EWOULDBLOCK || time.Now().After(deadline) {
			syscall.Close(fd)
			return nil, fmt.Errorf("state lock: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func recordName(slot string) string { return "slot-" + slot + ".json" }

// load returns the slot record. A record from another release starts fresh:
// a newly installed release gets its own tries. An unreadable or unsafe record
// is moved aside as evidence and replaced with an empty history.
func (g *guard) load(dir, slot, release, boot string) (record, bool, error) {
	fresh := record{Version: 1, Slot: slot, Release: release}
	name := filepath.Join(dir, recordName(slot))
	b, err := readPrivate(name, recordMax)
	if errors.Is(err, os.ErrNotExist) {
		return fresh, false, nil
	}
	var r record
	if err == nil {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		err = dec.Decode(&r)
		if err == nil && (r.Version != 1 || r.Slot != slot || r.Attempts < 0 || r.Sticky < 0 || r.Sticky > maxLevel) {
			err = errors.New("record fields out of range")
		}
	}
	if err != nil {
		aside := name + ".corrupt-" + sanitize(boot)
		if e := os.Rename(name, aside); e != nil {
			return fresh, true, fmt.Errorf("move corrupt record aside: %w", e)
		}
		g.logf("corrupt slot record moved aside to %s: %v", aside, err)
		return fresh, true, nil
	}
	if r.Release != release {
		return fresh, false, nil
	}
	return r, false, nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '_'
	}, strings.ToLower(s))
}

func (g *guard) save(dir string, r record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return writeAtomic(dir, recordName(r.Slot), append(b, '\n'))
}

// A queued SD request has a unique ID so an interrupted card acknowledgement
// can be retried without arming the same boot request twice.
const cardName = "card-request"

func (g *guard) armCard(id, value string) int {
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" || (value != "0" && value != "1" && value != "2") {
		return 64
	}
	dir := g.p(stateDir)
	if err := secureDir(dir, true); err != nil {
		return 1
	}
	unlock, err := lockDir(dir)
	if err != nil {
		return 1
	}
	defer unlock()
	prior, err := readPrivate(filepath.Join(dir, cardName), 128)
	if err != nil && !os.IsNotExist(err) {
		return 1
	}
	if _, err := readPrivate(filepath.Join(dir, "unack-"+id), 16); err == nil {
		return 0
	} else if !os.IsNotExist(err) {
		return 1
	}
	fields := strings.Fields(string(prior))
	if len(fields) == 3 && fields[0] == id {
		if fields[1] == value {
			return 0
		}
		return 1
	}
	// Retain an outstanding receipt if another card supersedes it before
	// the first card could fsync its acknowledgement.
	if len(fields) == 3 && len(fields[0]) == 32 && strings.Trim(fields[0], "0123456789abcdef") == "" && !strings.HasSuffix(fields[2], "-ack") {
		if err = writeAtomic(dir, "unack-"+fields[0], []byte("1\n")); err != nil {
			return 1
		}
	}
	if err = os.Remove(filepath.Join(dir, forceName)); err != nil && !os.IsNotExist(err) {
		return 1
	}
	if err = writeAtomic(dir, cardName, []byte(id+" "+value+" pending\n")); err != nil {
		return 1
	}
	return 0
}
func (g *guard) ackCard(id string) int {
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		return 64
	}
	dir := g.p(stateDir)
	if err := secureDir(dir, false); err != nil {
		return 1
	}
	unlock, err := lockDir(dir)
	if err != nil {
		return 1
	}
	defer unlock()
	data, err := readPrivate(filepath.Join(dir, cardName), 128)
	if err != nil {
		return 1
	}
	f := strings.Fields(string(data))
	if len(f) == 3 && f[0] == id && !strings.HasSuffix(f[2], "-ack") {
		if err = writeAtomic(dir, cardName, []byte(id+" "+f[1]+" "+f[2]+"-ack\n")); err != nil {
			return 1
		}
	}
	if err = os.Remove(filepath.Join(dir, "unack-"+id)); err != nil && !os.IsNotExist(err) {
		return 1
	}
	return 0
}
func (g *guard) cardLevel(dir string) (int, bool) {
	b, err := readPrivate(filepath.Join(dir, cardName), 128)
	if err != nil {
		return 0, false
	}
	f := strings.Fields(string(b))
	if len(f) != 3 || strings.TrimSuffix(f[2], "-ack") != "pending" {
		return 0, false
	}
	n, err := strconv.Atoi(f[1])
	if err != nil || n < 0 || n > 2 {
		return 0, false
	}
	if err = writeAtomic(dir, cardName, []byte(f[0]+" "+f[1]+" "+strings.Replace(f[2], "pending", "used", 1)+"\n")); err != nil {
		return 0, false
	}
	return n, true
}

type event struct {
	Time     int64  `json:"t"`
	Boot     string `json:"boot"`
	Slot     string `json:"slot,omitempty"`
	Release  string `json:"release,omitempty"`
	Event    string `json:"event"`
	Level    int    `json:"level"`
	Reason   string `json:"reason,omitempty"`
	Attempts int    `json:"attempts"`
	Checks   string `json:"checks,omitempty"`
}

// appendEvent keeps a small persistent boot history: which boots were counted
// and which reached a healthy state. A missing "healthy" after an "enter"
// means that boot died or degraded after the breaker ran.
func appendEvent(dir string, e event) error {
	p := filepath.Join(dir, eventsName)
	if st, err := os.Lstat(p); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("unsafe event log: %s", p)
		}
		if st.Size() > eventLimit {
			if err := os.Rename(p, p+".1"); err != nil {
				return err
			}
		}
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	fd, err := syscall.Open(p, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return &os.PathError{Op: "open", Path: p, Err: err}
	}
	f := os.NewFile(uintptr(fd), p)
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func tailEvents(dir string, n int) []json.RawMessage {
	var lines []json.RawMessage
	for _, name := range []string{eventsName + ".1", eventsName} {
		b, err := readPrivate(filepath.Join(dir, name), eventLimit*2)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(bytes.NewReader(b))
		s.Buffer(make([]byte, 4096), 64<<10)
		for s.Scan() {
			if json.Valid(s.Bytes()) {
				lines = append(lines, append(json.RawMessage(nil), s.Bytes()...))
			}
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// publish writes this boot's level. The allow marker exists only at level 0,
// so a breaker that never ran, crashed or could not persist leaves features
// off. The rescue marker exists only at level 2 and only after a successful
// decision: breaker failures never strip root channels.
func (g *guard) publish(level int, reason string) {
	dir := g.p(runDir)
	if err := secureDir(dir, true); err != nil {
		g.logf("cannot publish level (history unavailable): %v", err)
		return
	}
	if err := writeAtomic(dir, levelName, []byte(fmt.Sprintf("%d %s\n", level, reason))); err != nil {
		g.logf("cannot write level: %v", err)
	}
	if level == 0 {
		if err := writeAtomic(dir, allowName, []byte("1\n")); err != nil {
			g.logf("cannot publish feature marker: %v", err)
		}
	}
	if level >= 2 {
		if err := writeAtomic(dir, rescueName, []byte("1\n")); err != nil {
			g.logf("cannot publish rescue marker (root channels stay up): %v", err)
		}
	}
}

// clearMarkers removes markers left by an earlier run in this boot, so a
// decision is never inherited from a stale file.
func (g *guard) clearMarkers() error {
	for _, name := range []string{allowName, rescueName} {
		if err := os.Remove(filepath.Join(g.p(runDir), name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (g *guard) currentLevel() (int, string) {
	b, err := readPrivate(filepath.Join(g.p(runDir), levelName), 128)
	if err != nil {
		return 1, reasonState
	}
	f := strings.Fields(string(b))
	if len(f) != 2 {
		return 1, reasonState
	}
	n, err := strconv.Atoi(f[0])
	if err != nil || n < 0 || n > maxLevel {
		return 1, reasonState
	}
	return n, f[1]
}

func (g *guard) enter() int {
	if existing, err := readPrivate(filepath.Join(g.p(runDir), "decision-boot"), 128); err == nil && strings.TrimSpace(string(existing)) == g.bootID() {
		return 0
	}
	level, reason := 0, reasonState
	if err := g.clearMarkers(); err != nil {
		g.logf("cannot clear stale markers: %v", err)
		return 0
	}
	defer func() {
		g.publish(level, reason)
		if err := secureDir(g.p(runDir), false); err == nil {
			got, why := g.currentLevel()
			if got == level && why == reason {
				if e := writeAtomic(g.p(runDir), "decision-boot", []byte(g.bootID()+"\n")); e != nil {
					g.logf("boot decision receipt: %v", e)
				}
			}
		}
	}()
	boot := g.bootID()
	slot, err := g.slot()
	if err != nil {
		reason = reasonSlot
		g.logf("%v; boot history unavailable", err)
		return 0
	}
	release := g.release()
	dir := g.p(stateDir)
	if err = secureDir(dir, true); err != nil {
		g.logf("state directory unavailable (history unavailable): %v", err)
		return 0
	}
	unlock, err := lockDir(dir)
	if err != nil {
		g.logf("%v (history unavailable)", err)
		return 0
	}
	defer unlock()
	decided, why := 0, reasonOK
	if requested, ok := g.cardLevel(dir); ok {
		decided, why = requested, reasonCard
	}
	// An owner request for this one boot overrides the decision but not the
	// count: a boot that wedges while testing a level still accrues. The
	// request is consumed before anything else can fail, so a loop never
	// re-applies it.
	if forced, ok := g.takeForce(dir); ok {
		decided, why = forced, reasonForced
	}
	level, reason = decided, why
	r, _, err := g.load(dir, slot, release, boot)
	if err != nil {
		g.logf("%v (history unavailable)", err)
		return 0
	}
	r.Sticky = 0 // Discard quarantine decisions from earlier firmware.
	prior := r.Attempts
	r.Attempts++
	r.Level, r.Reason, r.Boot = decided, why, boot
	if err = g.save(dir, r); err != nil {
		g.logf("count not persisted (history unavailable): %v", err)
		return 0
	}
	if err = appendEvent(dir, event{Time: g.now().Unix(), Boot: boot, Slot: slot, Release: release, Event: "enter", Level: decided, Reason: why, Attempts: prior}); err != nil {
		g.logf("event log: %v", err)
	}
	return 0
}

func (g *guard) clear() int {
	slot, err := g.slot()
	if err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	dir := g.p(stateDir)
	if err = secureDir(dir, true); err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	unlock, err := lockDir(dir)
	if err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	defer unlock()
	if b, e := readPrivate(filepath.Join(dir, cardName), 128); e == nil {
		f := strings.Fields(string(b))
		if len(f) == 3 && strings.TrimSuffix(f[2], "-ack") == "pending" {
			if e = writeAtomic(dir, cardName, []byte(f[0]+" "+f[1]+" "+strings.Replace(f[2], "pending", "used", 1)+"\n")); e != nil {
				fmt.Fprintln(g.errout, e)
				return 1
			}
		}
	} else if !os.IsNotExist(e) {
		fmt.Fprintln(g.errout, e)
		return 1
	}
	boot, release := g.bootID(), g.release()
	r, _, err := g.load(dir, slot, release, boot)
	if err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	r.Attempts, r.Sticky = 0, 0
	if err = g.save(dir, r); err != nil {
		fmt.Fprintln(g.errout, err)
		return 1
	}
	if err = os.Remove(filepath.Join(dir, forceName)); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(g.errout, "pending level request not removed:", err)
		return 1
	}
	_ = appendEvent(dir, event{Time: g.now().Unix(), Boot: boot, Slot: slot, Release: release, Event: "owner-clear"})
	fmt.Fprintf(g.out, "slot %s cleared; features load again from the next boot\n", slot)
	return 0
}

func (g *guard) status() int {
	level, reason := g.currentLevel()
	_, allowErr := os.Lstat(filepath.Join(g.p(runDir), allowName))
	_, rescueErr := os.Lstat(filepath.Join(g.p(runDir), rescueName))
	slot, slotErr := g.slot()
	out := map[string]any{"level": level, "reason": reason, "features_allowed": allowErr == nil, "rescue": rescueErr == nil, "release": g.release()}
	if slotErr == nil {
		out["slot"] = slot
	} else {
		out["slot"] = "unknown"
	}
	dir := g.p(stateDir)
	records := map[string]json.RawMessage{}
	for _, s := range []string{"a", "b"} {
		if b, err := readPrivate(filepath.Join(dir, recordName(s)), recordMax); err == nil && json.Valid(bytes.TrimSpace(b)) {
			records[s] = json.RawMessage(bytes.TrimSpace(b))
		}
	}
	out["records"] = records
	if b, err := readPrivate(filepath.Join(dir, forceName), 16); err == nil {
		out["force_pending"] = strings.TrimSpace(string(b))
	}
	out["events"] = tailEvents(dir, 20)
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(g.out, string(b))
	return 0
}
