// Owner verifier/importer and locked SD coordinator. Verification commands remain read-only.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"openx6.local/components/holder"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const packagePath = "/data/x6-owner-update/package.bin"
const publicPath = holder.Dir + "/" + holder.Update
const manifestSize = 136
const domain = "X6 OWNER MWFI V1\x00"

// Optional additional signer for attributed packages. An inserted card may also
// contain a compatible unsigned plaintext package (see verifyEmbedded).
var projectUpdatePublic ed25519.PublicKey

var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.(0|[1-9][0-9]{0,5})){2,3}$`)

type policy struct {
	Package, Public, Boundary, Project, Base string
	UID                                      uint32
}
type manifest struct {
	Size                   uint64
	Digest                 [32]byte
	Project, Version, Base string
	Downgrade              bool
	Factory                bool // encrypted X6 distribution; local classification only
	Native                 bool // byte-preserving transport; format validation belongs to the native updater
	Unsigned               bool // self-described record with a null signature
}

func productionPolicy() policy     { return policy{packagePath, publicPath, "/", "c9", "1.2.36", 0} }
func fail(s string) error          { return errors.New(s) }
func u32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off : off+4]) }
func zero(b []byte) bool           { return bytes.Equal(b, make([]byte, len(b))) }
func compareVersion(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 4; i++ {
		x, y := 0, 0
		if i < len(aa) {
			x, _ = strconv.Atoi(aa[i])
		}
		if i < len(bb) {
			y, _ = strconv.Atoi(bb[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
func fixed(b []byte) (string, error) {
	n := bytes.IndexByte(b, 0)
	if n < 0 || !zero(b[n:]) {
		return "", fail("noncanonical string")
	}
	for _, c := range b[:n] {
		if c < 32 || c > 126 {
			return "", fail("invalid string")
		}
	}
	return string(b[:n]), nil
}

// All parent paths are checked up to a trusted boundary. Production boundary
// is /; only tests use an isolated temporary tree owned by the test UID.
func parents(path string, p policy) error {
	d := filepath.Dir(path)
	for {
		if d != p.Boundary && p.Boundary != "/" && !bytes.HasPrefix([]byte(d), []byte(p.Boundary+"/")) {
			return fail("outside trust boundary")
		}
		st, e := os.Lstat(d)
		if e != nil {
			return e
		}
		s, ok := st.Sys().(*syscall.Stat_t)
		if !st.IsDir() || !ok || s.Uid != p.UID || st.Mode().Perm()&0022 != 0 {
			return fail("unsafe ancestor")
		}
		if d == p.Boundary {
			return nil
		}
		next := filepath.Dir(d)
		if next == d {
			return fail("trust boundary missing")
		}
		d = next
	}
}
func openChecked(path string, p policy, private bool) (*os.File, error) {
	if e := parents(path, p); e != nil {
		return nil, e
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	st, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || !ok || s.Uid != p.UID || s.Nlink != 1 || st.Mode().Perm()&0022 != 0 || (private && st.Mode().Perm() != 0400) {
		f.Close()
		return nil, fail("unsafe owner file")
	}
	return f, nil
}
func readSmall(path string, p policy, private bool, n int) ([]byte, error) {
	if path == publicPath {
		b, e := holder.Read("/", holder.Update)
		if e != nil {
			return nil, e
		}
		if len(b) != n {
			return nil, fail("wrong holder key size")
		}
		return b, nil
	}
	f, e := openChecked(path, p, private)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(n+1)))
	if e != nil {
		return nil, e
	}
	if len(b) != n {
		return nil, fail("wrong file size")
	}
	return b, nil
}

func authenticate(b, pub []byte, p policy) (manifest, error) {
	var m manifest
	if len(b) != manifestSize+ed25519.SignatureSize || len(pub) != ed25519.PublicKeySize {
		return m, fail("bad authorization length")
	}
	message := append([]byte(domain), b[:manifestSize]...)
	valid := ed25519.Verify(pub, message, b[manifestSize:])
	if !valid && len(projectUpdatePublic) == ed25519.PublicKeySize {
		valid = ed25519.Verify(projectUpdatePublic, message, b[manifestSize:])
	}
	if !valid {
		if !zero(b[manifestSize:]) {
			return m, fail("owner signature invalid")
		}
		m.Unsigned = true
	}
	if string(b[:8]) != "X6UPD01\x00" || u32(b, 8) != 1 || u32(b, 12) > 1 {
		return m, fail("unsupported authorization")
	}
	m.Downgrade = u32(b, 12) == 1
	m.Size = binary.LittleEndian.Uint64(b[16:24])
	copy(m.Digest[:], b[24:56])
	var e error
	if m.Project, e = fixed(b[56:72]); e != nil {
		return m, e
	}
	if m.Version, e = fixed(b[72:104]); e != nil {
		return m, e
	}
	if m.Base, e = fixed(b[104:136]); e != nil {
		return m, e
	}
	if m.Project != p.Project || m.Base != p.Base || !versionRE.MatchString(m.Version) || !versionRE.MatchString(m.Base) {
		return m, fail("authorization model/base/version mismatch")
	}
	if m.Size < 9984 || m.Size >= 1<<32 {
		return m, fail("package size out of range")
	}
	if m.Unsigned {
		// Nobody vouched for this record, so its downgrade bit is not the owner's
		// word; the permission follows from the versions themselves, as the factory
		// updater had no owner gate at all.
		m.Downgrade = compareVersion(m.Version, m.Base) < 0
	}
	if !m.Downgrade && compareVersion(m.Version, m.Base) < 0 {
		return m, fail("downgrade not authorized")
	}
	return m, nil
}
func readAt(f *os.File, off int64, n int) ([]byte, error) {
	b := make([]byte, n)
	_, e := f.ReadAt(b, off)
	return b, e
}
func rangeHash(f *os.File, a, b int64) ([]byte, error) {
	if a < 0 || b < a {
		return nil, fail("invalid digest range")
	}
	h := sha256.New()
	n, e := io.Copy(h, io.NewSectionReader(f, a, b-a))
	if e != nil {
		return nil, e
	}
	if n != b-a {
		return nil, io.ErrUnexpectedEOF
	}
	return h.Sum(nil), nil
}
func validate(f *os.File, m manifest) error { return validateMode(f, m, false) }
func validateMode(f *os.File, m manifest, embedded bool) error {
	return validateModeObserved(f, m, embedded, nil)
}
func validateModeObserved(f *os.File, m manifest, embedded bool, progress digestProgress) error {
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if uint64(st.Size()) != m.Size {
		return fail("signed size mismatch")
	}
	head, e := readAt(f, 0, 4864)
	if e != nil {
		return e
	}
	if string(head[:4]) != "MWFI" || u32(head, 4) != 1 || u32(head, 92) != 3 || u32(head, 96) != 0 {
		return fail("not owner plaintext format 1")
	}
	project, e := fixed(head[8:24])
	if e != nil || project != m.Project {
		return fail("package model mismatch")
	}
	ver, e := fixed(head[24:88])
	if e != nil || ver != m.Version {
		return fail("package version mismatch")
	}
	off, n := int64(u32(head, 4100)), int64(u32(head, 4104))
	cert, chk := int64(u32(head, 4356)), int64(u32(head, 4612))
	if u32(head, 4096) != 1 || off != 4864 || n < 576 || n > int64(m.Size)-4864 ||
		u32(head, 4352) != 4096 || u32(head, 4360) != 4096 || cert != off+n+64 ||
		u32(head, 4608) != 4097 || u32(head, 4616) != 256 || chk != cert+4160 || chk+320 != int64(m.Size) {
		return fail("module layout invalid")
	}
	mod, e := readAt(f, off, 576)
	if e != nil {
		return e
	}
	name, e := fixed(mod[68:132])
	if e != nil || name != "qcs8625_ota.zip" {
		return fail("wrong module")
	}
	kind, e := fixed(mod[136:152])
	if e != nil || kind != "full" {
		return fail("not full main module")
	}
	target, e := fixed(mod[152:184])
	if e != nil || target != "host_8625" {
		return fail("wrong host target")
	}
	mv, e := fixed(mod[184:248])
	if e != nil || mv != m.Version {
		return fail("module version mismatch")
	}
	if !zero(mod[:64]) || u32(mod, 64) != 1 || u32(mod, 132) != 4 {
		return fail("MCU/non-host module refused")
	}
	pad, e := readAt(f, off+n, 64)
	if e != nil || !zero(pad) {
		return fail("boundary pad invalid")
	}
	tail, e := readAt(f, cert, 4480)
	if e != nil {
		return e
	}
	if !zero(tail[:64]) || string(tail[64:68]) != "CFSI" || u32(tail, 68) != 1 || u32(tail, 72) != 0 ||
		!zero(tail[4160:4224]) || string(tail[4224:4228]) != "CWFI" || u32(tail, 4228) != 2 {
		return fail("FwCert/FwChk invalid")
	}
	if !bytes.Equal(tail[4232:4296], tail[4296:4360]) {
		return fail("FwChk copies disagree")
	}
	if e = scanContent(f, int64(m.Size), off+576, off+n, cert, embedded, mod[264:328], tail[4232:4296], m.Digest[:], progress); e != nil {
		return e
	}
	// Detect concurrent replacement/modification as well as unsafe path changes.
	after, e := f.Stat()
	if e != nil {
		return e
	}
	live, e := os.Lstat(f.Name())
	if e != nil {
		return e
	}
	if !os.SameFile(st, after) || !os.SameFile(st, live) || st.Size() != after.Size() || st.ModTime() != after.ModTime() {
		return fail("file changed during verification")
	}
	if progress != nil {
		progress(st.Size(), st.Size())
	}
	return nil
}

func verify(p policy, path, version string, versionStage bool) (int, error) {
	return verifyObserved(p, path, version, versionStage, nil)
}
func verifyObserved(p policy, path, version string, versionStage bool, progress digestProgress) (int, error) {
	if path != p.Package || filepath.Clean(path) != path {
		return 64, fail("not private maintenance path")
	}
	dir, e := os.Lstat(filepath.Dir(path))
	if e != nil {
		return 64, e
	}
	if !dir.IsDir() || dir.Mode().Perm() != 0700 {
		return 64, fail("staging directory not private")
	}
	m, e := stagedRecord(p)
	if e != nil {
		return 64, e
	}
	if versionStage && !m.Native && version != m.Version {
		return 64, fail("native metadata version mismatch")
	}
	f, e := openChecked(path, p, true)
	if e != nil {
		return 64, e
	}
	defer f.Close()
	if m.Native {
		return verifyNativeCopyObserved(f, m, versionStage, progress)
	}
	if e = validateModeObserved(f, m, false, progress); e != nil {
		return 64, e
	}
	if versionStage && m.Downgrade {
		return 10, nil
	}
	return 0, nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "coordinate" {
		if e := runCoordinator(); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(64)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "seal-offline" {
		if e := sealDirectory(os.Args[2]); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(64)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "import" {
		time.AfterFunc(600*time.Second, func() { os.Exit(64) })
		if e := importSD(os.Args[2]); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(64)
		}
		return
	}
	// Child of the patched daemon. No env-based key/path/config overrides.
	if os.Geteuid() != 0 || len(os.Args) < 3 {
		os.Exit(64)
	}
	time.AfterFunc(120*time.Second, func() { os.Exit(64) })
	stage := os.Args[1]
	v := ""
	if stage == "entry" && len(os.Args) != 3 {
		os.Exit(64)
	}
	if stage == "version" {
		if len(os.Args) != 4 {
			os.Exit(64)
		}
		v = os.Args[3]
	} else if stage != "entry" {
		os.Exit(64)
	}
	code, e := verify(productionPolicy(), os.Args[2], v, stage == "version")
	if e != nil {
		fmt.Fprintln(os.Stderr, "owner update authorization denied:", e)
	}
	os.Exit(code)
}
