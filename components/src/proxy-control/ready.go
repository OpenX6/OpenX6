package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Only opt the square FreeFrame profile into the additional muxer check.
func squareRecording() (bool, error) {
	checks := []struct {
		key    string
		fields map[string]string
	}{
		{"kMediaMode", map[string]string{"shoot_mode": "kFreeFrameVideo", "lens_mode": "kSATAngle"}},
		{"kMediaResRatio", map[string]string{"media_res": "kRes8K", "media_ratio": "kRatio1x1"}},
		{"kMediaVideoFps", map[string]string{"video_fps": "k30"}},
	}
	for _, c := range checks {
		b, err := command("/usr/bin/ins_bll_cli", fmt.Sprintf(`{"cmd":"config","action":"get_general","key":"%s"}`, c.key))
		if err != nil {
			return false, err
		}
		var r struct {
			Ret  string
			Data map[string]json.RawMessage
		}
		if e := json.Unmarshal(b, &r); e != nil {
			return false, e
		}
		if r.Ret != "ok" {
			return false, errors.New("unavailable capture configuration")
		}
		for k, v := range c.fields {
			var s string
			if err := json.Unmarshal(r.Data[k], &s); err != nil {
				return false, err
			}
			if s != v && !(k == "media_res" && (s == "kRes5K" || s == "kRes3K")) {
				return false, nil
			}
		}
	}
	return true, nil
}

var movieName = regexp.MustCompile(`^(PRO_)?(VID|LRV)_([0-9]{8}_[0-9]{6})_(00|01)_([0-9]+)\.(mp4|lrv)$`)

// Refuse open-but-empty LRV placeholders, stale files, mismatched clips, and
// media from an album reader. Read the live daemon's own writable descriptors.
func pairedWriters(proc string) bool {
	desc, err := os.ReadDir(filepath.Join(proc, "fd"))
	if err != nil {
		return false
	}
	masks := map[string]uint8{}
	for _, f := range desc {
		fd := filepath.Join(proc, "fd", f.Name())
		target, err := os.Readlink(fd)
		if err != nil {
			continue
		}
		name := movieName.FindStringSubmatch(filepath.Base(target))
		if name == nil {
			continue
		}
		flag := uint8(0)
		switch {
		case name[2] == "VID" && name[4] == "00" && name[6] == "mp4":
			flag = 1
		case name[2] == "LRV" && name[4] == "01" && name[6] == "lrv":
			flag = 2
		default:
			continue
		}
		st, err := os.Stat(fd)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		info, err := os.ReadFile(filepath.Join(proc, "fdinfo", f.Name()))
		if err != nil {
			continue
		}
		writable := false
		for _, l := range strings.Split(string(info), "\n") {
			rest, ok := strings.CutPrefix(l, "flags:")
			if !ok {
				continue
			}
			bits, e := strconv.ParseUint(strings.TrimSpace(rest), 8, 64)
			writable = e == nil && (bits&3 == 1 || bits&3 == 2)
		}
		if !writable {
			continue
		}
		id := filepath.Join(filepath.Dir(target), name[1]+name[3]+"_"+name[5])
		masks[id] |= 0
		if st.Size() > 0 {
			masks[id] |= flag
		}
	}
	// The original recorder can keep earlier descriptors briefly while closing.
	// A second open pair is ambiguous; wait for a fresh, unique writer.
	if len(masks) != 1 {
		return false
	}
	for _, m := range masks {
		if m != 3 {
			return false
		}
	}
	tasks, err := os.ReadDir(filepath.Join(proc, "task"))
	if err != nil {
		return false
	}
	for _, task := range tasks {
		comm, e := os.ReadFile(filepath.Join(proc, "task", task.Name(), "comm"))
		if e == nil && strings.HasPrefix(string(comm), "dio-lrv-") {
			return true
		}
	}
	return false
}
func liveWriters() bool {
	raw, err := command("/bin/pidof", "ins_mediad")
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		return false
	}
	proc := fmt.Sprintf("/proc/%d", pid)
	exe, err := os.Readlink(filepath.Join(proc, "exe"))
	return err == nil && exe == "/usr/bin/ins_mediad" && pairedWriters(proc)
}
func (p *policy) canStop(m media, profile func() (bool, error), ready func() bool) bool {
	slots := m.Pipeline.Status.States
	if len(slots) < 9 {
		return false
	}
	// A raw auxiliary video port can satisfy the HAL with no encoded LRV producer.
	if len(slots) > 19 && slots[19] == 3 && (slots[3] == 0 || slots[3] == 1 || slots[3] == 6) {
		return false
	}
	if slots[2] == 3 {
		return true
	}

	if !p.known {
		sq, err := profile()
		if err != nil {
			return false
		}
		p.square, p.known = sq, true
		if sq {
			return false
		} // refresh recording state after the three configuration calls
	}
	if !p.square {
		return true
	}
	// A previously initialized secondary slot can persist across mode switches.
	secondary := slots[2]
	return (secondary == 0 || secondary == 1) && slots[1] == 3 && slots[3] == 3 && p.attempts == 0 && ready()
}
