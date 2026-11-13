package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"runtime"
	"syscall"
)

// Cleanup is reached only after embedded owner authorization has passed. Never
// read executable code or cleanup paths from a card/package.
const storageHelperSHA = "ad173b3b598e20c85fcc602a29d30574d6ce2452e5bdc6225e41946f763005bc"

func spaceCheck(required uint64, free func() (uint64, error), reclaim func() error) error {
	before, e := free()
	if e != nil {
		return e
	}
	if before >= required {
		return nil
	}
	cleanupErr := reclaim()
	after, e := free()
	if e != nil {
		return e
	}
	if after < required {
		return fmt.Errorf("insufficient import space: need %d bytes, available %d, shortfall %d; cleanup result: %v", required, after, required-after, cleanupErr)
	}
	// Even if some bytes were freed, a failed safety check must stop this attempt.
	if cleanupErr != nil {
		return fmt.Errorf("space cleanup stopped: %v", cleanupErr)
	}
	return nil
}
func ensureImportSpace(parent string, required uint64, p policy) error {
	free := func() (uint64, error) {
		var s syscall.Statfs_t
		e := syscall.Statfs(parent, &s)
		return uint64(s.Bavail) * uint64(s.Bsize), e
	}
	return spaceCheck(required, free, func() error {
		if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || p != productionPolicy() {
			return fail("cleanup unavailable outside target production policy")
		}
		f, e := openChecked("/usr/local/sbin/x6-storage-preflight", p, false)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil {
			return e
		}
		if fmt.Sprintf("%x", h.Sum(nil)) != storageHelperSHA {
			return fail("space helper hash mismatch")
		}
		b, e := localCommand(60, "/usr/bin/python3", "/usr/local/sbin/x6-storage-preflight", "--required-bytes", fmt.Sprint(required), "--apply")
		fmt.Printf("OpenX6 storage: %s\n", b)
		return e
	})
}
