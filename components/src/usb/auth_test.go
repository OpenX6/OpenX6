package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func authFixture(t *testing.T) (*authServer, ed25519.PrivateKey) {
	t.Helper()
	pub, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return newAuth(pub), k
}
func nonceFor(t *testing.T, a *authServer) [32]byte {
	t.Helper()
	op, b := a.process([]byte(challengeRequest))
	if op != 0 {
		t.Fatal("challenge dispatched")
	}
	n, e := challengeNonce(b)
	if e != nil {
		t.Fatal(e)
	}
	return n
}

var sample = []byte(`{"cmd":"DEVICEINFO","action":"get_product_uuid"}`)

func TestVendorHeaderAndPayloadCapacity(t *testing.T) {
	for _, size := range []int{1427, 1428, 1429} {
		a, k := authFixture(t)
		payload := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, size-2)...)
		payload = append(payload, '"')
		packet := signedPacket(k, nonceFor(t, a), payload)
		before := append([]byte(nil), packet...)
		op, out := a.process(packet)
		if size > 1428 {
			if op != 0 {
				t.Fatal("oversize accepted")
			}
			continue
		}
		if op != 1 || len(out) != 15+size || len(out) > maxPacket ||
			!bytes.Equal(out[:15], make([]byte, 15)) || !bytes.Equal(out[15:], payload) {
			t.Fatal("vendor header/payload boundary mismatch")
		}
		out[15] ^= 1
		if !bytes.Equal(before, packet) {
			t.Fatal("output aliases signed request")
		}
	}
}

func TestOwnerAuthPositiveReplayAndConcurrency(t *testing.T) {
	a, k := authFixture(t)
	n := nonceFor(t, a)
	packet := signedPacket(k, n, sample)
	op, out := a.process(packet)
	if op != 1 || !bytes.Equal(out, append(make([]byte, 15), sample...)) {
		t.Fatal("valid owner request missing canonical vendor header")
	}
	if op, _ = a.process(packet); op != 0 {
		t.Fatal("replay accepted")
	}
	n = nonceFor(t, a)
	packet = signedPacket(k, n, sample)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if op, _ := a.process(packet); op == 1 {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("concurrent replay success count %d", successes.Load())
	}
}
func TestOwnerAuthNegativeCases(t *testing.T) {
	for _, kind := range []string{"raw-json", "wrong-key", "changed-payload", "changed-length", "truncated", "oversize", "expired", "restart", "usb-epoch", "wrong-prefix", "signed-invalid-json", "nonce-one-shot-after-failure"} {
		t.Run(kind, func(t *testing.T) {
			a, k := authFixture(t)
			n := nonceFor(t, a)
			p := signedPacket(k, n, sample)
			switch kind {
			case "raw-json":
				p = sample
			case "wrong-key":
				_, other := authFixture(t)
				p = signedPacket(other, n, sample)
			case "changed-payload":
				p[len(p)-2] ^= 1
			case "changed-length":
				p[104]++
			case "truncated":
				p = p[:107]
			case "oversize":
				p = signedPacket(k, n, bytes.Repeat([]byte("x"), 1536))
			case "expired":
				a.now = func() time.Time { return time.Now().Add(31 * time.Second) }
			case "restart":
				a = newAuth(a.pub)
			case "usb-epoch":
				a.invalidate()
			case "wrong-prefix":
				p[0] = 'Y'
			case "signed-invalid-json":
				p = signedPacket(k, n, []byte("not json"))
			case "nonce-one-shot-after-failure":
				bad := append([]byte(nil), p...)
				bad[40] ^= 1
				a.process(bad)
			}
			if op, _ := a.process(p); op != 0 {
				t.Fatal("unauthorized request reached dispatcher")
			}
		})
	}
}
func TestChallengeLimitsAndExpiry(t *testing.T) {
	a, _ := authFixture(t)
	now := time.Now()
	a.now = func() time.Time { return now }
	for i := 0; i < 64; i++ {
		nonceFor(t, a)
	}
	_, b := a.process([]byte(challengeRequest))
	if _, e := challengeNonce(b); e == nil {
		t.Fatal("challenge budget bypass")
	}
	now = now.Add(31 * time.Second)
	nonceFor(t, a)
	a.random = bytes.NewReader(nil)
	_, b = a.process([]byte(challengeRequest))
	if _, e := challengeNonce(b); e == nil {
		t.Fatal("random failure accepted")
	}
}
func TestRealIPCWritesInFragments(t *testing.T) {
	a, k := authFixture(t)
	n := nonceFor(t, a)
	p := signedPacket(k, n, sample)
	server, client := net.Pipe()
	defer client.Close()
	go a.handle(server)
	request := binary.LittleEndian.AppendUint32(nil, uint32(len(p)))
	request = append(request, p...)
	go func() {
		for _, b := range request {
			if _, e := client.Write([]byte{b}); e != nil {
				return
			}
		}
	}()
	var hdr [5]byte
	if _, e := io.ReadFull(client, hdr[:]); e != nil {
		t.Fatal(e)
	}
	out := make([]byte, binary.LittleEndian.Uint32(hdr[1:]))
	if _, e := io.ReadFull(client, out); e != nil {
		t.Fatal(e)
	}
	if hdr[0] != 1 || !bytes.Equal(out, append(make([]byte, 15), sample...)) {
		t.Fatal("IPC output mismatch")
	}
}
func TestIPCOversizeAndShortInput(t *testing.T) {
	for _, n := range []uint32{0, 1537, 0xffffffff} {
		a, _ := authFixture(t)
		s, c := net.Pipe()
		go a.handle(s)
		b := binary.LittleEndian.AppendUint32(nil, n)
		c.Write(b)
		c.SetReadDeadline(time.Now().Add(time.Second))
		v := make([]byte, 1)
		if _, e := c.Read(v); e == nil {
			t.Fatal("oversize IPC accepted")
		}
		c.Close()
	}
}
func TestFactoryFrameHasFiveByteHeader(t *testing.T) {
	b := ucdFrame([]byte(challengeRequest))
	if len(b) != 13 || binary.LittleEndian.Uint32(b[:4]) != 13 || b[4] != 254 || string(b[5:]) != challengeRequest {
		t.Fatal("wrong factory frame")
	}
}
