package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// This receipt describes the exact private copy. It is never read from the SD
// package or accepted as evidence of a signature, release identity or version.
const nativeCopyMagic = "X6COPY1\x00"

func copyManifest(b []byte) (manifest, error) {
	var m manifest
	if len(b) != embeddedSize || string(b[:8]) != nativeCopyMagic || !zero(b[48:]) {
		return m, fail("invalid local copy receipt")
	}
	m.Size = binary.LittleEndian.Uint64(b[8:16])
	if m.Size < 4 || m.Size > 1<<63-1 {
		return m, fail("invalid local copy size")
	}
	copy(m.Digest[:], b[16:48])
	m.Native = true
	m.Unsigned = true
	return m, nil
}
func stagedRecord(p policy) (manifest, error) {
	b, e := readSmall(p.Package+".owner-auth", p, true, embeddedSize)
	if e != nil {
		return manifest{}, e
	}
	if string(b[:8]) == nativeCopyMagic {
		return copyManifest(b)
	}
	pub, e := readSmall(p.Public, p, false, 32)
	if e != nil {
		return manifest{}, e
	}
	return authenticate(b, pub, p)
}
func hashNativeInput(f *os.File) (manifest, []byte, int64, error) {
	return hashNativeInputObserved(f, nil)
}
func hashNativeInputObserved(f *os.File, progress digestProgress) (manifest, []byte, int64, error) {
	var m manifest
	s, e := f.Stat()
	if e != nil {
		return m, nil, -1, e
	}
	if !s.Mode().IsRegular() || s.Size() < 4 {
		return m, nil, -1, fail("firmware is not a regular MWFI file")
	}
	magic, e := readAt(f, 0, 4)
	if e != nil || string(magic) != "MWFI" {
		return m, nil, -1, fail("missing native MWFI magic")
	}
	h := sha256.New()
	if progress != nil {
		progress(0, s.Size())
	}
	writer := &observedHash{Writer: h, report: progress, total: s.Size()}
	// Classify the exact bytes included in the copy digest.
	header, err := readAt(f, 0, headerBytes(s.Size()))
	if err != nil {
		return m, nil, -1, err
	}
	if _, err = writer.Write(header); err != nil {
		return m, nil, -1, err
	}
	if n, e := io.Copy(writer, io.NewSectionReader(f, int64(len(header)), s.Size()-int64(len(header)))); e != nil || n+int64(len(header)) != s.Size() {
		return m, nil, -1, fail("incomplete card read")
	}
	a, e := f.Stat()
	if e != nil {
		return m, nil, -1, e
	}
	live, e := os.Lstat(f.Name())
	if e != nil || !os.SameFile(s, live) || !os.SameFile(s, a) || a.Size() != s.Size() || s.ModTime() != a.ModTime() {
		return m, nil, -1, fail("firmware changed during copy check")
	}
	b := make([]byte, embeddedSize)
	copy(b, nativeCopyMagic)
	binary.LittleEndian.PutUint64(b[8:16], uint64(s.Size()))
	copy(b[16:48], h.Sum(nil))
	m, e = copyManifest(b)
	m.Factory = factoryHeader(header)
	if e == nil && progress != nil {
		progress(s.Size(), s.Size())
	}
	return m, b, -1, e
}
func inspectCard(f *os.File, pub []byte, p policy) (manifest, []byte, int64, error) {
	return inspectCardObserved(f, pub, p, nil)
}
func inspectCardObserved(f *os.File, pub []byte, p policy, progress digestProgress) (manifest, []byte, int64, error) {
	// Only recognized, signed releases use the OpenX6 migration/repair route.
	// All other MWFI files, including foreign signatures, retain every input byte.
	if len(pub) == 32 {
		if _, e := packageRelease(f); e == nil {
			if m, a, at, x := verifyEmbeddedSignedObserved(f, pub, p, progress); x == nil {
				return m, a, at, nil
			}
		}
	}
	return hashNativeInputObserved(f, progress)
}
func cardRelease(f *os.File, m manifest) (string, error) {
	if m.Native {
		return hex.EncodeToString(m.Digest[:]), nil
	}
	return packageRelease(f)
}
func verifyNativeCopy(f *os.File, m manifest, versionStage bool) (int, error) {
	return verifyNativeCopyObserved(f, m, versionStage, nil)
}
func verifyNativeCopyObserved(f *os.File, m manifest, versionStage bool, progress digestProgress) (int, error) {
	var report digestProgress
	if progress != nil {
		report = func(done, total int64) {
			if done < total {
				progress(done, total)
			}
		}
	}
	observed, _, _, e := hashNativeInputObserved(f, report)
	if e != nil || observed.Size != m.Size || !bytes.Equal(observed.Digest[:], m.Digest[:]) {
		return 64, fail("staged native package changed")
	}
	if progress != nil {
		progress(int64(m.Size), int64(m.Size))
	}
	if versionStage {
		return 10, nil
	} // select the native version check's allow-downgrade branch
	return 0, nil
}

// A third-party package need not implement our release ID or completion receipt.
// After the next boot the consumed input can be kept under a non-scanned name.
// Never call this while its installer could still be using SD.
func archiveNativeCard(path, digest string) error {
	if !validID(digest) {
		return fail("invalid consumed digest")
	}
	st, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() {
		return fail("unexpected card entry")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !os.SameFile(st, info) {
		return fail("consumed file changed")
	}
	m, _, _, e := hashNativeInput(f)
	if e != nil || fmt.Sprintf("%x", m.Digest) != digest {
		return fail("consumed native file differs")
	}
	to := path + "." + digest
	if prev, err := os.Lstat(to); err == nil {
		if !prev.Mode().IsRegular() {
			return fail("archive collision")
		}
		ar, err := os.Open(to)
		if err != nil {
			return err
		}
		other, _, _, err := hashNativeInput(ar)
		ar.Close()
		if err != nil || other.Digest != m.Digest {
			return fail("different archive exists")
		}
		return removeConsumedCardInput(f, path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if check, err := os.Lstat(path); err != nil || !os.SameFile(info, check) {
		return fail("consumed entry replaced")
	}
	if e = os.Rename(path, to); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
