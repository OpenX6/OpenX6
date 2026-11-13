package main

const nativeSDIndex = "/mnt/sdcard/MISC/dcf_management.db"

// Only an existing regular file descriptor, not a path/dir handle. Both inode
// identities must match; symlinks, hardlinks, renamed/deleted files and the new
// staging image are never exempted. The media service itself is not restarted.
func safeSDIndexReference(exe, name string, mode uint32, links, dev, ino uint64, pathMode uint32, pathLinks, pathDev, pathIno uint64) bool {
	return exe == "/usr/bin/ins_mediad" && name == nativeSDIndex &&
		mode&0170000 == 0100000 && pathMode&0170000 == 0100000 &&
		links == 1 && pathLinks == 1 && ino != 0 && dev == pathDev && ino == pathIno
}
