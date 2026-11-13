package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const maxPacket = 1536       // Existing IpcFactory capacity, not the UCD frame size.
const factoryHeaderSize = 15 // ProtocolProcessor always skips this vendor metadata header.
const challengeRequest = "X6CHAL01"
const executePrefix = "X6EXEC01"
const signatureDomain = "X6 OWNER UCD1 v1\x00"

type challenge struct {
	expires time.Time
	epoch   uint64
}
type authServer struct {
	mu      sync.Mutex
	pub     ed25519.PublicKey
	pending map[[32]byte]challenge
	epoch   uint64
	now     func() time.Time
	random  io.Reader
}

func newAuth(pub ed25519.PublicKey) *authServer {
	return &authServer{pub: append(ed25519.PublicKey(nil), pub...), pending: make(map[[32]byte]challenge), now: time.Now, random: rand.Reader}
}
func (a *authServer) invalidate() { a.mu.Lock(); defer a.mu.Unlock(); a.epoch++; clear(a.pending) }
func signatureMessage(nonce [32]byte, payload []byte) []byte {
	b := append([]byte(signatureDomain), nonce[:]...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}
func signedPacket(k ed25519.PrivateKey, nonce [32]byte, payload []byte) []byte {
	b := append([]byte(executePrefix), nonce[:]...)
	b = append(b, ed25519.Sign(k, signatureMessage(nonce, payload))...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}
func denial() []byte {
	return []byte(`{"cmd":"OWNER","action":"error","result":-1,"reason":"owner authorization required"}`)
}

// Every execution has a new nonce; no connection-wide "unlocked" flag.
// Nonces expire, are consumed even on a bad signature, and vanish on restart
// or a USB-state/composition epoch change. The exact payload bytes are signed.
func (a *authServer) process(packet []byte) (byte, []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for n, c := range a.pending {
		if !now.Before(c.expires) {
			delete(a.pending, n)
		}
	}
	if len(packet) > maxPacket {
		return 0, denial()
	}
	if string(packet) == challengeRequest {
		if len(a.pending) >= 64 {
			return 0, denial()
		}
		var n [32]byte
		if _, e := io.ReadFull(a.random, n[:]); e != nil {
			return 0, denial()
		}
		if _, exists := a.pending[n]; exists {
			return 0, denial()
		}
		a.pending[n] = challenge{now.Add(30 * time.Second), a.epoch}
		b, _ := json.Marshal(map[string]any{"cmd": "OWNER", "action": "challenge", "nonce": base64.StdEncoding.EncodeToString(n[:]), "expires_in": 30, "max_payload": maxPacket - 108})
		return 0, b
	}
	if len(packet) < 108 || string(packet[:8]) != executePrefix {
		return 0, denial()
	}
	var n [32]byte
	copy(n[:], packet[8:40])
	c, ok := a.pending[n]
	delete(a.pending, n)
	if !ok || c.epoch != a.epoch || !now.Before(c.expires) {
		return 0, denial()
	}
	size := binary.LittleEndian.Uint32(packet[104:108])
	payload := packet[108:]
	if size == 0 || int(size) != len(payload) || !json.Valid(payload) || bytes.IndexByte(payload, 0) >= 0 {
		return 0, denial()
	}
	if !ed25519.Verify(a.pub, signatureMessage(n, payload), packet[40:104]) {
		return 0, denial()
	}
	// The signature covers the exact JSON, not caller-controlled vendor metadata.
	// Synthesize the 15-byte header only after authentication. The stock parser
	// stores it for BuildResponsePacket and starts parsing at byte 15.
	out := make([]byte, factoryHeaderSize+len(payload))
	copy(out[factoryHeaderSize:], payload)
	return 1, out
}

// Linux root-only Unix socket. Wire: LE32 input length + bytes; reply:
// one action byte (0=return response,1=dispatch original), LE32 length + bytes.
func (a *authServer) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	var header [4]byte
	if _, e := io.ReadFull(c, header[:]); e != nil {
		return
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n == 0 || n > maxPacket {
		return
	}
	b := make([]byte, n)
	if _, e := io.ReadFull(c, b); e != nil {
		return
	}
	op, out := a.process(b)
	reply := binary.LittleEndian.AppendUint32([]byte{op}, uint32(len(out)))
	reply = append(reply, out...)
	for len(reply) > 0 {
		n, e := c.Write(reply)
		if e != nil || n == 0 {
			return
		}
		reply = reply[n:]
	}
}
func challengeNonce(data []byte) ([32]byte, error) {
	var n [32]byte
	var v struct {
		Nonce string `json:"nonce"`
	}
	if e := json.Unmarshal(data, &v); e != nil {
		return n, e
	}
	b, e := base64.StdEncoding.DecodeString(v.Nonce)
	if e != nil || len(b) != 32 {
		return n, errors.New("invalid nonce")
	}
	copy(n[:], b)
	return n, nil
}
