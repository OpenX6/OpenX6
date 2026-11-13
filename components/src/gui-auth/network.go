package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"net"
	"time"
)

// TLS proves possession of the pinned owner key before reaching native code.
// Inner signatures retain one-shot authorization even across TLS connections.
func certificate(k ed25519.PrivateKey) (tls.Certificate, error) {
	t := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "X6 owner GUI"}, NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, e := x509.CreateCertificate(rand.Reader, t, t, k.Public(), k)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}, e
}
func serverTLS(k ed25519.PrivateKey, owner ed25519.PublicKey) (*tls.Config, error) {
	cert, e := certificate(k)
	if e != nil {
		return nil, e
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAnyClientCert, SessionTicketsDisabled: true, VerifyConnection: func(s tls.ConnectionState) error {
		if len(s.PeerCertificates) != 1 {
			return errors.New("owner certificate required")
		}
		p, ok := s.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
		if !ok || subtle.ConstantTimeCompare(p, owner) != 1 {
			return errors.New("owner key mismatch")
		}
		return nil
	}}, nil
}
func readFrame(c net.Conn, max uint32) ([]byte, error) {
	var h [4]byte
	if _, e := io.ReadFull(c, h[:]); e != nil {
		return nil, e
	}
	n := binary.LittleEndian.Uint32(h[:])
	if n == 0 || n > max {
		return nil, errors.New("frame size")
	}
	b := make([]byte, n)
	_, e := io.ReadFull(c, b)
	return b, e
}
func writeFrame(c net.Conn, b []byte) error {
	b = append(binary.LittleEndian.AppendUint32(nil, uint32(len(b))), b...)
	for len(b) > 0 {
		n, e := c.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

type frontend struct {
	config *tls.Config
	bridge string
	peer   func(*net.UnixConn) error
	slots  chan struct{}
}

func newFrontend(config *tls.Config, path string, peer func(*net.UnixConn) error) *frontend {
	return &frontend{config, path, peer, make(chan struct{}, 4)}
}
func (f *frontend) handle(raw net.Conn) {
	defer raw.Close()
	select {
	case f.slots <- struct{}{}:
		defer func() { <-f.slots }()
	default:
		return
	}
	_ = raw.SetDeadline(time.Now().Add(4 * time.Second))
	c := tls.Server(raw, f.config)
	if c.Handshake() != nil {
		return
	}
	b, e := readFrame(c, 767)
	if e != nil {
		return
	}
	if string(b) != challengeName && !bytes.HasPrefix(b, []byte(execName)) {
		return
	}
	for _, v := range b {
		if v < 32 || v > 126 {
			return
		}
	}
	d := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, e := d.Dial("unix", f.bridge)
	if e != nil {
		return
	}
	defer conn.Close()
	u, ok := conn.(*net.UnixConn)
	if !ok || f.peer(u) != nil {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(2500 * time.Millisecond))
	if writeFrame(conn, b) != nil {
		return
	}
	out, e := readFrame(conn, 1023)
	if e != nil {
		return
	}
	for _, v := range out {
		if v < 32 || v > 126 {
			return
		}
	}
	_ = writeFrame(c, out)
}

// Explicit device public-key pin. No trust-on-first-use or CA/clock dependency.
func clientTLS(k ed25519.PrivateKey, pin [32]byte) (*tls.Config, error) {
	cert, e := certificate(k)
	if e != nil {
		return nil, e
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true, VerifyConnection: func(s tls.ConnectionState) error {
		if len(s.PeerCertificates) != 1 {
			return errors.New("device certificate required")
		}
		got := sha256.Sum256(s.PeerCertificates[0].RawSubjectPublicKeyInfo)
		if subtle.ConstantTimeCompare(got[:], pin[:]) != 1 {
			return errors.New("device pin mismatch")
		}
		return nil
	}}, nil
}
