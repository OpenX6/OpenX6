// Host-only detached authorization. This tool is NEVER included in the overlay.
// It does not install, upload, stage on a camera, or authorize removing locks.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

func checked(path string, private bool) (*os.File, error) {
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	s, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || !s.Mode().IsRegular() || st.Uid != uint32(os.Geteuid()) || s.Mode().Perm()&0022 != 0 || (private && s.Mode().Perm()&0077 != 0) {
		f.Close()
		return nil, errors.New("unsafe local file")
	}
	return f, nil
}
func small(path string, private bool, limit int64) ([]byte, error) {
	f, e := checked(path, private)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > int(limit) {
		return nil, errors.New("oversize local file")
	}
	return b, e
}
func field(b []byte, o, n int, s string) { copy(b[o:o+n], s) }
func fixed(b []byte) (string, error) {
	n := bytes.IndexByte(b, 0)
	if n < 0 || !bytes.Equal(b[n:], make([]byte, len(b)-n)) {
		return "", errors.New("bad header string")
	}
	return string(b[:n]), nil
}
func lower(v string) bool {
	a := strings.Split(v, ".")
	base := []int{1, 2, 36, 0}
	for i := 0; i < 4; i++ {
		x := 0
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if x < base[i] {
			return true
		}
		if x > base[i] {
			return false
		}
	}
	return false
}
func run(args []string) error {
	fs := flag.NewFlagSet("x6-owner-authorize", flag.ContinueOnError)
	pkg := fs.String("package", "", "local plaintext MWFI file")
	key := fs.String("key", "", "host-only OpenSSH Ed25519 private key")
	pub := fs.String("expected-public", "", "exact 32-byte public key from candidate overlay")
	out := fs.String("out", "", "new detached authorization file; refuses overwrite")
	expected := fs.String("sha256", "", "reviewed whole-file SHA-256; mandatory")
	down := fs.Bool("allow-downgrade", false, "sign explicit permission for this exact package only")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 || *pkg == "" || *key == "" || *pub == "" || *out == "" {
		return errors.New("package, key, expected-public, sha256 and out are required")
	}
	reviewed, e := hex.DecodeString(*expected)
	if e != nil || len(reviewed) != 32 {
		return errors.New("reviewed sha256 required")
	}
	f, e := checked(*pkg, false)
	if e != nil {
		return e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return e
	}
	if before.Size() < 9984 || before.Size() >= 1<<32 {
		return errors.New("package size out of range")
	}
	h := make([]byte, 4864)
	if _, e = f.ReadAt(h, 0); e != nil {
		return e
	}
	u := func(o int) uint32 { return binary.LittleEndian.Uint32(h[o : o+4]) }
	project, e := fixed(h[8:24])
	if e != nil {
		return e
	}
	version, e := fixed(h[24:88])
	if e != nil {
		return e
	}
	if string(h[:4]) != "MWFI" || u(4) != 1 || u(92) != 3 || u(96) != 0 || project != "c9" || u(4096) != 1 || u(4352) != 4096 || u(4608) != 4097 {
		return errors.New("requires c9 plaintext format 1, one host module plus FwCert/FwChk")
	}
	if !regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.(0|[1-9][0-9]{0,5})){2,3}$`).MatchString(version) {
		return errors.New("noncanonical version")
	}
	if lower(version) && !*down {
		return errors.New("explicit allow-downgrade required")
	}
	hash := sha256.New()
	n, e := io.Copy(hash, f)
	if e != nil {
		return e
	}
	sum := hash.Sum(nil)
	after, e := f.Stat()
	if e != nil {
		return e
	}
	if n != before.Size() || after.Size() != before.Size() || after.ModTime() != before.ModTime() || !bytes.Equal(sum, reviewed) {
		return errors.New("reviewed package hash/size changed")
	}
	kb, e := small(*key, true, 16384)
	if e != nil {
		return e
	}
	k, e := ssh.ParseRawPrivateKey(kb)
	if e != nil {
		return e
	}
	priv, ok := k.(*ed25519.PrivateKey)
	if !ok {
		return errors.New("Ed25519 key required")
	}
	public, e := small(*pub, false, 32)
	if e != nil {
		return e
	}
	if len(public) != 32 || !bytes.Equal(public, priv.Public().(ed25519.PublicKey)) {
		return errors.New("key does not match candidate public key")
	}
	m := make([]byte, 136)
	copy(m, "X6UPD01\x00")
	binary.LittleEndian.PutUint32(m[8:12], 1)
	if *down {
		binary.LittleEndian.PutUint32(m[12:16], 1)
	}
	binary.LittleEndian.PutUint64(m[16:24], uint64(n))
	copy(m[24:56], sum)
	field(m, 56, 16, project)
	field(m, 72, 32, version)
	field(m, 104, 32, "1.2.36")
	data := append(m, ed25519.Sign(*priv, append([]byte("X6 OWNER MWFI V1\x00"), m...))...)
	dst, e := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = dst.Write(data); e != nil {
		dst.Close()
		return e
	}
	if e = dst.Sync(); e != nil {
		dst.Close()
		return e
	}
	if e = dst.Close(); e != nil {
		return e
	}
	fmt.Printf("Detached authorization created for %s; SHA-256 %x; downgrade=%t. No camera operation performed.\n", version, sum, *down)
	return nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
