package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// stagingSpace is the only resource gate left before a package is staged.
// Owner decision 2026-11-02: no battery or USB-power condition of our own; the
// factory updater installed from the card without one, and ours used to sit
// silently at "not-ready" below 70 % battery or without a charger.
func stagingSpace(size uint64) bool {
	var st syscall.Statfs_t
	if syscall.Statfs("/tmp", &st) != nil {
		return false
	}
	return uint64(st.Bavail)*uint64(st.Bsize) > size+(512<<20)
}
func numericVersion() (string, error) {
	b, e := os.ReadFile("/ins/etc/version.txt")
	if e != nil {
		return "", e
	}
	v := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "qcs8625_Version:V") {
			if v != "" {
				return "", fail("duplicate version")
			}
			v = strings.TrimSpace(strings.TrimPrefix(l, "qcs8625_Version:V"))
		}
	}
	if !versionRE.MatchString(v) {
		return "", fail("invalid current version")
	}
	return v, nil
}

// The owner drops cardInputPath on the card; the coordinator renames it to
// consumedCardPath right before the vendor handoff so a reboot or re-insert
// never installs it twice. After the install is confirmed complete, the consumed
// copy is removed (see removeConsumedCardInput) so the next package the owner
// drops is not refused as "ambiguous SD packages".
const (
	cardInputPath    = "/mnt/sdcard/Insta360X6FW.bin"
	consumedCardPath = "/mnt/sdcard/.x6-owner-package.bin"
)

