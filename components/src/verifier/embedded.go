package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// The only normalized bytes are the 200-byte owner authorization record inside
// FwCert. Its offset is fixed relative to the canonical three-module layout.
// No pointer or exclusion length is accepted from the authorization itself.
const embeddedOffset int64 = 512
const embeddedSize = manifestSize + 64

func canonicalHash(f *os.File, size, auth int64) ([]byte, error) {
	if auth < 4864 || auth > size-embeddedSize {
		return nil, fail("authorization range")
	}
	h := sha256.New()
	if n, e := io.Copy(h, io.NewSectionReader(f, 0, auth)); e != nil || n != auth {
		return nil, io.ErrUnexpectedEOF
	}
	h.Write(make([]byte, embeddedSize))
	remain := size - auth - embeddedSize
	if n, e := io.Copy(h, io.NewSectionReader(f, auth+embeddedSize, remain)); e != nil || n != remain {
		return nil, io.ErrUnexpectedEOF
	}
	return h.Sum(nil), nil
}
func embeddedLocation(f *os.File) (int64, error) {
	st, e := f.Stat()
	if e != nil {
		return 0, e
	}
	size := st.Size()
	if size < 9984 || size >= 1<<32 {
		return 0, fail("package size")
	}
	head, e := readAt(f, 0, 4864)
	if e != nil {
		return 0, e
	}
	if string(head[:4]) != "MWFI" || u32(head, 4) != 1 || u32(head, 92) != 3 || u32(head, 96) != 0 {
		return 0, fail("unsupported embedded container")
	}
	off, n, cert, chk := int64(u32(head, 4100)), int64(u32(head, 4104)), int64(u32(head, 4356)), int64(u32(head, 4612))
	if off != 4864 || n < 576 || cert != off+n+64 || chk != cert+4160 || chk+320 != size || u32(head, 4096) != 1 || u32(head, 4352) != 4096 || u32(head, 4360) != 4096 || u32(head, 4608) != 4097 || u32(head, 4616) != 256 {
		return 0, fail("noncanonical embedded layout")
	}
	return cert + embeddedOffset, nil
}

// hasOwnerRecord says whether the authorization slot holds one of our records
// at all. Anything else there (zeros, or a vendor certificate's own bytes in a
// factory-style package) is simply not an authorization.
func hasOwnerRecord(slot []byte) bool { return string(slot[:8]) == "X6UPD01\x00" }

// verifyEmbedded accepts a signed package, or (owner policy) a plaintext package
// without any owner record, which is described from its own header. A record that
// is present but does not verify is still refused. The returned offset is -1 for
// an unsigned package: nothing in it is normalized.
func verifyEmbedded(f *os.File, pub []byte, p policy) (manifest, []byte, int64, error) {
	var m manifest
	at, e := embeddedLocation(f)
	if e != nil {
		return m, nil, 0, e
	}
	auth, e := readAt(f, at, embeddedSize)
	if e != nil {
		return m, nil, 0, e
	}
	if hasOwnerRecord(auth) {
		m, e = authenticate(auth, pub, p)
		if e != nil {
			return m, nil, 0, e
		}
		if e = validateMode(f, m, true); e != nil {
			return m, nil, 0, e
		}
		return m, auth, at, nil
	}
	m, rec, e := describeUnsigned(f, pub, p)
	if e != nil {
		return m, nil, 0, e
	}
	return m, rec, -1, nil
}

// verifyEmbeddedSigned is the strict form used when sealing: the package must carry
// a record signed by the owner key, never a null signature or no record.
func verifyEmbeddedSigned(f *os.File, pub []byte, p policy) (manifest, []byte, int64, error) {
	return verifyEmbeddedSignedObserved(f, pub, p, nil)
}
func verifyEmbeddedSignedObserved(f *os.File, pub []byte, p policy, progress digestProgress) (manifest, []byte, int64, error) {
	var m manifest
	at, e := embeddedLocation(f)
	if e != nil {
		return m, nil, 0, e
	}
	auth, e := readAt(f, at, embeddedSize)
	if e != nil {
		return m, nil, 0, e
	}
	m, e = authenticate(auth, pub, p)
	if e != nil {
		return m, nil, 0, e
	}
	if m.Unsigned {
		return m, nil, 0, fail("owner signature required")
	}
	if e = validateModeObserved(f, m, true, progress); e != nil {
		return m, nil, 0, e
	}
	return m, auth, at, nil
}

// describeUnsigned builds the self-describing record for a plaintext package that
// carries no owner authorization. The structural checks are exactly those applied
// to signed packages; the digest covers the file as it is, byte for byte.
func describeUnsigned(f *os.File, pub []byte, p policy) (manifest, []byte, error) {
	var m manifest
	st, e := f.Stat()
	if e != nil {
		return m, nil, e
	}
	head, e := readAt(f, 0, 4864)
	if e != nil {
		return m, nil, e
	}
	project, e := fixed(head[8:24])
	if e != nil || project != p.Project {
		return m, nil, fail("package model mismatch")
	}
	version, e := fixed(head[24:88])
	if e != nil || !versionRE.MatchString(version) {
		return m, nil, fail("package version invalid")
	}
	sum, e := rangeHash(f, 0, st.Size())
	if e != nil {
		return m, nil, e
	}
	rec := make([]byte, embeddedSize)
	copy(rec, "X6UPD01\x00")
	binary.LittleEndian.PutUint32(rec[8:12], 1)
	if compareVersion(version, p.Base) < 0 {
		binary.LittleEndian.PutUint32(rec[12:16], 1)
	}
	binary.LittleEndian.PutUint64(rec[16:24], uint64(st.Size()))
	copy(rec[24:56], sum)
	copy(rec[56:72], project)
	copy(rec[72:104], version)
	copy(rec[104:136], p.Base)
	// The signature stays zero: an explicit null signature, never a forged one.
	m, e = authenticate(rec, pub, p)
	if e != nil {
		return m, nil, e
	}
	if e = validateMode(f, m, false); e != nil {
		return m, nil, e
	}
	return m, rec, nil
}

