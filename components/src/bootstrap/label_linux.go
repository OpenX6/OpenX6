//go:build linux

package main

import (
	"bytes"
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

// Preserve only the old SELinux context, never capabilities or arbitrary xattrs.
// A label read/set/readback failure aborts before the active file is replaced.
func preserveSELinux(old, new *os.File) error {
	name := "security.selinux"
	buf := make([]byte, 4096)
	n, err := unix.Fgetxattr(int(old.Fd()), name, buf)
	if err == unix.ENODATA || err == unix.ENOTSUP {
		return nil
	}
	if err != nil {
		return err
	}
	if n <= 0 || n >= len(buf) {
		return errors.New("SELinux label bounds")
	}
	label := buf[:n]
	if err = unix.Fsetxattr(int(new.Fd()), name, label, 0); err != nil {
		return err
	}
	// label must not alias the readback buffer.
	check := make([]byte, 4096)
	n, err = unix.Fgetxattr(int(new.Fd()), name, check)
	if err != nil || !bytes.Equal(label, check[:n]) {
		return errors.New("SELinux label readback differs")
	}
	return nil
}
