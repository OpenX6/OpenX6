package main

import "golang.org/x/sys/unix"

func renameAbsent(src, dst string) error { return unix.RenamexNp(src, dst, unix.RENAME_EXCL) }
