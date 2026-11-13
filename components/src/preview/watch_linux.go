package main

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

func watchPresets(settings string) (<-chan struct{}, func()) {
	fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
	if err != nil {
		return nil, func() {}
	}
	f := os.NewFile(uintptr(fd), "preset change")
	_, err = syscall.InotifyAddWatch(fd, settings, syscall.IN_MOVED_TO|syscall.IN_CLOSE_WRITE|syscall.IN_MOVE_SELF|syscall.IN_DELETE_SELF)
	if err != nil {
		f.Close()
		return nil, func() {}
	}
	events := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var buf [8192]byte
		for {
			n, e := f.Read(buf[:])
			if e != nil {
				return
			}
			for i := 0; i+syscall.SizeofInotifyEvent <= n; {
				event := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[i]))
				beg := i + syscall.SizeofInotifyEvent
				end := beg + int(event.Len)
				if end < beg || end > n {
					return
				}
				name := strings.TrimRight(string(buf[beg:end]), "\x00")
				i = end
				if name == "kPanorama_kNormalVideo.json" || name == "kSATAngle_kFreeFrameVideo.json" || event.Mask&syscall.IN_Q_OVERFLOW != 0 {
					select {
					case events <- struct{}{}:
					default:
					}
				}
				if event.Mask&syscall.IN_IGNORED != 0 {
					return
				}
			}
		}
	}()
	return events, func() { f.Close(); <-done }
}
