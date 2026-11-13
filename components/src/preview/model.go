package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The active settings object in the 1.2.36 GUI changes before configd saves
// its preset. The registry key, object class and instruction signature are
// checked against the running image; opening proc memory never stops the GUI.
// Set by the desktop builder after laying out the selected menus.
var modeSignature string

type guiReader struct {
	mem                *os.File
	node, object, base int64
	retry              time.Time
}

func (r *guiReader) close() {
	if r.mem != nil {
		r.mem.Close()
		r.mem = nil
	}
}
func (r *guiReader) pointer(addr int64) int64 {
	var b [8]byte
	if _, err := r.mem.ReadAt(b[:], addr); err != nil {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}
func (r *guiReader) connect() bool {
	pid, err := exec.Command("systemctl", "show", "-p", "MainPID", "--value", "ins_gui").Output()
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(pid)))
	if err != nil || n <= 1 {
		return false
	}
	proc := fmt.Sprintf("/proc/%d", n)
	maps, err := os.ReadFile(filepath.Join(proc, "maps"))
	if err != nil {
		return false
	}
	r.base = 0
	for _, line := range strings.Split(string(maps), "\n") {
		v := strings.Fields(line)
		if len(v) == 6 && v[2] == "00000000" && v[5] == "/usr/bin/ins_gui" {
			r.base, _ = strconv.ParseInt(strings.Split(v[0], "-")[0], 16, 64)
			break
		}
	}
	if r.base == 0 {
		return false
	}
	r.mem, err = os.Open(filepath.Join(proc, "mem"))
	if err != nil {
		return false
	}
	var sig [16]byte
	if _, err = r.mem.ReadAt(sig[:], r.base+0x5dc488); err != nil || fmt.Sprintf("%x", sig) != modeSignature {
		return false
	}
	if r.pointer(r.base+0x7166558) != r.base+0x7033cf8 || r.pointer(r.base+0x7160fc0) != r.base+0x7033d18 {
		return false
	}
	for node, i := r.pointer(r.base+0x7341510), 0; node != 0 && i < 512; i++ {
		if r.pointer(node+8) == r.base+0x7033d10 {
			r.node = node
			r.object = r.pointer(node + 16)
			return r.object != 0 && r.pointer(r.object) == r.base+0x7033d18
		}
		node = r.pointer(node)
	}
	return false
}
func (r *guiReader) current() (window, tag string) {
	if r.mem == nil {
		if time.Now().Before(r.retry) {
			return "", ""
		}
		r.retry = time.Now().Add(time.Second)
		if !r.connect() {
			r.close()
			return "", ""
		}
	}
	if r.pointer(r.node+16) != r.object || r.pointer(r.object) != r.base+0x7033d18 || r.pointer(r.node+8) != r.base+0x7033d10 {
		r.close()
		return "", ""
	}
	var b [0x9c]byte
	if _, err := r.mem.ReadAt(b[:], r.object+0x1558); err != nil {
		r.close()
		return "", ""
	}
	u := func(i int) uint32 { return binary.LittleEndian.Uint32(b[i:]) }
	return modelWindow(u(0), u(0x98), u(4), u(8), u(0x30))
}
func modelWindow(mode, lens, res, fps, ratio uint32) (string, string) {
	if mode == 0 && lens == 1 {
		if ratio == 3 {
			switch {
			case res == 0 && fps == 10:
				return "pano-window", "T"
			case res == 5 && fps == 7:
				return "pano-window", "P"
			case res == 7 && fps == 10:
				return "pano-window", "Q"
			}
		}
		return "pano-window", ""
	}
	if mode == 2 && lens == 3 {
		switch {
		case res == 7 && ratio == 3 && fps >= 10 && fps <= 12:
			return "ff-window", "A"
		case res == 2 && (fps == 10 || fps == 11):
			return "ff-window", "B"
		case res == 0 && ratio == 1 && fps == 7:
			return "ff-window", "C"
		case res == 4 && ratio == 1 && fps == 5:
			return "ff-window", "D"
		}
		return "ff-window", ""
	}
	return "", ""
}
