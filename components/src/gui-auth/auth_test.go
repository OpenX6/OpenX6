package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func sign(k ed25519.PrivateKey, n [32]byte, c []byte) string {
	b := append([]byte(nil), n[:]...)
	b = append(b, ed25519.Sign(k, message(n, c))...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(c)))
	return execName + base64.StdEncoding.EncodeToString(append(b, c...))
}
func nonce(t *testing.T, a *auth) [32]byte {
	t.Helper()
	r := a.process(challengeName)
	b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(r, "OWNER_GUI_NONCE:"))
	if e != nil || len(b) != 32 {
		t.Fatal(r)
	}
	var n [32]byte
	copy(n[:], b)
	return n
}
func TestAuthentication(t *testing.T) {
	pub, k, _ := ed25519.GenerateKey(rand.Reader)
	_, wrong, _ := ed25519.GenerateKey(rand.Reader)
	for _, name := range []string{"correct", "wrong-key", "replay", "tamper", "expired", "restart", "unsigned", "old-domain", "nul", "empty", "oversize", "length"} {
		t.Run(name, func(t *testing.T) {
			a := newAuth(pub)
			calls := 0
			a.execute = func(c []byte) result { calls++; return result{Output: c} }
			n := nonce(t, a)
			packet := sign(k, n, []byte("printf hello"))
			want := 0
			switch name {
			case "correct":
				want = 1
			case "wrong-key":
				packet = sign(wrong, n, []byte("printf hello"))
			case "replay":
				a.process(packet)
				calls = 0
			case "tamper":
				b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(packet, execName))
				b[len(b)-1] ^= 1
				packet = execName + base64.StdEncoding.EncodeToString(b)
			case "expired":
				a.now = func() time.Time { return time.Now().Add(16 * time.Second) }
			case "restart":
				a = newAuth(pub)
				a.execute = func(c []byte) result { calls++; return result{} }
			case "unsigned":
				packet = "id"
			case "old-domain":
				b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(packet, execName))
				m := append([]byte("X6 OWNER UCD1 v1\x00"), n[:]...)
				m = binary.LittleEndian.AppendUint32(m, 12)
				m = append(m, []byte("printf hello")...)
				copy(b[32:96], ed25519.Sign(k, m))
				packet = execName + base64.StdEncoding.EncodeToString(b)
			case "nul":
				packet = sign(k, n, []byte("id\x00x"))
			case "empty":
				packet = sign(k, n, nil)
			case "oversize":
				packet = sign(k, n, []byte(strings.Repeat("x", 401)))
			case "length":
				b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(packet, execName))
				b[96]++
				packet = execName + base64.StdEncoding.EncodeToString(b)
			}
			r := a.process(packet)
			if calls != want {
				t.Fatalf("calls %d want %d", calls, want)
			}
			if want == 0 && r != denied {
				t.Fatal(r)
			}
		})
	}
}
func TestConsumedOnBadSignature(t *testing.T) {
	pub, k, _ := ed25519.GenerateKey(rand.Reader)
	_, w, _ := ed25519.GenerateKey(rand.Reader)
	a := newAuth(pub)
	n := nonce(t, a)
	a.process(sign(w, n, []byte("id")))
	if a.process(sign(k, n, []byte("id"))) != denied {
		t.Fatal("nonce reused")
	}
}
func TestConcurrentReplay(t *testing.T) {
	pub, k, _ := ed25519.GenerateKey(rand.Reader)
	a := newAuth(pub)
	var calls atomic.Int32
	a.execute = func([]byte) result { calls.Add(1); return result{} }
	p := sign(k, nonce(t, a), []byte("id"))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); a.process(p) }()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestNonceCapacityAndEntropy(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := newAuth(pub)
	for i := 0; i < 32; i++ {
		nonce(t, a)
	}
	if a.process(challengeName) != denied {
		t.Fatal("capacity")
	}
	a = newAuth(pub)
	a.random = strings.NewReader("")
	if a.process(challengeName) != denied {
		t.Fatal("entropy failure")
	}
}
func TestCommandBounds(t *testing.T) {
	r := runCommand([]byte("printf GUI_OK"))
	if r.Code != 0 || string(r.Output) != "GUI_OK" {
		t.Fatal(r)
	}
	r = runCommand([]byte("exit 7"))
	if r.Code != 7 {
		t.Fatal(r)
	}
	r = runCommand([]byte("yes x | head -c 10000"))
	if !r.Truncated || len(r.Output) != 384 {
		t.Fatal(r)
	}
	start := time.Now()
	r = runCommand([]byte("sleep 3"))
	if !r.Timeout || time.Since(start) > 1500*time.Millisecond {
		t.Fatal(r)
	}
}
func TestFraming(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := newAuth(pub)
	for _, n := range []uint32{0, 768, 0xffffffff} {
		c, s := net.Pipe()
		go a.handle(s)
		h := binary.LittleEndian.AppendUint32(nil, n)
		c.Write(h)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		var b [1]byte
		if _, e := c.Read(b[:]); e == nil {
			t.Fatal("oversize response")
		}
		c.Close()
	}
}

// Invoked only by the offline Unicorn harness. This is a Go test executable,
// not a production CLI mode. All keys are newly generated temporary fixtures.
func TestNativeHarness(t *testing.T) {
	path := os.Getenv("X6_GUI_TEST_SOCKET")
	if path == "" {
		t.Skip("native harness only")
	}
	pub, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path+".fixture", k, 0600); e != nil {
		t.Fatal(e)
	}
	ln, e := net.Listen("unix", path)
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	a := newAuth(pub)
	fmt.Println("HARNESS_READY")
	for {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		go a.handle(c)
	}
}

var _ io.Reader
