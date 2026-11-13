package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndependentMobileRequests(t *testing.T) {
	var uploads, installs atomic.Int32
	prompted := make(chan byte, 2)
	answers := make(chan bool)
	g := phoneEndpoint{connected: func(string) bool { return true }, ready: func() bool { return true },
		question: func(n byte, live func() bool) bool {
			prompted <- n
			for live() {
				select {
				case answer := <-answers:
					return answer
				case <-time.After(2 * time.Millisecond):
				}
			}
			return false
		}, spool: func(int64) (*os.File, error) { uploads.Add(1); return os.CreateTemp(t.TempDir(), "part") },
		install: func(phoneFile) error { installs.Add(1); return nil }}
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("ox6-test-%d.sock", os.Getpid()))
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	srv := httptest.NewUnstartedServer(&g)
	srv.Listener.Close()
	srv.Listener = listener
	srv.Config.ConnContext = phoneRequestContext
	srv.Start()
	defer srv.Close()
	cl := http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}

	data := make([]byte, 8192)
	copy(data, "MWFI")
	copy(data[8:], "c9")
	binary.LittleEndian.PutUint32(data[96:], 1)
	copy(data[len(data)-4416:], "CFSI")
	copy(data[len(data)-256:], "CWFI")
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "update.bin")
	part.Write(data)
	mw.Close()
	req := func(ctx context.Context) int {
		r, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/receive", bytes.NewReader(buf.Bytes()))
		r.Header.Set("Content-Type", mw.FormDataContentType())
		res, err := cl.Do(r)
		if err != nil {
			return -1
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}
	finish := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { finish <- req(ctx) }()
	if <-prompted != 1 {
		t.Fatal("phase 1")
	}
	answers <- true
	if <-prompted != 2 {
		t.Fatal("phase 2")
	}
	if req(context.Background()) != 409 || uploads.Load() != 0 {
		t.Fatal("concurrent upload")
	}
	cancel()
	if <-finish != -1 {
		t.Fatal("disconnected client")
	}
	// Wait for the server to observe the closed request and release its prompt.
	until := time.Now().Add(2 * time.Second)
	for !g.mu.TryLock() {
		if time.Now().After(until) {
			t.Fatal("lease survived disconnect")
		}
		time.Sleep(time.Millisecond)
	}
	if uploads.Load() != 0 || installs.Load() != 0 {
		t.Fatal("cancelled request staged")
	}
	if !g.cooldown.After(time.Now()) {
		t.Fatal("retry cooldown missing")
	}
	g.cooldown = time.Time{}
	g.mu.Unlock()
	go func() { finish <- req(context.Background()) }()
	if <-prompted != 1 {
		t.Fatal("retry inherited first answer")
	}
	answers <- true
	if <-prompted != 2 {
		t.Fatal("missing second answer")
	}
	answers <- true
	if <-finish != 200 || uploads.Load() != 1 || installs.Load() != 1 {
		t.Fatal("fresh authorization failed")
	}
}
