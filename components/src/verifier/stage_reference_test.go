package main

import "testing"

func TestSDIndexReferenceException(t *testing.T) {
	type args struct {
		exe, name       string
		mode            uint32
		links, dev, ino uint64
		pm              uint32
		pl, pd, pi      uint64
	}
	good := args{"/usr/bin/ins_mediad", nativeSDIndex, 0100600, 1, 42, 7, 0100600, 1, 42, 7}
	check := func(a args) bool {
		return safeSDIndexReference(a.exe, a.name, a.mode, a.links, a.dev, a.ino, a.pm, a.pl, a.pd, a.pi)
	}
	if !check(good) {
		t.Fatal("native regular database rejected")
	}
	for _, which := range []string{"exe", "name", "deleted", "directory", "symlink", "hardlink", "path-hardlink", "inode", "device", "path-symlink", "zero"} {
		a := good
		switch which {
		case "exe":
			a.exe = "/usr/bin/other"
		case "name":
			a.name = "/mnt/sdcard/OpenX6-private-stage-x.img"
		case "deleted":
			a.name += " (deleted)"
		case "directory":
			a.mode = 0040700
		case "symlink":
			a.mode = 0120700
		case "hardlink":
			a.links = 2
		case "path-hardlink":
			a.pl = 2
		case "inode":
			a.pi++
		case "device":
			a.pd++
		case "path-symlink":
			a.pm = 0120700
		case "zero":
			a.ino = 0
			a.pi = 0
		}
		if check(a) {
			t.Fatal("unsafe exemption", which)
		}
	}
}
