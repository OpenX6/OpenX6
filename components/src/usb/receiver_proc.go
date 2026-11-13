package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type procReceiver struct{ proc string }

// The observed 1.2.36 teardown hang occurs after SimpleUsb closed its fd and
// joined its read/write threads. Refuse force cleanup in every other state.
func (o procReceiver) closedTransport(p receiverProcess) error {
	base := filepath.Join(o.proc, strconv.Itoa(p.PID))
	now, ok, e := o.identity(p.PID)
	if e != nil {
		return e
	}
	if !ok || now != p {
		return fmt.Errorf("cleanup identity not present")
	}
	stat, e := os.ReadFile(base + "/stat")
	if e != nil {
		return e
	}
	i := strings.LastIndexByte(string(stat), ')')
	if i < 0 {
		return fmt.Errorf("invalid stat")
	}
	f := strings.Fields(string(stat)[i+1:])
	if len(f) < 20 || f[0] != "S" || f[19] != p.Start {
		return fmt.Errorf("cleanup requires the same sleeping receiver, not paused/running/uninterruptible")
	}
	w, e := os.ReadFile(base + "/wchan")
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(w)) != "futex_wait_queue" {
		return fmt.Errorf("receiver is not in the observed teardown wait")
	}
	fds, e := os.ReadDir(base + "/fd")
	if e != nil {
		return e
	}
	for _, f := range fds {
		link, e := os.Readlink(base + "/fd/" + f.Name())
		if e != nil {
			return e
		}
		if link == "/dev/ins_gadget" {
			return fmt.Errorf("USB device still open")
		}
	}
	tasks, e := os.ReadDir(base + "/task")
	if e != nil {
		return e
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no task identity")
	}
	for _, t := range tasks {
		comm, e := os.ReadFile(base + "/task/" + t.Name() + "/comm")
		if e != nil {
			return e
		}
		if strings.HasPrefix(strings.TrimSpace(string(comm)), "simple_usb_") {
			return fmt.Errorf("USB worker still present")
		}
	}
	return nil
}

func (o procReceiver) Now() time.Time        { return time.Now() }
func (o procReceiver) Pause(d time.Duration) { time.Sleep(d) }
func (o procReceiver) identity(pid int) (receiverProcess, bool, error) {
	base := filepath.Join(o.proc, strconv.Itoa(pid))
	comm, e := os.ReadFile(base + "/comm")
	if os.IsNotExist(e) {
		return receiverProcess{}, false, nil
	}
	if e != nil {
		return receiverProcess{}, false, e
	}
	exe, readErr := os.Readlink(base + "/exe")
	named := strings.TrimSpace(string(comm)) == "ins_usb"
	if !named && exe != "/usr/bin/ins_usb" {
		return receiverProcess{}, false, nil
	}
	stat, e := os.ReadFile(base + "/stat")
	if os.IsNotExist(e) {
		return receiverProcess{}, false, nil
	}
	if e != nil {
		return receiverProcess{}, false, e
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return receiverProcess{}, false, fmt.Errorf("malformed receiver stat")
	}
	fields := strings.Fields(string(stat)[end+1:])
	if len(fields) < 20 {
		return receiverProcess{}, false, fmt.Errorf("short receiver stat")
	}
	// A zombie has released its file descriptions, including the Ibus socket.
	if fields[0] == "Z" || fields[0] == "X" {
		return receiverProcess{}, false, nil
	}
	if os.IsNotExist(readErr) {
		return receiverProcess{}, false, &receiverExitPending{receiverProcess{pid, fields[19]}, fields[0], readErr}
	}
	if readErr != nil || exe != "/usr/bin/ins_usb" {
		return receiverProcess{}, false, fmt.Errorf("unexpected receiver executable for pid %d state %s: exe=%q error=%v", pid, fields[0], exe, readErr)
	}
	return receiverProcess{pid, fields[19]}, true, nil
}
func (o procReceiver) Snapshot() ([]receiverProcess, error) {
	entries, e := os.ReadDir(o.proc)
	if e != nil {
		return nil, e
	}
	var out []receiverProcess
	for _, entry := range entries {
		pid, e := strconv.Atoi(entry.Name())
		if e != nil || pid < 1 {
			continue
		}
		p, ok, e := o.identity(pid)
		if e != nil {
			return nil, e
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, nil
}
