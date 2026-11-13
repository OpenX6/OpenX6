package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type check struct {
	Name     string
	Required bool
	OK       bool
	Detail   string
}

const (
	healthStart  = 75 * time.Second
	healthGiveUp = 180 * time.Second
	healthPoll   = 5 * time.Second
	// The camera's own access point address. Checking SSH there (not only on
	// loopback) also proves the interface the owner connects through is up.
	apAddress = "192.168.42.1"
)

var coreServices = []string{"ins_config.service", "ins_mediad.service", "ins_gui.service"}

func passed(cs []check) bool {
	if len(cs) == 0 {
		return false
	}
	for _, c := range cs {
		if c.Required && !c.OK {
			return false
		}
	}
	return true
}

func summary(cs []check) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		s := c.Name + "="
		switch {
		case c.OK:
			s += "ok"
		case c.Required:
			s += "FAIL"
		default:
			s += "absent"
		}
		if !c.OK && c.Detail != "" {
			d := c.Detail
			if len(d) > 80 {
				d = d[:80]
			}
			s += "(" + d + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}

// health waits for the boot to settle, then checks until it passes or the
// window closes. It never restarts, stops or reboots anything; a failed
// check only leaves this boot counted, which the next boot's enter sees.
func (g *guard) health() int {
	level, reason := g.currentLevel()
	var last []check
	for {
		up, err := g.uptime()
		if err != nil {
			g.logf("uptime unavailable: %v", err)
			return 0
		}
		if up < healthStart {
			g.sleep(healthStart - up)
			continue
		}
		last = g.probe(level)
		if passed(last) {
			g.recordHealth(true, level, reason, last)
			return 0
		}
		if up >= healthGiveUp {
			break
		}
		g.sleep(healthPoll)
	}
	g.recordHealth(false, level, reason, last)
	return 0
}

func (g *guard) recordHealth(ok bool, level int, reason string, checks []check) {
	slot, err := g.slot()
	if err != nil {
		g.logf("health result not recorded: %v", err)
		return
	}
	dir := g.p(stateDir)
	if err = secureDir(dir, true); err != nil {
		g.logf("health result not recorded: %v", err)
		return
	}
	unlock, err := lockDir(dir)
	if err != nil {
		g.logf("health result not recorded: %v", err)
		return
	}
	defer unlock()
	boot, release := g.bootID(), g.release()
	e := event{Time: g.now().Unix(), Boot: boot, Slot: slot, Release: release, Level: level, Reason: reason, Checks: summary(checks)}
	if !ok {
		e.Event = "unhealthy"
		if r, _, err := g.load(dir, slot, release, boot); err == nil {
			e.Attempts = r.Attempts
		}
		if err = appendEvent(dir, e); err != nil {
			g.logf("event log: %v", err)
		}
		g.logf("boot not healthy, count kept: %s", e.Checks)
		return
	}
	r, _, err := g.load(dir, slot, release, boot)
	if err != nil {
		g.logf("health result not recorded: %v", err)
		return
	}
	e.Attempts = r.Attempts
	r.Attempts = 0
	r.Sticky = 0
	r.Healthy = boot
	if err = g.save(dir, r); err != nil {
		g.logf("healthy boot not persisted: %v", err)
		return
	}
	e.Event = "healthy"
	if err = appendEvent(dir, e); err != nil {
		g.logf("event log: %v", err)
	}
}

// sshCheck accepts the owner sshd on its normal port or on the fallback port
// the start guard uses when the regular unit was overridden; which one
// answered is recorded.
func sshCheck(addrs []string) check {
	c := check{Name: "ssh-ap", Required: true}
	for i, addr := range addrs {
		d := dialBanner(addr)
		if d.OK {
			c.OK = true
			if i > 0 {
				c.Detail = "via " + addr
			}
			return c
		}
		if c.Detail != "" {
			c.Detail += "; "
		}
		c.Detail += addr + ": " + d.Detail
	}
	return c
}

// sharedEtcAudit lists what sits in the shared /etc write layer, which both
// slots see (design R5: nothing of ours may live there). Observe-only: the
// vendor writes a few files there at runtime, so it never decides health.
func (g *guard) sharedEtcAudit() check {
	c := check{Name: "shared-etc-audit"}
	root := g.p("/overlay/etc")
	if st, err := os.Lstat(root); err != nil || !st.IsDir() {
		c.OK = true
		c.Detail = "no write layer visible"
		return c
	}
	var ours []string
	total := 0
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		total++
		if total > 2000 {
			return filepath.SkipAll
		}
		rel := strings.TrimPrefix(p, root)
		low := strings.ToLower(rel)
		if strings.Contains(low, "x6-") || strings.Contains(low, "openx6") {
			ours = append(ours, rel)
		}
		return nil
	})
	if len(ours) == 0 {
		c.OK = true
		c.Detail = fmt.Sprintf("%d entries, none ours", total)
		return c
	}
	sort.Strings(ours)
	if len(ours) > 5 {
		ours = append(ours[:5], fmt.Sprintf("+%d", len(ours)-5))
	}
	c.Detail = fmt.Sprintf("%d entries, ours: %s", total, strings.Join(ours, " "))
	return c
}

func dialBanner(addr string) check {
	c := check{Name: "ssh-ap", Required: true}
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReaderSize(conn, 256).ReadString('\n')
	if strings.HasPrefix(line, "SSH-2.0-") {
		c.OK = true
		return c
	}
	if err != nil {
		c.Detail = err.Error()
	} else {
		c.Detail = "no SSH banner"
	}
	return c
}

func dialTCP(name, addr string, required bool) check {
	c := check{Name: name, Required: required}
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	conn.Close()
	c.OK = true
	return c
}

func (g *guard) rootReadOnly() check {
	c := check{Name: "root-readonly", Required: true}
	b, err := os.ReadFile(g.p("/proc/self/mountinfo"))
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	opts := ""
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 5 && f[4] == "/" {
			opts = f[5] // last entry is the visible root
		}
	}
	for _, o := range strings.Split(opts, ",") {
		if o == "ro" {
			c.OK = true
			return c
		}
	}
	c.Detail = "root mount options: " + opts
	return c
}

func systemdActive(name string, required bool, units ...string) check {
	c := check{Name: name, Required: required}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/systemctl", append([]string{"is-active"}, units...)...).CombinedOutput()
	if err == nil {
		c.OK = true
		return c
	}
	c.Detail = strings.Join(strings.Fields(string(out)), " ")
	return c
}

// requiredAt says which checks decide health at a level. SSH is required at
// every level. In rescue (level 2) the GUI root channel and factory USB
// authorization are deliberately stopped and the vendor stack may be what is
// failing, so only SSH decides; everything else is still recorded.
func requiredAt(level int, name string) bool {
	switch name {
	case "ssh-ap":
		return true
	case "gui-broker", "root-readonly", "core-services":
		return level < 2
	default:
		return false
	}
}

// liveChecks are what the camera can verify by itself. They cannot prove the
// owner can log in and get uid 0 (the owner key never lives on the device);
// that end-to-end check stays with the owner.
func (g *guard) liveChecks(level int) []check {
	cs := []check{
		sshCheck([]string{apAddress + ":2222", apAddress + ":2223"}),
		dialTCP("gui-broker", apAddress+":56444", true),
		g.rootReadOnly(),
		systemdActive("core-services", true, coreServices...),
		systemdActive("factory-usb-auth", false, "x6-owner-auth.service"),
		g.sharedEtcAudit(),
	}
	for i := range cs {
		cs[i].Required = requiredAt(level, cs[i].Name)
	}
	return cs
}
