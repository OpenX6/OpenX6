// Read only the LCD preview plane. Never import an encoder or sensor buffer.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const root = "/run/openx6-clip/"
const W = 480
const H = 600

type plane struct {
	id, crtc, fb, possible, gamma, count uint32
	ptr                                  uint64
}
type fb struct {
	id, w, h, format, flags   uint32
	handles, pitches, offsets [4]uint32
	_                         uint32
	mods                      [4]uint64
}
type prime struct {
	h, flags uint32
	fd       int32
}

func io(fd int, req uintptr, arg unsafe.Pointer) bool {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	return e == 0
}

type rect struct{ w, h, x, y int }

func window(state string) (rect, bool) {
	var g rect
	_, s, ok := strings.Cut(state, "plane[131]:")
	if !ok {
		return rect{}, false
	}
	s, _, _ = strings.Cut(s, "\nplane[")
	for _, v := range []string{"crtc=crtc-0\n", "format=NV12 ", "rotation=1\n", "color-encoding=ITU-R BT.601 YCbCr\n", "color-range=YCbCr limited range\n"} {
		if !strings.Contains(s+"\n", v) {
			return rect{}, false
		}
	}
	for _, line := range strings.Split(s, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "crtc-pos=") {
			n, e := fmt.Sscanf(l, "crtc-pos=%dx%d+%d+%d", &g.w, &g.h, &g.x, &g.y)
			if e != nil || n != 4 || l != fmt.Sprintf("crtc-pos=%dx%d+%d+%d", g.w, g.h, g.x, g.y) {
				return rect{}, false
			}
		}
	}
	if g.w <= 0 || g.w > W || g.w%8 != 0 || g.h <= 0 || g.h > H || g.h%2 != 0 || g.x < 0 || g.x > W-g.w || g.x%8 != 0 || g.y < 0 || g.y > H-g.h {
		return rect{}, false
	}
	src := fmt.Sprintf("src-pos=%d.000000x%d.000000+0.000000+0.000000\n", g.w, g.h)
	return g, strings.Contains(s+"\n", src)
}
func geometry() (rect, bool) {
	b, e := os.ReadFile("/sys/class/backlight/panel0-backlight/brightness")
	if e != nil || strings.TrimSpace(string(b)) == "0" {
		return rect{}, false
	}
	b, e = os.ReadFile("/sys/kernel/debug/dri/0/state")
	if e != nil {
		return rect{}, false
	}
	return window(string(b))
}
func frame(fd int, mask []byte, log bool, g rect) bool {
	p := plane{id: 131}
	if !io(fd, 0xc02064b6, unsafe.Pointer(&p)) || p.fb == 0 {
		return false
	}
	f := fb{id: p.fb}
	if !io(fd, 0xc06864ce, unsafe.Pointer(&f)) {
		return false
	}
	for i, h := range f.handles {
		if h != 0 {
			dup := false
			for j := 0; j < i; j++ {
				if h == f.handles[j] {
					dup = true
				}
			}
			if !dup {
				defer func(h uint32) { c := [2]uint32{h, 0}; io(fd, 0x40086409, unsafe.Pointer(&c)) }(h)
			}
		}
	}
	if f.format != 0x3231564e || f.w < uint32(g.w) || f.h < uint32(g.h) || f.w > 1024 || f.h > 1024 || f.mods[0] != 0 || f.mods[1] != 0 || f.handles[0] == 0 || f.handles[0] != f.handles[1] {
		return false
	}
	for i := 0; i < 2; i++ {
		if f.pitches[i] < uint32(g.w) || f.pitches[i] > 4096 || f.offsets[i] > 4000000 {
			return false
		}
	}
	size := int(f.offsets[1] + f.pitches[1]*uint32(g.h/2))
	if int(f.offsets[0]+f.pitches[0]*uint32(g.h)) > size {
		return false
	}
	ph := prime{f.handles[0], syscall.O_CLOEXEC, -1}
	if !io(fd, 0xc00c642d, unsafe.Pointer(&ph)) {
		return false
	}
	defer syscall.Close(int(ph.fd))
	m, e := syscall.Mmap(int(ph.fd), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if e != nil {
		return false
	}
	defer syscall.Munmap(m)
	sync := uint64(1)
	if !io(int(ph.fd), 0x40086200, unsafe.Pointer(&sync)) {
		return false
	}
	for i := range mask {
		mask[i] = 0
	}
	for y := 0; y < g.h; y++ {
		a := int(f.offsets[0]) + y*int(f.pitches[0])
		b := int(f.offsets[1]) + y/2*int(f.pitches[1])
		for x := 0; x < g.w; x += 8 {
			var bits byte
			for k := 0; k < 8; k++ {
				uv := b + x + (k &^ 1)
				if warning(int(m[a+x+k]), int(m[uv]), int(m[uv+1]), log) {
					bits |= 1 << k
				}
			}
			mask[((y+g.y)*W+x+g.x)/8] = bits
		}
	}
	sync = 5
	return io(int(ph.fd), 0x40086200, unsafe.Pointer(&sync))
}
func mono() uint64 {
	var ts syscall.Timespec
	_, _, e := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&ts)), 0)
	if e != 0 {
		return 0
	}
	return uint64(ts.Sec)*1000 + uint64(ts.Nsec)/1000000
}
func main() {
	fd, e := syscall.Open("/dev/dri/card0", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if e != nil {
		panic(e)
	}
	defer syscall.Close(fd)
	cap := [2]uint64{2, 1}
	if !io(fd, 0x4010640d, unsafe.Pointer(&cap)) {
		panic("plane access")
	}
	data := make([]byte, 8+W*H/8)
	valid := false
	var nextProfile uint64
	var log, pending bool
	var epoch uint64
	type result struct {
		epoch uint64
		log   bool
	}
	results := make(chan result, 1)
	for range time.Tick(100 * time.Millisecond) {
		b, e := os.ReadFile(root + "demand")
		now := mono()
		demand := e == nil && len(b) == 8 && now-binary.LittleEndian.Uint64(b) <= 550
		if !demand {
			nextProfile = 0
			log = false
			epoch++
		}
		select {
		case r := <-results:
			pending = false
			if demand && r.epoch == epoch {
				log = r.log
			}
		default:
		}
		if demand && !pending && now >= nextProfile {
			pending = true
			nextProfile = now + 4000
			go func(e uint64) { results <- result{e, logPreview()} }(epoch)
		}
		var g rect
		var ok bool
		if demand {
			g, ok = geometry()
		}
		if !ok || !frame(fd, data[8:], log, g) {
			if valid {
				os.Remove(root + "mask")
				valid = false
			}
			continue
		}
		binary.LittleEndian.PutUint64(data, now)
		if os.WriteFile(root+"next", data, 0600) == nil && os.Rename(root+"next", root+"mask") == nil {
			if !valid {
				fmt.Println("preview active")
			}
			valid = true
		}
	}
}
