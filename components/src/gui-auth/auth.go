package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const domain = "X6 OWNER GUI ACTION v1\x00"
const challengeName = "X6GUI_CHALLENGE_V1"
const execName = "X6GUI_EXEC_V1:"
const denied = "OWNER_GUI_DENIED"
const maxCommand = 400

type result struct {
	Code      int
	Output    []byte
	Timeout   bool
	Truncated bool
}
type auth struct {
	mu      sync.Mutex
	pub     ed25519.PublicKey
	pending map[[32]byte]time.Time
	now     func() time.Time
	random  io.Reader
	busy    chan struct{}
	execute func([]byte) result
}

func newAuth(pub ed25519.PublicKey) *auth {
	return &auth{pub: append(ed25519.PublicKey(nil), pub...), pending: make(map[[32]byte]time.Time), now: time.Now, random: rand.Reader, busy: make(chan struct{}, 1), execute: runCommand}
}
func message(nonce [32]byte, command []byte) []byte {
	b := append([]byte(domain), nonce[:]...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(command)))
	return append(b, command...)
}
func (a *auth) authorize(input string) (reply string, command []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for n, t := range a.pending {
		if !now.Before(t) {
			delete(a.pending, n)
		}
	}
	if len(input) > 767 {
		return denied, nil
	}
	if input == challengeName {
		if len(a.pending) >= 32 {
			return denied, nil
		}
		var n [32]byte
		if _, e := io.ReadFull(a.random, n[:]); e != nil {
			return denied, nil
		}
		if _, ok := a.pending[n]; ok {
			return denied, nil
		}
		a.pending[n] = now.Add(15 * time.Second)
		return "OWNER_GUI_NONCE:" + base64.StdEncoding.EncodeToString(n[:]), nil
	}
	if len(input) <= len(execName) || input[:len(execName)] != execName {
		return denied, nil
	}
	b, e := base64.StdEncoding.Strict().DecodeString(input[len(execName):])
	if e != nil || len(b) < 100 {
		return denied, nil
	}
	var n [32]byte
	copy(n[:], b[:32])
	expiry, ok := a.pending[n]
	delete(a.pending, n)
	c := b[100:]
	if !ok || !now.Before(expiry) || len(c) == 0 || len(c) > maxCommand || int(binary.LittleEndian.Uint32(b[96:100])) != len(c) || bytes.IndexByte(c, 0) >= 0 {
		return denied, nil
	}
	if !ed25519.Verify(a.pub, message(n, c), b[32:96]) {
		return denied, nil
	}
	return "", append([]byte(nil), c...)
}
func (a *auth) process(input string) string {
	reply, command := a.authorize(input)
	if command == nil {
		return reply
	}
	select {
	case a.busy <- struct{}{}:
	default:
		return "OWNER_GUI_BUSY"
	}
	defer func() { <-a.busy }()
	r := a.execute(command)
	return fmt.Sprintf("OWNER_GUI_RESULT:%d:%t:%t:%s", r.Code, r.Timeout, r.Truncated, base64.StdEncoding.EncodeToString(r.Output))
}

// Private socket is reached by the patched GUI action. The TLS frontend
// forwards through the native bridge and never calls process directly.
func (a *auth) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	var header [4]byte
	if _, e := io.ReadFull(c, header[:]); e != nil {
		return
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n == 0 || n > 767 {
		return
	}
	b := make([]byte, n)
	if _, e := io.ReadFull(c, b); e != nil {
		return
	}
	reply := []byte(a.process(string(b)))
	out := binary.LittleEndian.AppendUint32(nil, uint32(len(reply)))
	out = append(out, reply...)
	for len(out) > 0 {
		n, e := c.Write(out)
		if e != nil || n == 0 {
			return
		}
		out = out[n:]
	}
}
