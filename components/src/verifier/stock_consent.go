package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"os"
	"syscall"
	"time"
)

// Apply the warning to the c9 encrypted MWFI format used for factory downloads.
// The native updater still authenticates the package. Plain MWFI is open for development.
func headerBytes(n int64) int {
	if n < 104 {
		return int(n)
	}
	return 104
}

func factoryHeader(h []byte) bool {
	return len(h) >= 100 && string(h[:4]) == "MWFI" && string(bytes.TrimRight(h[8:24], "\x00")) == "c9" && binary.LittleEndian.Uint32(h[96:100]) == 1
}

const stockPrompt = "/run/openx6-stock-prompt"
const stockAnswer = "/run/openx6-stock-answer"

type stockDecision struct{ refused string }

func (d *stockDecision) allow(key string, ask func(byte) bool) bool {
	if key == "" || d.refused == key {
		return false
	}
	for _, phase := range []byte{1, 2} {
		if !ask(phase) {
			d.refused = key
			return false
		}
	}
	return true
}

// Hold an exclusive lock while the question is live. A stopped process drops it.
// Use a fresh random challenge for each screen so stale/duplicated taps cannot authorize.
func stockQuestion(phase byte, stillHere func() bool) bool {
	file, err := holdSDNotice(stockPrompt)
	if err != nil {
		return false
	}
	defer file.Close()
	var token [17]byte
	token[0] = phase
	if _, err = rand.Read(token[1:]); err != nil {
		return false
	}
	if err = os.Remove(stockAnswer); err != nil && !os.IsNotExist(err) {
		return false
	}
	defer os.Remove(stockAnswer)
	if err = file.Truncate(0); err != nil {
		return false
	}
	if n, err := file.WriteAt(token[:], 0); err != nil || n != len(token) {
		return false
	}
	if phase == 1 {
		_, _ = localCommand(2, "/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kBla", "Gui", "GuiSetScreenState", "ScreenBright", "GuiScreenStateChangeUnknown")
	}
	end := time.Now().Add(2 * time.Minute)
	for time.Now().Before(end) && stillHere() {
		fd, e := syscall.Open(stockAnswer, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if e == nil {
			f := os.NewFile(uintptr(fd), stockAnswer)
			var b [19]byte
			n, er := f.Read(b[:])
			st, se := f.Stat()
			f.Close()
			if se != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Sys().(*syscall.Stat_t).Uid != 0 {
				return false
			}
			if n != 0 {
				return er == nil && n == 18 && bytes.Equal(b[:17], token[:]) && b[17] == 1
			}
		} else if e != syscall.ENOENT {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
