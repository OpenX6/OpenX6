package main

import (
	"io"
	"math/bits"
	"os"
)

const (
	noticeChecking      byte = 1
	noticePreparing     byte = 2
	noticeVerifying     byte = 3
	noticeHandoff       byte = 4
	noticeSourceRecheck byte = 5
	noticeFinal         byte = 6
	noticeConfirming    byte = 7
	noticeSpace         byte = 16  // 16..116 = percent of staging image allocated
	noticeCopy          byte = 128 // 128..228 = percent of input bytes copied
)

// The exclusive marker lock is authoritative. A byte left by an exited process
// has no meaning without the lock. Display failures must not change OTA policy.
type stageNotice struct {
	file         *os.File
	last, detail byte
}

func (n *stageNotice) begin(path string) bool {
	if n.file != nil {
		return false
	}
	f, err := holdSDNotice(path)
	if err != nil {
		return false
	}
	n.file = f
	n.set(noticeChecking)
	return true
}
func (n *stageNotice) set(code byte) {
	n.publish(code, 0)
}
func (n *stageNotice) close() {
	if n.file != nil {
		n.file.Close()
		n.file = nil
		n.last, n.detail = 0, 0
	}
}

// CopyN retains its exact-length and short-input semantics. Percent is reported
// only after those bytes have been written, never for the following validation.
func copyProgress(dst io.Writer, src io.Reader, size int64, n *stageNotice, phase byte) (int64, error) {
	if n == nil || n.file == nil || size <= 0 {
		return io.CopyN(dst, src, size)
	}
	var written int64
	n.set(phase)
	// ceil(size*p/100) without multiplying a potentially huge file size by 100.
	for p := int64(1); p <= 100; p++ {
		end := (size/100)*p + ((size%100)*p+99)/100
		count, err := io.CopyN(dst, src, end-written)
		written += count
		if err != nil {
			return written, err
		}
		n.set(phase + byte(p))
	}
	return written, nil
}

// The first byte stays readable by the earlier GUI. A zero extension denotes
// an indeterminate phase. Other values are percent+1 for this named phase.
func (n *stageNotice) publish(code, detail byte) {
	if n == nil || n.file == nil || (n.last == code && n.detail == detail) {
		return
	}
	if count, e := n.file.WriteAt([]byte{code, detail}, 0); e == nil && count == 2 {
		n.last, n.detail = code, detail
	}
}
func (n *stageNotice) reporter(code byte, pass, passes int) digestProgress {
	if n == nil || n.file == nil {
		return nil
	}
	previous := -1
	return func(done, total int64) {
		if total <= 0 || done < 0 || done > total {
			return
		}
		// Native MWFI sizes can exceed the 32-bit OpenX6 format. Round down.
		hi, lo := bits.Mul64(uint64(done), 100)
		fraction, _ := bits.Div64(hi, lo, uint64(total))
		percent := (pass*100 + int(fraction)) / passes
		if percent <= previous {
			return
		}
		previous = percent
		n.publish(code, byte(percent+1))
	}
}
