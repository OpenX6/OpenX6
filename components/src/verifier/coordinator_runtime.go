package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"openx6.local/components/holder"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed coordinator-policy.json
var coordinatorPolicyBytes []byte

type SDPolicy struct {
	Release          string
	AutomaticEnabled bool
	Files            map[string][]string
	OwnerTrust       map[string]string
}

const sdJournal = "/data/x6-sd-update/state.json"
const releaseOffset int64 = 768

func packageRelease(f *os.File) (string, error) {
	at, e := embeddedLocation(f)
	if e != nil {
		return "", e
	}
	b, e := readAt(f, at-embeddedOffset+releaseOffset, 40)
	if e != nil {
		return "", e
	}
	if string(b[:8]) != "X6REL01\x00" {
		return "", fail("missing signed release identity")
	}
	id := hex.EncodeToString(b[8:])
	if !validID(id) {
		return "", fail("release identity")
	}
	return id, nil
}

type limitedOutput struct{ b []byte }

func (b *limitedOutput) Write(x []byte) (int, error) {
	if len(b.b)+len(x) > 65536 {
		return 0, fail("command output limit")
	}
	b.b = append(b.b, x...)
	return len(x), nil
}
func localCommand(seconds int, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	b := &limitedOutput{}
	cmd.Stdout = b
	cmd.Stderr = &limitedOutput{}
	e := cmd.Run()
	return b.b, e
}
func localQuery(typ string) (map[string]any, error) {
	q, _ := json.Marshal(map[string]string{"cmd": "share_state", "action": "get", "type": typ})
	b, e := localCommand(8, "/usr/bin/ins_bll_cli", string(q))
	if e != nil {
		return nil, e
	}
	var d struct {
		Ret  string
		Data map[string]any
	}
	if e = json.Unmarshal(b, &d); e != nil || d.Ret != "ok" {
		return nil, fail("query rejected")
	}
	return d.Data, nil
}
func readTrim(path string) (string, error) {
	b, e := os.ReadFile(path)
	return strings.TrimSpace(string(b)), e
}
func fileDigest(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func member(s string, a []string) bool {
	for _, v := range a {
		if s == v {
			return true
		}
	}
	return false
}
func holderReady(cfg SDPolicy) bool {
	p := productionPolicy()
	if len(cfg.OwnerTrust) != 0 {
		return false
	}
	for _, role := range []string{holder.SSH, holder.GUI, holder.Factory, holder.Update, holder.ADB} {
		if _, err := holder.Read("/", role); err != nil {
			return false
		}
	}
	for _, path := range []string{"/data/x6-owner-ssh/host_ed25519", "/data/x6-owner-gui/server_ed25519", "/data/x6-owner-gui/server_ed25519.pub"} {
		f, e := openChecked(path, p, false)
		if e != nil {
			return false
		}
		s, e := f.Stat()
		f.Close()
		if e != nil || s.Mode().Perm() != 0600 || s.Size() == 0 {
			return false
		}
	}
	return true
}

// Successful installation needs a working owner command channel, not an
// unchanged feature GUI. Slot, release and device identity are checked by Step.
func localRootReady(cfg SDPolicy) bool {
	return completionRootReady(cfg, holderReady(cfg), localCommand, os.ReadFile, fileDigest, processMatches)
}
func completionRootReady(cfg SDPolicy, enrolled bool, command func(int, ...string) ([]byte, error), read func(string) ([]byte, error), digest func(string) (string, error), matches func(int, string, SDPolicy) bool) bool {
	if !enrolled {
		return false
	}
	const helper = "/usr/local/sbin/x6-owner-maintenance"
	b, e := command(4, "/bin/systemctl", "show", "x6-owner-maintenance.service", "-p", "MainPID", "--value")
	if e != nil {
		return false
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(b)))
	if e != nil || pid < 2 {
		return false
	}
	status, e := read(fmt.Sprintf("/proc/%d/status", pid))
	if e != nil {
		return false
	}
	root := false
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			f := strings.Fields(line)
			root = len(f) == 5 && f[1] == "0" && f[2] == "0" && f[3] == "0" && f[4] == "0"
		}
	}
	if !root || !matches(pid, helper, cfg) {
		return false
	}
	h, e := digest(helper)
	if e != nil || !member(h, cfg.Files[helper]) {
		return false
	}
	// The existing probe pins the device host key, checks root uid, and verifies
	// acceptance of the installed owner public key without exporting a secret.
	_, e = command(6, helper, "--probe-root", "--listen", "127.0.0.1:2222")
	return e == nil
}
func identityDigest() (string, error) {
	h := sha256.New()
	for _, p := range []string{"/data/x6-owner-ssh/host_ed25519", "/data/x6-owner-gui/server_ed25519", "/data/x6-owner-gui/server_ed25519.pub"} {
		f, e := openChecked(p, productionPolicy(), false)
		if e != nil {
			return "", e
		}
		h.Write([]byte(p))
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func currentSlot(cfg SDPolicy) (string, error) {
	h, e := fileDigest("/sbin/abctl")
	if e != nil || !member(h, cfg.Files["/sbin/abctl"]) {
		return "", fail("unknown slot reader")
	}
	b, e := localCommand(5, "/sbin/abctl", "--boot_slot")
	if e != nil {
		return "", e
	}
	s := strings.TrimSpace(string(b))
	if s != "_a" && s != "_b" {
		return "", fail("invalid active slot")
	}
	return s, nil
}
func mediaIdle() bool {
	d, e := localQuery("media")
	if e != nil {
		return false
	}
	r, ok := d["record"].(map[string]any)
	if !ok || r["record_status"] != "RecordIdle" {
		return false
	}
	c, ok := d["capture"].(map[string]any)
	return ok && c["capture_status"] == "CaptureIdle"
}
func otaHeld() bool {
	for _, p := range []string{"/run/x6-owner/vendor-ota-enabled", "/run/x6-owner/validated-boot"} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			return false
		}
	}
	b, e := localCommand(4, "/bin/systemctl", "show", "ins_otad.service", "-p", "MainPID", "--value")
	return e == nil && strings.TrimSpace(string(b)) == "0"
}