func openCardPackage(allowDuplicate bool) (f *os.File, path string, err error) {
	for _, p := range []string{cardInputPath, consumedCardPath} {
		s, e := os.Lstat(p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, "", e
		}
		if !s.Mode().IsRegular() {
			return nil, "", fail("unsafe SD entry")
		}
		if path != "" {
			if allowDuplicate {
				continue
			} // Verified against visible input before use.
			return nil, "", fail("ambiguous SD packages")
		}
		path = p
	}
	if path == "" {
		return nil, "", nil
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, "", e
	}
	return os.NewFile(uintptr(fd), path), path, nil
}
func consumeCardName(f *os.File, path string, duplicate os.FileInfo) error {
	if path == consumedCardPath {
		return nil
	}
	if path != cardInputPath {
		return fail("unexpected source name")
	}
	a, e := f.Stat()
	if e != nil {
		return e
	}
	b, e := os.Lstat(path)
	if e != nil || !os.SameFile(a, b) {
		return fail("card source replaced")
	}
	dest := consumedCardPath
	if target, err := os.Lstat(dest); !os.IsNotExist(err) && (err != nil || duplicate == nil || !os.SameFile(duplicate, target)) {
		return fail("card consumed name occupied")
	}
	if e = os.Rename(path, dest); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func maintenanceReady(expected, current string, readErr error, enrolled bool) bool {
	return enrolled && readErr == nil && validID(expected) && current == expected
}

func runCoordinator() error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 {
		return fail("target Linux ARM64 root required")
	}
	var cfg SDPolicy
	if e := json.Unmarshal(coordinatorPolicyBytes, &cfg); e != nil {
		return e
	}
	id, e := readTrim("/usr/share/x6-release-id")
	if e != nil || id != cfg.Release || !validID(id) {
		return fail("installed release identity mismatch")
	}
	p := productionPolicy()
	if e = privateDir(filepath.Dir(sdJournal), p); e != nil {
		return e
	}
	lockPath := filepath.Join(filepath.Dir(sdJournal), "lock")
	fd, e := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	lock := os.NewFile(uintptr(fd), lockPath)
	defer lock.Close()
	st, e := lock.Stat()
	if e != nil {
		return e
	}
	ss := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || ss.Uid != 0 || ss.Nlink != 1 {
		return fail("unsafe coordinator lock")
	}
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fail("coordinator already running")
	}
	state, e := loadSDState(sdJournal, p)
	if e != nil {
		return e
	}
	c := SDCoordinator{State: state, Save: func(s SDState) error { return saveSDState(sdJournal, p, s) }}
	boot, e := readTrim("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return e
	}
	slot, e := currentSlot(cfg)
	if e != nil {
		return e
	}
	version, e := numericVersion()
	if e != nil {
		return e
	}
	pub, _ := readSmall(p.Public, p, false, 32)
	var disk, cacheKey, lastStatus string
	var noticeCheckedKey string
	var cached manifest
	var cachedRelease string
	lastCheck := time.Time{}
	storageRecovered := false
	cleanedDigest := ""
	var failure failureWindow
	var consent stockDecision
	var seenCard, removedCard string
	jobs := make(chan phoneJob)
	if cfg.AutomaticEnabled {
		stop, err := startPhone(jobs, func() bool {
			state, err := loadSDState(sdJournal, p)
			return err == nil && state.Phase != "intent" && state.Phase != "dispatched" && state.Phase != "uncertain" && phoneIdle(cfg)
		})
		if err != nil {
			fmt.Println("Mobile update endpoint unavailable:", err)
		} else {
			defer stop()
		}
	}
	for {
		if !storageRecovered && c.State.Phase != "intent" && c.State.Phase != "dispatched" {
			rs := c.State
			if rs.Version == 0 {
				rs.Phase = "idle"
			}
			if e := recoverPrivateSD(rs); e != nil {
				fmt.Printf("SD stage recovery deferred: %v\n", e)
				time.Sleep(10 * time.Second)
				continue
			}
			storageRecovered = true
		}
		if c.State.Phase == "completed" && cleanedDigest != c.State.Digest {
			if !mediaIdle() || !otaHeld() || noUSBExport() != nil {
				time.Sleep(10 * time.Second)
				continue
			}
			if e := cleanupCompletedStage(p, c.State); e != nil {
				fmt.Printf("Completed stage cleanup deferred: %v\n", e)
				time.Sleep(10 * time.Second)
				continue
			}
			cleanedDigest = c.State.Digest
			storageRecovered = true
		}
		if c.State.Phase == "intent" || c.State.Phase == "dispatched" {
			rootReady := !c.State.Native && c.State.Boot != boot && c.State.SourceSlot != slot && localRootReady(cfg)
			if rootReady || c.State.Native || c.State.Boot == boot {
				identity, e := identityDigest()
				if e != nil {
					rootReady = false
				}
				status, e := c.Step(SDObservation{Boot: boot, Slot: slot, CurrentRelease: id, Identity: identity, Ready: rootReady, CompletionVerified: rootReady && identity == c.State.Identity, NativeIdle: c.State.Native && otaHeld() && mediaIdle()})
				if e != nil {
					return e
				}
				if status != lastStatus {
					fmt.Println(status)
					lastStatus = status
				}
				time.Sleep(10 * time.Second)
				continue
			}
		}
		select {
		case job := <-jobs:
			err := installPhone(job.pkg, &c, cfg, p, id, boot, slot)
			if err != nil && (c.State.Version == 0 || stageMayRecover(c.State.Phase)) {
				storageRecovered = false
			}
			job.result <- err
			continue
		default:
		}
		presence, card, d := cardObservation(disk)
		disk = d
		switch presence {
		case "absent":
			if seenCard != "" {
				removedCard = seenCard
				seenCard = ""
			}
		case "present":
			if card != "" {
				seenCard = card
				if removedCard != card {
					removedCard = ""
				}
			}
		}
		if presence == "present" && c.State.Native && c.State.Phase == "completed" && card == c.State.Card && cleanedDigest == c.State.Digest {
			if err := archiveNativeCard(consumedCardPath, c.State.Digest); err != nil {
				fmt.Println(err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		if presence == "present" && card == "" {
			presence = "unknown"
		}
		// Poll physical removal promptly. Expensive hashing/readiness checks are
		// cached only for unchanged file metadata and reverified before each dispatch.
		if presence != "present" {
			consent = stockDecision{}
			noticeCheckedKey = ""
			_, _ = c.Step(SDObservation{Boot: boot, Slot: slot, CurrentRelease: id, Presence: presence})
			cacheKey = ""
			time.Sleep(2 * time.Second)
			continue
		}
		if time.Since(lastCheck) < 10*time.Second {
			time.Sleep(time.Second)
			continue
		}
		lastCheck = time.Now()
		identity, ie := identityDigest()
		enrolled := ie == nil && validID(identity) && holderReady(cfg)
		o := SDObservation{Boot: boot, Slot: slot, CurrentRelease: id, Presence: presence, Card: card, Identity: identity, InstallAllowed: cfg.AutomaticEnabled, NativeIdle: c.State.Native && otaHeld() && mediaIdle()}
		failureReady := failure.observed(time.Now(), stoppedOnCamera())
		f, path, e := openCardPackage(failureReady)
		if e != nil {
			fmt.Println("SD package unavailable or ambiguous")
			continue
		}
		if f == nil {
			consent = stockDecision{}
			_, _ = c.Step(o)
			continue
		}
		info, e := f.Stat()
		if e != nil {
			f.Close()
			continue
		}
		key := fmt.Sprintf("%s:%s:%d:%d:%v", card, path, info.Size(), info.ModTime().UnixNano(), info.Sys().(*syscall.Stat_t).Ino)
		func() {
			var notice stageNotice
			defer notice.close()
			beginSDNotice := func() {
				if notice.begin("/run/x6-sd-update-notice") {
					_, _ = localCommand(2, "/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kBla", "Gui", "GuiSetScreenState", "ScreenBright", "GuiScreenStateChangeUnknown")
				}
			}
			if key != cacheKey {
				if key != noticeCheckedKey && path == cardInputPath && cfg.AutomaticEnabled && enrolled {
					noticeCheckedKey = key
					data, ed := freeBytes("/data")
					sd, es := freeBytes("/mnt/sdcard")
					plan, ep := planStage(uint64(info.Size()), data, sd)
					if ed == nil && es == nil && ep == nil && plan.Backend == "sd" && mediaIdle() {
						beginSDNotice()
					}
				}
				cached, _, _, e = inspectCardObserved(f, pub, p, notice.reporter(noticeChecking, 0, 1))
				if e == nil {
					cachedRelease, e = cardRelease(f, cached)
				}
				if e == nil && cached.Unsigned {
					fmt.Println("Passing card firmware to the native updater; OpenX6 has not authenticated its publisher")
				}
				if e != nil {
					f.Close()
					fmt.Println("SD authorization rejected")
					return
				}
				cacheKey = key
			}
			if !cached.Native && compareVersion(cached.Version, version) < 0 && !cached.Downgrade {
				f.Close()
				fmt.Println("downgrade not authorized")
				return
			}
			o.PackageDigest = fmt.Sprintf("%x", cached.Digest)
			o.PackageRelease = cachedRelease
			o.Native = cached.Native
			o.Factory = cached.Factory
			c.ConfirmFactory = func() bool {
				notice.close()
				present := func() bool {
					now, err := os.Lstat(path)
					return err == nil && os.SameFile(info, now) && info.Size() == now.Size() && info.ModTime() == now.ModTime()
				}
				return consent.allow(key+o.PackageDigest, func(phase byte) bool {
					return present() && stockQuestion(phase, present) && present()
				})
			}
			o.Repair = !cached.Native && failureReady && cached.Version == version && cachedRelease == id &&
				enrolled && noUSBExport() == nil
			var duplicate os.FileInfo
			if path == cardInputPath {
				duplicate, e = matchingConsumed(consumedCardPath, o.Repair, o.PackageDigest, pub, p)
				if e != nil {
					f.Close()
					fmt.Println("conflicting SD inputs")
					return
				}
			}
			o.NewPresentation = path == cardInputPath || (removedCard != "" && removedCard == card)
			// Preparation and dispatch bind the enrolled identity to this operation.
			// Actual service health is collected only on the booted target.
			accessReady := func() bool {
				if noUSBExport() != nil {
					return false
				}
				if o.Native {
					return true
				}
				nowID, err := identityDigest()
				return maintenanceReady(identity, nowID, err, holderReady(cfg)) && (!o.Repair || stoppedOnCamera())
			}
			o.Ready = (o.Native || enrolled) && noUSBExport() == nil && mediaIdle() && otaHeld() && stagingSpace(cached.Size)
			c.Stage = func() error {
				beginSDNotice()
				if e := retireVerifiedStage(p, c.State, o.PackageDigest); e != nil {
					return e
				}
				m, _, _, e := inspectCardObserved(f, pub, p, notice.reporter(noticeConfirming, 0, 1))
				if e != nil || fmt.Sprintf("%x", m.Digest) != o.PackageDigest {
					return fail("changed input")
				}
				if e = stageWithPolicy(f, &path, card, o.PackageDigest, boot, p, duplicate, cached.Native, &notice); e != nil {
					return e
				}
				return nil
			}
			c.Restage = func() error {
				if _, e := os.Lstat(p.Package); os.IsNotExist(e) {
					return c.Stage()
				} else if e != nil {
					return e
				}
				return nil
			}
			c.Prepare = func() error {
				if !cfg.AutomaticEnabled {
					return fail("candidate locked")
				}
				if !accessReady() || !otaHeld() || !mediaIdle() {
					return fail("readiness changed")
				}
				if e := checkPrivateSD(); e != nil {
					return e
				}
				if e := checkHandoffSpace(p, freeBytes); e != nil {
					return e
				}
				if e := consumeCardName(f, path, duplicate); e != nil {
					return e
				}
				return nil
			}
			c.Dispatch = func() error {
				beginSDNotice()
				notice.set(noticeHandoff)
				m, e := stagedRecord(p)
				if e != nil || fmt.Sprintf("%x", m.Digest) != c.State.Digest {
					return fail("staged identity changed")
				}
				return dispatchNativeObserved(cfg, accessReady, &notice, notice.close)
			}
			status, stepErr := c.Step(o)
			if status == "repair-armed" {
				removedCard = ""
				storageRecovered = false
			}
			if stepErr == nil && shouldRemoveConsumedInput(status, path, c.State, cleanedDigest, o.PackageDigest) {
				if e := removeConsumedCardInput(f, path); e != nil {
					fmt.Printf("consumed card input kept: %v\n", e)
				} else {
					fmt.Println("consumed card input removed after confirmed completion")
					cacheKey = ""
				}
			}
			f.Close()
			if stepErr != nil {
				fmt.Printf("coordinator refused; durable state retained: %v\n", stepErr)
				if stageMayRecover(c.State.Phase) || c.State.Version == 0 {
					storageRecovered = false
				}
			} else if status != lastStatus {
				fmt.Println(status)
				lastStatus = status
			}
		}()
	}
}