// StageSingle normalizes a verified single-file input into the existing private
// package.bin + owner-auth interface. The authorization remains cryptographically
// identical; the original legacy verifier must pass before publication.
// It never triggers OTA or changes its hold. The caller must ensure media/OTA idle
// and provide an SD source descriptor from the verified mount. No public key is
// read from the input package. Existing non-identical staging is never overwritten.
func StageSingle(source *os.File, p policy, fault func(string) error) error {
	return stageInput(source, p, fault, func(f *os.File, pub []byte, p policy, _ digestProgress) (manifest, []byte, int64, error) {
		return verifyEmbedded(f, pub, p)
	}, nil)
}
func stageCard(source *os.File, p policy, fault func(string) error) error {
	return stageInput(source, p, fault, inspectCardObserved, nil)
}
func stageInput(source *os.File, p policy, fault func(string) error, inspect func(*os.File, []byte, policy, digestProgress) (manifest, []byte, int64, error), notice *stageNotice) error {
	hit := func(s string) error {
		if fault != nil {
			return fault(s)
		}
		return nil
	}
	parent := filepath.Dir(filepath.Dir(p.Package))
	if e := parents(filepath.Join(parent, "sentinel"), p); e != nil {
		return e
	}
	if filepath.Base(p.Package) != "package.bin" {
		return fail("wrong staging basename")
	}
	lockPath := filepath.Join(parent, ".x6-single-import.lock")
	fd, e := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	lf := os.NewFile(uintptr(fd), lockPath)
	defer lf.Close()
	ls, e := lf.Stat()
	if e != nil {
		return e
	}
	ss, ok := ls.Sys().(*syscall.Stat_t)
	if !ok || !ls.Mode().IsRegular() || ls.Mode().Perm() != 0600 || ss.Uid != p.UID || ss.Nlink != 1 {
		return fail("unsafe import lock")
	}
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fail("import already in progress")
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	st, e := source.Stat()
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() < 4 {
		return fail("invalid input file")
	}
	pub, e := readSmall(p.Public, p, false, 32)
	m, _, _, e := inspect(source, pub, p, notice.reporter(noticeSourceRecheck, 0, 1))
	if e != nil {
		return e
	}
	if e = cleanImportOrphans(parent, p, fmt.Sprintf("%x", m.Digest)); e != nil {
		return e
	}
	final := filepath.Dir(p.Package)
	if _, e = os.Lstat(final); e == nil {
		if _, e = verify(p, p.Package, m.Version, true); e != nil {
			return fail("occupied staging not a verified package")
		}
		f, e := openChecked(p.Package, p, true)
		if e != nil {
			return e
		}
		defer f.Close()
		h, e := rangeHash(f, 0, int64(m.Size))
		if e != nil || string(h) != string(m.Digest[:]) {
			return fail("occupied by a different package")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	if e = ensureImportSpace(parent, uint64(st.Size())+(16<<20), p); e != nil {
		return e
	}
	temp, e := os.MkdirTemp(parent, ".x6-single-import-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	marker, e := os.OpenFile(filepath.Join(temp, ".x6-import-receipt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
	if e != nil {
		return e
	}
	_, e = fmt.Fprintf(marker, "X6 SINGLE IMPORT V2\n%x\n", m.Digest)
	if e == nil {
		e = marker.Sync()
	}
	marker.Close()
	if e != nil {
		return e
	}
	path := filepath.Join(temp, "package.bin")
	out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer out.Close()
	if e = hit("before-copy"); e != nil {
		return e
	}
	if _, e = copyProgress(out, io.NewSectionReader(source, 0, st.Size()), st.Size(), notice, noticeCopy); e != nil {
		return e
	}
	if e = hit("copied"); e != nil {
		return e
	}
	notice.set(noticeVerifying)
	// Reverify the private copy; untrusted input mutations cannot bypass this check.
	passes := 2
	if p.Package == sdStagePackage {
		passes = 3
	}
	copied, auth, at, e := inspect(out, pub, p, notice.reporter(noticeVerifying, 0, passes))
	if e != nil {
		return e
	}
	if copied.Digest != m.Digest {
		return fail("input changed while staging")
	}
	if at >= 0 {
		if _, e = out.WriteAt(make([]byte, embeddedSize), at); e != nil {
			return e
		}
	}
	if e = out.Chmod(0400); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = hit("normalized"); e != nil {
		return e
	}
	af, e := os.OpenFile(path+".owner-auth", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
	if e != nil {
		return e
	}
	_, e = af.Write(auth)
	if e == nil {
		e = af.Sync()
	}
	ce := af.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	staged := p
	staged.Package = path
	if _, e = verifyObserved(staged, path, m.Version, true, notice.reporter(noticeVerifying, 1, passes)); e != nil {
		return fmt.Errorf("legacy check: %w", e)
	}
	d, e := os.Open(temp)
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	if e != nil {
		return e
	}
	if e = hit("before-publish"); e != nil {
		return e
	}
	// Parent is root-controlled and imports are serialized by the held flock. Never
	// overwrite a live maintenance directory, including an empty/symlink directory.
	if _, e = os.Lstat(final); !os.IsNotExist(e) {
		return fail("staging destination appeared")
	}
	if e = os.Rename(temp, final); e != nil {
		return e
	}
	d, e = os.Open(parent)
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	if e != nil {
		return e
	}
	return hit("published")
}
