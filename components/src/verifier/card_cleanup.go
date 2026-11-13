package main

import (
	"os"
	"path/filepath"
)

// shouldRemoveConsumedInput says whether the card copy the coordinator itself
// consumed may now be deleted. Only after the install it belonged to is
// confirmed complete and the completed stage has been cleaned up, only for the
// hidden consumed name, and only when the file on the card is (by its
// authenticated digest) exactly the package that was installed. A package the
// owner left under its visible name, a resident package, or any other digest is
// never touched.
func shouldRemoveConsumedInput(status, path string, s SDState, cleanedDigest, packageDigest string) bool {
	return status == "left-card-suppressed" && path == consumedCardPath && s.Phase == "completed" &&
		validID(s.Digest) && cleanedDigest == s.Digest && packageDigest == s.Digest
}

// removeConsumedCardInput unlinks the consumed card copy that f was opened
// from. The name must still refer to the very file that was hashed (same
// device and inode); a replaced, linked or non-regular entry is left alone.
func removeConsumedCardInput(f *os.File, path string) error {
	if filepath.Base(path) != filepath.Base(consumedCardPath) {
		return fail("not the consumed card input")
	}
	a, e := f.Stat()
	if e != nil {
		return e
	}
	b, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !b.Mode().IsRegular() || !os.SameFile(a, b) {
		return fail("card entry changed since it was verified")
	}
	if e = os.Remove(path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return nil // removed; directory sync is best effort on exFAT
	}
	_ = d.Sync()
	d.Close()
	return nil
}