// Discover the physical card through the mounted block device, not Wi-Fi or USB
// connectivity. A live card with its filesystem unmounted remains present.
func cardObservation(previousDisk string) (presence, card, disk string) {
	b, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return "unknown", "", previousDisk
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 6 || f[4] != "/mnt/sdcard" {
			continue
		}
		real, e := filepath.EvalSymlinks("/sys/dev/block/" + f[2])
		if e != nil {
			return "unknown", "", previousDisk
		}
		if _, e = os.Stat(filepath.Join(real, "partition")); e == nil {
			real = filepath.Dir(real)
		}
		name := filepath.Base(real)
		if !strings.HasPrefix(name, "mmcblk") || strings.Contains(name, "p") {
			return "unknown", "", previousDisk
		}
		cid, e := readTrim(filepath.Join(real, "device/cid"))
		if e != nil || len(cid) != 32 {
			return "unknown", "", previousDisk
		}
		raw, e := hex.DecodeString(cid)
		if e != nil {
			return "unknown", "", previousDisk
		}
		h := sha256.Sum256(raw)
		return "present", hex.EncodeToString(h[:]), name
	}
	if previousDisk != "" {
		if _, e := os.Stat("/sys/class/block/" + previousDisk); os.IsNotExist(e) {
			return "absent", "", previousDisk
		}
		return "present", "", previousDisk
	}
	return "unknown", "", ""
}
func waitOTA(want string, initialized bool) error {
	until := time.Now().Add(35 * time.Second)
	for time.Now().Before(until) {
		d, e := localQuery("otad")
		if e == nil && d["update_status"] == want {
			if !initialized {
				return nil
			}
			if v, ok := d["version"].(map[string]any); ok && v["initialized"] == true {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fail("native OTA state timeout")
}
func nativeEvent(name, arg string) error {
	b, e := localCommand(15, "/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kOtad", "Upgrade", "UpgradeProcessEvent", name, arg)
	if e != nil || !strings.Contains(string(b), "Error code: Success(0)") {
		return fail("native event not confirmed")
	}
	return nil
}
func dispatchNative(cfg SDPolicy, accessReady func() bool, handoff ...func()) error {
	return dispatchNativeObserved(cfg, accessReady, nil, handoff...)
}
func dispatchNativeObserved(cfg SDPolicy, accessReady func() bool, notice *stageNotice, handoff ...func()) error {
	// The locked candidate embeds false. No service or CLI writes a permission
	// marker while that lock is false. A future release needs separate review.
	if !cfg.AutomaticEnabled {
		return fail("candidate installation lock")
	}
	if !accessReady() || !mediaIdle() || !otaHeld() {
		return fail("pre-dispatch readiness changed")
	}
	if _, e := verifyObserved(productionPolicy(), packagePath, "", false, notice.reporter(noticeFinal, 0, 1)); e != nil {
		return e
	}
	notice.set(noticeHandoff)
	if e := checkPrivateSD(); e != nil {
		return e
	}
	// Refuse while any native SD scan filename remains visible. The coordinator
	// consumes only the authenticated file into an inert card name before intent.
	for _, dir := range []string{"/mnt/sdcard", "/data/ins"} {
		entries, e := os.ReadDir(dir)
		if e != nil {
			return e
		}
		for _, f := range entries {
			n := f.Name()
			if strings.HasPrefix(n, "insta_factory_on") || (strings.HasPrefix(n, "Insta360") && strings.Contains(n, "FW")) {
				return fail("native scan conflict")
			}
		}
	}

	if err := os.Mkdir("/run/x6-owner", 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if err := parents("/run/x6-owner/vendor-ota-enabled", productionPolicy()); err != nil {
		return err
	}
	fd, e := syscall.Open("/run/x6-owner/vendor-ota-enabled", syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), "owner OTA window")
	e = f.Sync()
	f.Close()
	if e != nil {
		os.Remove("/run/x6-owner/vendor-ota-enabled")
		return e
	}
	sent := false
	defer func() {
		if !sent {
			d, x := localQuery("otad")
			if x == nil && d["update_status"] == "Idle" {
				if _, x = localCommand(8, "/bin/systemctl", "stop", "ins_otad.service"); x == nil {
					_ = os.Remove("/run/x6-owner/vendor-ota-enabled")
				}
			}
		}
	}()
	if _, e = localCommand(15, "/bin/systemctl", "start", "ins_otad.service"); e != nil {
		return e
	}
	if e = waitOTA("Idle", true); e != nil {
		return e
	}
	if e = checkHandoffSpace(productionPolicy(), freeBytes); e != nil {
		return e
	}
	b, e := localCommand(15, "/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kOtad", "Upgrade", "UpgradeProcessFirmwarePath", packagePath)
	if e != nil || !strings.Contains(string(b), packagePath) {
		return fail("native private path not confirmed")
	}
	for _, transfer := range handoff {
		transfer()
	}
	sent = true // From here a timeout/failure is ambiguous; never stop the updater.
	if e = nativeEvent("OtaEventRecvSdFwRequest", "\"\""); e != nil {
		return e
	}
	if e = waitOTA("PreProcess", false); e != nil {
		return e
	}
	return nativeEvent("OtaEventStoragePkgFind", packagePath)
}
