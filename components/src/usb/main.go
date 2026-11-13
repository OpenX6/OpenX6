// Offline candidate for the owner's X6. No operation is run by building or
// testing this program. Deployment commands require Linux and effective uid 0.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"openx6.local/components/holder"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

const authSocket = "/run/x6-owner/auth.sock"
const factoryPub = holder.Dir + "/" + holder.Factory

func main() {
	if len(os.Args) < 2 {
		log.Fatal("expected serve-auth, switch, run-adbd, or host-sign")
	}
	if os.Args[1] == "host-sign" {
		if e := hostSign(os.Args[2:]); e != nil {
			log.Fatal(e)
		}
		return
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		log.Fatal("device commands require Linux root")
	}
	var e error
	switch os.Args[1] {
	case "stop-ins-usb":
		e = stopInsUSB()
	case "serve-auth":
		e = serveAuth()
	case "run-adbd":
		e = runADBD()
	case "switch":
		if len(os.Args) != 3 {
			log.Fatal("expected composition C001 or 901D")
		}
		e = switchUSB(&realUSB{}, os.Args[2])
	default:
		e = fmt.Errorf("unknown command")
	}
	if e != nil {
		log.Fatal(e)
	}
}
func serveAuth() error {
	if e := verifyCandidate(); e != nil {
		return e
	}
	if e := privateRunDir(); e != nil {
		return e
	}
	b, e := holder.Read("/", holder.Factory)
	if e != nil {
		return e
	}
	if len(b) != 32 {
		return fmt.Errorf("factory public key must be 32 raw Ed25519 bytes")
	}
	// Do not replace an unrelated socket or a second live instance.
	if c, e := net.DialTimeout("unix", authSocket, 100*time.Millisecond); e == nil {
		c.Close()
		return fmt.Errorf("auth broker already running")
	}
	if s, e := os.Lstat(authSocket); e == nil {
		if s.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("unexpected socket path type")
		}
		if e = os.Remove(authSocket); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	l, e := net.Listen("unix", authSocket)
	if e != nil {
		return e
	}
	defer l.Close()
	defer os.Remove(authSocket)
	if e = os.Chmod(authSocket, 0600); e != nil {
		return e
	}
	a := newAuth(ed25519.PublicKey(b))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP)
	defer signal.Stop(signals)
	done := make(chan struct{})
	defer close(done)
	readUDCState := func() string {
		paths, _ := filepath.Glob("/sys/class/udc/*/state")
		s := ""
		for _, p := range paths {
			v, _ := os.ReadFile(p)
			s += p + ":" + string(v)
		}
		return s
	}
	initialState := readUDCState()
	go func() {
		last := initialState
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-signals:
				a.invalidate()
			case <-ticker.C:
				s := readUDCState()
				if s != last {
					a.invalidate()
					last = s
				}
			case <-done:
				return
			}
		}
	}()
	for {
		c, e := l.Accept()
		if e != nil {
			return e
		}
		go a.handle(c)
	}
}
func hostSign(args []string) error {
	f := flag.NewFlagSet("host-sign", flag.ContinueOnError)
	key := f.String("key", "", "owner SSH Ed25519 private key, stays on host")
	nonce := f.String("nonce", "", "base64 challenge")
	payload := f.String("payload", "", "file containing exact factory JSON")
	out := f.String("out", "", "output UCD1 frame")
	if e := f.Parse(args); e != nil {
		return e
	}
	if *key == "" || *payload == "" || *out == "" {
		return fmt.Errorf("key, payload and out required")
	}
	b, e := readOwned(*key, true)
	if e != nil {
		return e
	}
	k, e := ssh.ParseRawPrivateKey(b)
	if e != nil {
		return e
	}
	priv, ok := k.(*ed25519.PrivateKey)
	if !ok {
		return fmt.Errorf("Ed25519 private key required")
	}
	n, e := base64.StdEncoding.DecodeString(*nonce)
	if e != nil || len(n) != 32 {
		return fmt.Errorf("invalid nonce")
	}
	var nn [32]byte
	copy(nn[:], n)
	data, e := os.ReadFile(*payload)
	if e != nil {
		return e
	}
	if len(data) == 0 || len(data) > maxPacket-108 {
		return fmt.Errorf("payload too large")
	}
	packet := signedPacket(*priv, nn, data)
	frame := ucdFrame(packet)
	fout, e := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = fout.Write(frame)
	closeErr := fout.Close()
	if e != nil {
		return e
	}
	return closeErr
}

// Factory type 254 uses WriteFactoryHead (5 bytes), not WriteUcdHead (7).
func ucdFrame(b []byte) []byte {
	n := uint32(len(b) + 5)
	return append([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24), 254}, b...)
}
