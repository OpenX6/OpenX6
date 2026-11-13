//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const stageRun = "/run/x6-sd-stage"

type stageMount struct {
	ID                                   int
	Dev, Root, Point, Opts, Source, Type string
}

func stageMounts() ([]stageMount, error) {
	b, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return nil, e
	}
	var out []stageMount
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Fields(line)
		sep := -1
		for i, x := range f {
			if x == "-" {
				sep = i
				break
			}
		}
		if len(f) < 10 || sep < 6 || sep+3 >= len(f) {
			return nil, fail("invalid mountinfo")
		}
		id, e := strconv.Atoi(f[0])
		if e != nil {
			return nil, e
		}
		out = append(out, stageMount{id, f[2], f[3], f[4], f[5], f[sep+2], f[sep+1]})
	}
	return out, nil
}
func mountsAt(ms []stageMount, path string) []stageMount {
	var r []stageMount
	for _, m := range ms {
		if m.Point == path {
			r = append(r, m)
		}
	}
	return r
}
func loadStageLease() (StageLease, error) {
	return loadStageLeaseAt(stageLeasePath, productionPolicy())
}
func saveStageLease(s StageLease) error {
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	if _, e = decodeStageLease(b); e != nil {
		return e
	}
	dir := filepath.Dir(stageLeasePath)
	if e = privateDir(dir, productionPolicy()); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".stage-lease-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), stageLeasePath); e != nil {
		return e
	}
	return syncDir(dir)
}
func syncDir(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func noUSBExport() error {
	// Do not start staging while any mass-storage LUN exports a block device.
	paths, e := filepath.Glob("/sys/kernel/config/usb_gadget/*/functions/mass_storage.*/lun.*/file")
	if e != nil {
		return e
	}
	old, _ := filepath.Glob("/sys/class/android_usb/android0/f_mass_storage/lun*/file")
	paths = append(paths, old...)
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil || strings.TrimSpace(string(b)) != "" {
			return fail("USB storage export active or unknown")
		}
	}
	return nil
}
func stagingIdle() error {
	if runtime.GOARCH != "arm64" || os.Geteuid() != 0 {
		return fail("target ARM64 root required")
	}
	if !otaHeld() || !mediaIdle() {
		return fail("staging requires held idle camera")
	}
	return noUSBExport()
}
func currentCardLease(s StageLease) error {
	presence, card, _ := cardObservation("")
	if presence != "present" || card != s.Card {
		return fail("staging card absent or changed")
	}
	return nil
}
func onlyOurSDReaders(dev uint64) error {
	entries, e := os.ReadDir("/proc")
	if e != nil {
		return e
	}
	for _, ent := range entries {
		pid, e := strconv.Atoi(ent.Name())
		if e != nil || pid == os.Getpid() {
			continue
		}
		for _, link := range []string{"cwd", "root"} {
			var st syscall.Stat_t
			x := syscall.Stat("/proc/"+ent.Name()+"/"+link, &st)
			if os.IsNotExist(x) {
				continue
			}
			if x != nil {
				return x
			}
			if uint64(st.Dev) == dev {
				return fail("SD held by process cwd/root")
			}
		}
		fds, e := os.ReadDir("/proc/" + ent.Name() + "/fd")
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		for _, fd := range fds {
			var st syscall.Stat_t
			e = syscall.Stat("/proc/"+ent.Name()+"/fd/"+fd.Name(), &st)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return e
			}
			if uint64(st.Dev) == dev {
				// The native media service keeps this existing regular database open
				// while idle. Its FD cannot traverse directories or name our fresh
				// image. Directory FDs/cwd/root and every other SD FD still refuse.
				proc := "/proc/" + ent.Name()
				exe, xe := os.Readlink(proc + "/exe")
				name, ne := os.Readlink(proc + "/fd/" + fd.Name())
				var db syscall.Stat_t
				de := syscall.Lstat(nativeSDIndex, &db)
				if xe == nil && ne == nil && de == nil && safeSDIndexReference(exe, name,
					uint32(st.Mode), uint64(st.Nlink), uint64(st.Dev), st.Ino,
					uint32(db.Mode), uint64(db.Nlink), uint64(db.Dev), db.Ino) {
					continue
				}
				return fail("SD is held by another process")
			}
		}
	}
	return nil
}
func preparePrivateSD(plan StagePlan, card, digest, boot string, notice *stageNotice) error {
	if e := stagingIdle(); e != nil {
		return e
	}
	if _, e := os.Lstat(stageLeasePath); !os.IsNotExist(e) {
		return fail("previous SD lease must be recovered first")
	}
	ms, e := stageMounts()
	if e != nil {
		return e
	}
	cm := mountsAt(ms, "/mnt/sdcard")
	if len(cm) != 1 || cm[0].Root != "/" || !strings.HasPrefix(cm[0].Source, "/dev/mmcblk") || !member("rw", strings.Split(cm[0].Opts, ",")) {
		return fail("SD mount not an exclusive writable root")
	}
	for _, m := range ms {
		if m.Dev == cm[0].Dev && m.ID != cm[0].ID {
			return fail("alternate SD mount must be removed first")
		}
	}
	if _, e = os.Lstat(filepath.Dir(packagePath)); !os.IsNotExist(e) {
		return fail("occupied destination")
	}
	if _, e = os.Lstat(stageRun); !os.IsNotExist(e) {
		return fail("occupied SD runtime directory")
	}
	s := StageLease{Native: plan.Native, Version: 1, Card: card, Digest: digest, Boot: boot, Bytes: plan.ImageBytes, Phase: "creating", CardMountID: cm[0].ID}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	s.Image = "OpenX6-private-stage-" + hex.EncodeToString(nonce) + ".img"
	if e = currentCardLease(s); e != nil {
		return e
	}
	cardstat, e := os.Stat("/mnt/sdcard")
	if e != nil {
		return e
	}
	if e = onlyOurSDReaders(uint64(cardstat.Sys().(*syscall.Stat_t).Dev)); e != nil {
		return e
	}
	if e = saveStageLease(s); e != nil {
		return e
	}
	if e = os.Mkdir(stageRun, 0700); e != nil {
		return e
	}
	for _, n := range []string{"card", "fs"} {
		if e = os.Mkdir(stageRun+"/"+n, 0700); e != nil {
			return e
		}
	}
	// Keep a private writable view, then freeze the public mount before creation.
	if e = unix.Mount("/mnt/sdcard", stageRun+"/card", "", unix.MS_BIND, ""); e != nil {
		return e
	}
	// The native card is a shared mount. Detach the new writable alias before
	// adding the public self-bind, otherwise it propagates a second alias stack.
	// Only our two new mounts change propagation; the original SD remains intact.
	if e = unix.Mount("", stageRun+"/card", "", unix.MS_PRIVATE, ""); e != nil {
		return e
	}
	if e = unix.Mount("/mnt/sdcard", "/mnt/sdcard", "", unix.MS_BIND, ""); e != nil {
		return e
	}
	if e = unix.Mount("", "/mnt/sdcard", "", unix.MS_PRIVATE, ""); e != nil {
		return e
	}
	if e = unix.Mount("", "/mnt/sdcard", "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NODEV|unix.MS_NOSUID|unix.MS_NOEXEC, ""); e != nil {
		return e
	}
	if e = onlyOurSDReaders(uint64(cardstat.Sys().(*syscall.Stat_t).Dev)); e != nil {
		return e
	}
	fd, e := unix.Open(stageRun+"/card/"+s.Image, unix.O_CREAT|unix.O_EXCL|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	image := os.NewFile(uintptr(fd), s.Image)
	defer image.Close()
	// Write real blocks, never a sparse file whose later allocation can fail mid-OTA.
	if _, e = copyProgress(image, zeroReader{}, int64(s.Bytes), notice, noticeSpace); e != nil {
		return e
	}
	notice.set(noticePreparing)
	if e = image.Sync(); e != nil {
		return e
	}
	st, e := image.Stat()
	if e != nil {
		return e
	}
	raw := st.Sys().(*syscall.Stat_t)
	s.Device = uint64(raw.Dev)
	s.Inode = raw.Ino
	s.Phase = "allocated"
	if e = saveStageLease(s); e != nil {
		return e
	}
	// mkfs receives the already-open file, never an attacker-replaceable SD pathname.
	mkfs := ""
	for _, p := range []string{"/sbin/mkfs.ext4", "/usr/sbin/mkfs.ext4"} {
		if _, e := os.Stat(p); e == nil {
			mkfs = p
			break
		}
	}
	if mkfs == "" {
		return fail("mkfs.ext4 unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mkfs, "-q", "-F", "-m", "0", "-O", "^metadata_csum,^64bit", "-E", "lazy_itable_init=0,lazy_journal_init=0", "/proc/self/fd/3")
	cmd.ExtraFiles = []*os.File{image}
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	if b, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("private mkfs failed: %v %.200s", e, b)
	}
	ctl, e := unix.Open("/dev/loop-control", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(ctl)
	number, e := unix.IoctlRetInt(ctl, unix.LOOP_CTL_GET_FREE)
	if e != nil {
		return e
	}
	loop := fmt.Sprintf("/dev/loop%d", number)
	lfd, e := unix.Open(loop, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(lfd)
	cfg := unix.LoopConfig{Fd: uint32(fd), Info: unix.LoopInfo64{Flags: unix.LO_FLAGS_AUTOCLEAR}}
	copy(cfg.Info.File_name[:], []byte(s.Image))
	if e = unix.IoctlLoopConfigure(lfd, &cfg); e != nil {
		return e
	}
	if e = unix.Mount(loop, stageRun+"/fs", "ext4", unix.MS_NODEV|unix.MS_NOSUID|unix.MS_NOEXEC, ""); e != nil {
		return e
	}
	if e = os.Chmod(stageRun+"/fs", 0700); e != nil {
		return e
	}
	s.Phase = "mounted"
	return saveStageLease(s)
}

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) { clear(b); return len(b), nil }
func leaseLoop(s StageLease) (string, error) {
	for i := 0; i < 256; i++ {
		p := fmt.Sprintf("/dev/loop%d", i)
		fd, e := unix.Open(p, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if e != nil {
			continue
		}
		v, e := unix.IoctlLoopGetStatus64(fd)
		unix.Close(fd)
		if e == nil && v.Device == s.Device && v.Inode == s.Inode && strings.TrimRight(string(v.File_name[:]), "\x00") == s.Image {
			return p, nil
		}
	}
	return "", nil
}
func publishPrivateSD() error {
	if e := stagingIdle(); e != nil {
		return e
	}
	s, e := loadStageLease()
	if e != nil {
		return e
	}
	if e = currentCardLease(s); e != nil {
		return e
	}
	if s.Phase != "mounted" {
		return fail("unprepared SD lease")
	}
	if _, e = verify(policy{sdStagePackage, publicPath, "/", "c9", "1.2.36", 0}, sdStagePackage, "", false); e != nil {
		return e
	}
	if e = os.Mkdir(filepath.Dir(packagePath), 0700); e != nil {
		return e
	}
	if e = unix.Mount(filepath.Dir(sdStagePackage), filepath.Dir(packagePath), "", unix.MS_BIND, ""); e != nil {
		return e
	}
	s.Phase = "published"
	return saveStageLease(s)
}

// Never mount a persisted image to recover it. Before handoff, discard only this
// receipt's staging image and rebuild from the authenticated card package. After
// handoff, wait until the coordinator proves successful new-slot boot and roots.
func recoverPrivateSD(state SDState) error {
	s, e := loadStageLease()
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !stageMayRecover(state.Phase) {
		return fail("private staging outcome pending; no cleanup or retry")
	}
	if e = stagingIdle(); e != nil {
		return e
	}
	if e = currentCardLease(s); e != nil {
		return e
	}
	ms, e := stageMounts()
	if e != nil {
		return e
	}
	loop, e := leaseLoop(s)
	if e != nil {
		return e
	}
	loopDev := ""
	if loop != "" {
		st, e := os.Stat(loop)
		if e != nil {
			return e
		}
		dev := uint64(st.Sys().(*syscall.Stat_t).Rdev)
		loopDev = fmt.Sprintf("%d:%d", unix.Major(dev), unix.Minor(dev))
	}
	for _, path := range []string{filepath.Dir(packagePath), stageRun + "/fs"} {
		m := mountsAt(ms, path)
		if len(m) > 1 {
			return fail("ambiguous private mounts")
		}
		if len(m) == 1 {
			if loopDev == "" || m[0].Dev != loopDev || m[0].Type != "ext4" || (path == filepath.Dir(packagePath) && m[0].Root != "/work") {
				return fail("foreign private mount")
			}
			if e = unix.Unmount(path, 0); e != nil {
				return e
			}
		}
	}
	// AUTOCLEAR releases only our loop after the last mount reference disappears.
	if loop != "" {
		fd, e := unix.Open(loop, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if e != nil {
			return e
		}
		v, x := unix.IoctlLoopGetStatus64(fd)
		unix.Close(fd)
		if x == nil && v.Inode == s.Inode {
			return fail("private loop still busy")
		}
	}
	pub := mountsAt(ms, "/mnt/sdcard")
	private := mountsAt(ms, stageRun+"/card")
	if len(private) > 1 {
		return fail("ambiguous private card mount")
	}
	if len(pub) > 1 {
		if len(pub) != 2 || pub[0].ID != s.CardMountID || pub[1].Dev != pub[0].Dev || pub[1].Root != pub[0].Root {
			return fail("foreign SD mount stack")
		}
		if e = unix.Unmount("/mnt/sdcard", 0); e != nil {
			return e
		}
	}
	if len(private) == 1 {
		if len(pub) == 0 || private[0].Dev != pub[0].Dev || private[0].Root != pub[0].Root {
			return fail("foreign private card mount")
		}
		if e = unix.Unmount(stageRun+"/card", 0); e != nil {
			return e
		}
	}
	// A reboot naturally removed all runtime mounts. Only the random name in the
	// root-private durable receipt may be unlinked; never recurse through the card.
	dir, e := os.Open("/mnt/sdcard")
	if e != nil {
		return e
	}
	defer dir.Close()
	var st unix.Stat_t
	e = unix.Fstatat(int(dir.Fd()), s.Image, &st, unix.AT_SYMLINK_NOFOLLOW)
	if e == nil {
		if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
			return fail("unsafe residual image")
		}
		if e = unix.Unlinkat(int(dir.Fd()), s.Image, 0); e != nil {
			return e
		}
		if e = dir.Sync(); e != nil {
			return e
		}
	} else if e != unix.ENOENT {
		return e
	}
	for _, p := range []string{filepath.Dir(packagePath), stageRun + "/fs", stageRun + "/card", stageRun} {
		if e = os.Remove(p); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	if e = os.Remove(stageLeasePath); e != nil {
		return e
	}
	return syncDir(filepath.Dir(stageLeasePath))
}

func checkPrivateSD() error {
	s, e := loadStageLease()
	if os.IsNotExist(e) {
		return noUSBExport()
	}
	if e != nil {
		return e
	}
	if s.Phase != "published" {
		return fail("private stage not published")
	}
	if e = currentCardLease(s); e != nil {
		return e
	}
	if e = noUSBExport(); e != nil {
		return e
	}
	ms, e := stageMounts()
	if e != nil {
		return e
	}
	pub := mountsAt(ms, "/mnt/sdcard")
	dest := mountsAt(ms, filepath.Dir(packagePath))
	fs := mountsAt(ms, stageRun+"/fs")
	if len(pub) != 2 || pub[0].ID != s.CardMountID || !member("ro", strings.Split(pub[1].Opts, ",")) || len(dest) != 1 || len(fs) != 1 || dest[0].Dev != fs[0].Dev || dest[0].Root != "/work" {
		return fail("private staging mount changed")
	}
	return nil
}
