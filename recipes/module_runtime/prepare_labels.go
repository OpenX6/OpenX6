// Boot-time migration for the immutable menu manifest. This helper never opens
// preference contents. O_PATH pins the inode checked before a label change.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

var names string // Set at build time from validated menu IDs.

const (
	pathOnly = 0x200000 // Linux arm64 O_PATH
	attr     = "security.selinux"
	target   = "system_u:object_r:initrc_state_t:s0"
)

func directory(parent int, name string, private bool) (int, error) {
	fd, err := syscall.Openat(parent, name, pathOnly|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	var st syscall.Stat_t
	if err = syscall.Fstat(fd, &st); err != nil || st.Uid != 0 || st.Mode&0022 != 0 || (private && st.Mode&07777 != 0700) {
		syscall.Close(fd)
		return -1, errors.New("unsafe preference directory")
	}
	return fd, nil
}

func label(fd int) error {
	// Follow only our own pinned descriptor, never a name supplied by a caller.
	path := "/proc/self/fd/" + strconv.Itoa(fd)
	var buf [256]byte
	n, err := syscall.Getxattr(path, attr, buf[:])
	if err != nil {
		return err
	}
	old := string(bytes.TrimSuffix(buf[:n], []byte{0}))
	if old == target {
		return nil
	}
	if old != "system_u:object_r:data_t:s0" {
		return errors.New("unexpected starting context: " + old)
	}
	return syscall.Setxattr(path, attr, []byte(target+"\x00"), 0)
}

func prepare() error {
	if os.Geteuid() != 0 || len(os.Args) != 1 || names == "" {
		return errors.New("invalid invocation")
	}
	data, err := directory(-100, "/data", false)
	if err != nil {
		return err
	}
	defer syscall.Close(data)
	parent, err := directory(data, "x6-owner-features", false)
	if err != nil {
		return err
	}
	defer syscall.Close(parent)
	dir, err := directory(parent, "extensions-v1", true)
	if err != nil {
		return err
	}
	defer syscall.Close(dir)
	files := []int{}
	defer func() {
		for _, fd := range files {
			syscall.Close(fd)
		}
	}()
	for _, name := range strings.Split(names, ",") {
		for _, suffix := range []string{"", ".next"} {
			fd, err := syscall.Openat(dir, name+suffix, pathOnly|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
			if err == syscall.ENOENT {
				continue
			}
			if err != nil {
				return err
			}
			files = append(files, fd)
			var st syscall.Stat_t
			if err = syscall.Fstat(fd, &st); err != nil {
				return err
			}
			if st.Uid != 0 || st.Nlink != 1 || st.Mode != syscall.S_IFREG|0600 {
				return errors.New("unsafe preference inode: " + name + suffix)
			}
		}
	}
	// Relabel files first. Interruption is idempotent; the publisher starts only on success.
	for _, fd := range files {
		if err := label(fd); err != nil {
			return err
		}
	}
	return label(dir)
}

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
