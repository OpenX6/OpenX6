package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// This entry only authenticates and stages a file. It NEVER releases either OTA
// hold or requests an upgrade. The install coordinator is a separate boundary.
func importSD(path string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 || path != "/mnt/sdcard/Insta360X6FW.bin" {
		return fail("unsupported import target/path")
	}
	for _, p := range []string{"/run/x6-owner/vendor-ota-enabled", "/run/x6-owner/validated-boot"} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			return fail("OTA is not held")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/bin/systemctl", "show", "ins_otad.service", "-p", "MainPID", "--value").Output()
	if e != nil || strings.TrimSpace(string(b)) != "0" {
		return fail("OTAD may be active")
	}
	// Check a real distinct SD mount. The file itself is untrusted until signature
	// verification; mount names do not grant authority and no card scripts execute.
	b, e = os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return e
	}
	mounted := false
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 6 && f[4] == "/mnt/sdcard" {
			if mounted {
				return fail("ambiguous SD mount")
			}
			mounted = true
		}
	}
	if !mounted {
		return fail("SD not mounted")
	}
	for _, d := range []string{"/mnt", filepath.Dir(path)} {
		s, e := os.Lstat(d)
		if e != nil || !s.IsDir() {
			return fail("SD path not directory")
		}
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	return stageCard(f, productionPolicy(), nil)
}
