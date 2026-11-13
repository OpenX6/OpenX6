package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNetworkNative(t *testing.T) {
	dir := os.Getenv("X6_GUI_NETWORK_TEST_DIR")
	if dir == "" {
		t.Skip("native harness required")
	}
	owner, key, _ := ed25519.GenerateKey(rand.Reader)
	_, wrong, _ := ed25519.GenerateKey(rand.Reader)
	_, server, _ := ed25519.GenerateKey(rand.Reader)
	cfg, e := serverTLS(server, owner)
	if e != nil {
		t.Fatal(e)
	}
	der, _ := x509.MarshalPKIXPublicKey(server.Public())
	pin := sha256.Sum256(der)
	a := newAuth(owner)
	var reached atomic.Int32
	var executions atomic.Int32
	a.execute = func(c []byte) result { executions.Add(1); return runCommand(c) }
	bl, e := net.Listen("unix", filepath.Join(dir, "auth"))
	if e != nil {
		t.Fatal(e)
	}
	defer bl.Close()
	go func() {
		for {
			c, e := bl.Accept()
			if e != nil {
				return
			}
			go a.handle(c)
		}
	}()
	f := newFrontend(cfg, filepath.Join(dir, "bridge"), func(*net.UnixConn) error { reached.Add(1); return nil }) // explicit host uid model
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			go f.handle(c)
		}
	}()
	wire := func(k ed25519.PrivateKey, p [32]byte, msg string) (string, error) {
		cc, e := clientTLS(k, p)
		if e != nil {
			return "", e
		}
		d := &net.Dialer{Timeout: time.Second}
		c, e := tls.DialWithDialer(d, "tcp", ln.Addr().String(), cc)
		if e != nil {
			return "", e
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		if e = writeFrame(c, []byte(msg)); e != nil {
			return "", e
		}
		b, e := readFrame(c, 1023)
		return string(b), e
	}
	challenge := func() [32]byte {
		s, e := wire(key, pin, challengeName)
		if e != nil {
			t.Fatal(e)
		}
		b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "OWNER_GUI_NONCE:"))
		if e != nil || len(b) != 32 {
			t.Fatal(s, e)
		}
		var n [32]byte
		copy(n[:], b)
		return n
	}
	checks := 0
	good := func(name string, test func(*testing.T)) { t.Run(name, test); checks++ }
	good("correct-key-native-roundtrip", func(t *testing.T) {
		p := sign(key, challenge(), []byte("printf NETWORK_NATIVE_OK"))
		s, e := wire(key, pin, p)
		want := "OWNER_GUI_RESULT:0:false:false:" + base64.StdEncoding.EncodeToString([]byte("NETWORK_NATIVE_OK"))
		if e != nil || s != want {
			t.Fatal(s, e)
		}
		before := executions.Load()
		s, e = wire(key, pin, p)
		if e != nil || s != denied || executions.Load() != before {
			t.Fatal("replay", s, e)
		}
	})
	good("wrong-inner-key-and-consumed-nonce", func(t *testing.T) {
		n := challenge()
		before := executions.Load()
		for _, k := range []ed25519.PrivateKey{wrong, key} {
			s, e := wire(key, pin, sign(k, n, []byte("printf MUST_NOT_RUN")))
			if e != nil || s != denied {
				t.Fatal(s, e)
			}
		}
		if executions.Load() != before {
			t.Fatal("executed")
		}
	})
	good("wrong-TLS-key-before-native", func(t *testing.T) {
		before := reached.Load()
		s, e := wire(wrong, pin, challengeName)
		if e == nil {
			t.Fatal(s)
		}
		if reached.Load() != before {
			t.Fatal("rejected TLS reached GUI bridge")
		}
	})
	good("wrong-server-pin", func(t *testing.T) {
		bad := pin
		bad[0] ^= 1
		before := reached.Load()
		s, e := wire(key, bad, challengeName)
		if e == nil {
			t.Fatal(s)
		}
		if reached.Load() != before {
			t.Fatal("rejected TLS reached GUI bridge")
		}
	})
	good("legacy-unsigned-action-rejected", func(t *testing.T) {
		s, e := wire(key, pin, "id")
		if e == nil {
			t.Fatal(s)
		}
	})
	good("oversize-network-frame", func(t *testing.T) {
		s, e := wire(key, pin, execName+strings.Repeat("A", 768))
		if e == nil {
			t.Fatal(s)
		}
	})
	good("malformed-inner-packet", func(t *testing.T) {
		s, e := wire(key, pin, execName+"notbase64")
		if e != nil || s != denied {
			t.Fatal(s, e)
		}
	})
	good("signature-tamper", func(t *testing.T) {
		p := sign(key, challenge(), []byte("printf forbidden"))
		b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(p, execName))
		b[len(b)-1] ^= 1
		s, e := wire(key, pin, execName+base64.StdEncoding.EncodeToString(b))
		if e != nil || s != denied {
			t.Fatal(s, e)
		}
	})
	good("expired-challenge", func(t *testing.T) {
		n := challenge()
		a.mu.Lock()
		a.pending[n] = time.Now().Add(-time.Second)
		a.mu.Unlock()
		s, e := wire(key, pin, sign(key, n, []byte("printf expired")))
		if e != nil || s != denied {
			t.Fatal(s, e)
		}
	})
	good("host-write-read-delete", func(t *testing.T) {
		p := filepath.Join(dir, "disposable")
		cmd := fmt.Sprintf("printf writable > '%s'; cat '%s'; rm '%s'", p, p, p)
		s, e := wire(key, pin, sign(key, challenge(), []byte(cmd)))
		want := "OWNER_GUI_RESULT:0:false:false:" + base64.StdEncoding.EncodeToString([]byte("writable"))
		if e != nil || s != want {
			t.Fatal(s, e)
		}
		if _, e = os.Stat(p); !os.IsNotExist(e) {
			t.Fatal("leftover")
		}
	})
	good("bounded-timeout", func(t *testing.T) {
		s, e := wire(key, pin, sign(key, challenge(), []byte("sleep 3")))
		if e != nil || !strings.HasPrefix(s, "OWNER_GUI_RESULT:-1:true:") {
			t.Fatal(s, e)
		}
	})
	good("broker-restart-invalidates-nonce", func(t *testing.T) {
		n := challenge()
		a.mu.Lock()
		a.pending = make(map[[32]byte]time.Time)
		a.mu.Unlock()
		s, e := wire(key, pin, sign(key, n, []byte("printf old")))
		if e != nil || s != denied {
			t.Fatal(s, e)
		}
	})
	good("concurrent-network-replay", func(t *testing.T) {
		packet := sign(key, challenge(), []byte("printf ONCE"))
		before := executions.Load()
		ch := make(chan string, 2)
		for i := 0; i < 2; i++ {
			go func() {
				s, e := wire(key, pin, packet)
				if e != nil {
					ch <- "ERROR:" + e.Error()
				} else {
					ch <- s
				}
			}()
		}
		aa, bb := <-ch, <-ch
		want := "OWNER_GUI_RESULT:0:false:false:" + base64.StdEncoding.EncodeToString([]byte("ONCE"))
		if !((aa == want && bb == denied) || (bb == want && aa == denied)) || executions.Load() != before+1 {
			t.Fatal(aa, bb, executions.Load())
		}
	})

	good("missing-broker-fails-closed", func(t *testing.T) {
		bl.Close()
		s, e := wire(key, pin, challengeName)
		if e != nil || s != denied {
			t.Fatal(s, e)
		}
	})
	if executions.Load() != 4 {
		t.Fatal("unexpected executions", executions.Load())
	}
	fmt.Printf("NETWORK_CHECKS=%d EXECUTIONS=%d HOST_UID=%d\n", checks, executions.Load(), os.Geteuid())
}
func TestTLSWithoutCertificate(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, server, _ := ed25519.GenerateKey(rand.Reader)
	cfg, _ := serverTLS(server, pub)
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(time.Second))
	b.SetDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	go func() { done <- tls.Server(a, cfg).Handshake() }()
	c := tls.Client(b, &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true})
	_ = c.Handshake()
	if <-done == nil {
		t.Fatal("missing owner certificate accepted")
	}
}
