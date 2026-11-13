package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Separate from the signed package's own install guard. This conservative floor
// exceeds the current ~690 MB migration budget; the native guard still rechecks.
const migrationReserve uint64 = 1 << 30
const importMargin uint64 = 16 << 20
const sdStagePackage = "/run/x6-sd-stage/fs/work/package.bin"

type StagePlan struct {
	Native                                 bool
	Backend                                string
	DataRequired, ImageBytes, CardRequired uint64
}

func planStage(size, dataFree, cardFree uint64) (StagePlan, error) {
	return planInputStage(size, dataFree, cardFree, false, migrationReserve)
}

// The release marker only increases the space budget. It does not authenticate
// a publisher, change the copy receipt or normalize a foreign package.
func inputReserve(f *os.File, native bool) (uint64, error) {
	if !native {
		return migrationReserve, nil
	}
	_, err := packageRelease(f)
	if err == nil {
		return migrationReserve, nil
	}
	var ioErr *os.PathError
	if errors.As(err, &ioErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, err
	}
	return importMargin, nil
}

var errHandoffSpace = errors.New("insufficient migration or staging headroom before OTA handoff")

func checkHandoffSpace(p policy, available func(string) (uint64, error)) error {
	m, err := stagedRecord(p)
	if err != nil {
		return err
	}
	f, err := openChecked(p.Package, p, true)
	if err != nil {
		return err
	}
	defer f.Close()
	reserve, err := inputReserve(f, m.Native)
	if err != nil {
		return err
	}
	free, err := available("/data")
	if err != nil {
		return err
	}
	if free < reserve {
		return fmt.Errorf("%w: need %d, available %d", errHandoffSpace, reserve, free)
	}
	return nil
}

func planInputStage(size, dataFree, cardFree uint64, native bool, reserve uint64) (StagePlan, error) {
	if native {
		if size < 4 || size > 1<<63-1-(512<<20) {
			return StagePlan{}, fail("invalid host file size")
		}
	} else if size < 9984 || size >= 1<<32 {
		return StagePlan{}, fail("invalid staging size")
	}
	need := size + importMargin + reserve
	if dataFree >= need {
		return StagePlan{Native: native, Backend: "data", DataRequired: need}, nil
	}
	if dataFree < reserve {
		return StagePlan{}, fmt.Errorf("staging headroom shortfall: need %d, available %d", reserve, dataFree)
	}
	image := (size + (128 << 20) + (64 << 20) - 1) / (64 << 20) * (64 << 20)
	if cardFree < image+(64<<20) {
		return StagePlan{}, fail("SD private staging space insufficient")
	}
	return StagePlan{Native: native, Backend: "sd", DataRequired: reserve, ImageBytes: image, CardRequired: image + (64 << 20)}, nil
}

func freeBytes(path string) (uint64, error) {
	var s syscall.Statfs_t
	e := syscall.Statfs(path, &s)
	return uint64(s.Bavail) * uint64(s.Bsize), e
}

// Caller has already authenticated the package. No cleanup or SD writes precede
// that boundary. A completed private stage is still verified at dispatch.
func stageWithSpace(f *os.File, path *string, card, digest, boot string, p policy, duplicate os.FileInfo) error {
	return stageWithPolicy(f, path, card, digest, boot, p, duplicate, false, nil)
}
func stageWithPolicy(f *os.File, path *string, card, digest, boot string, p policy, duplicate os.FileInfo, native bool, notice *stageNotice) error {
	if _, e := os.Lstat(p.Package); e == nil {
		_, x := verify(p, p.Package, "", false)
		if x != nil {
			return x
		}
		m, x := stagedRecord(p)
		if x != nil {
			return x
		}
		if fmt.Sprintf("%x", m.Digest) != digest {
			return fail("staged package digest mismatch")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	st, e := f.Stat()
	if e != nil {
		return e
	}
	data, e := freeBytes("/data")
	if e != nil {
		return e
	}
	reserve, e := inputReserve(f, native)
	if e != nil {
		return e
	}
	if data < reserve {
		if e = ensureImportSpace("/data", reserve, p); e != nil {
			return e
		}
		data, e = freeBytes("/data")
		if e != nil {
			return e
		}
	}
	sd, e := freeBytes("/mnt/sdcard")
	if e != nil {
		return e
	}
	plan, e := planInputStage(uint64(st.Size()), data, sd, native, reserve)
	if e != nil {
		return e
	}
	if plan.Backend == "data" {
		return stageInput(f, p, nil, inspectCardObserved, notice)
	}
	notice.set(noticePreparing)
	// Rename while the public card is still writable. This does not grant install
	// permission, and avoids a native scan after a failed/interrupted import.
	if e = consumeCardName(f, *path, duplicate); e != nil {
		return e
	}
	*path = consumedCardPath
	if f.Name() != *path {
		reopened, err := reopenConsumed(f, *path)
		if err != nil {
			return err
		}
		defer reopened.Close()
		f = reopened
	}
	if e = preparePrivateSD(plan, card, digest, boot, notice); e != nil {
		return e
	}
	q := p
	q.Package = sdStagePackage
	if e = stageInput(f, q, nil, inspectCardObserved, notice); e != nil {
		return e
	}
	if e = publishPrivateSD(); e != nil {
		return e
	}
	_, e = verifyObserved(p, p.Package, "", false, notice.reporter(noticeVerifying, 2, 3))
	return e
}
func cleanupCompletedStage(p policy, s SDState) error {
	if s.Phase != "completed" {
		return fail("cleanup requires confirmed completion")
	}
	if e := recoverPrivateSD(s); e != nil {
		return e
	}
	// Always resume retired-directory cleanup, even when the original import
	// path disappeared after a durable rename before the previous crash.
	return retireVerifiedStage(p, s, "")
}

// Rename deliberately changes the pathname. Reopen only if the consumed name
// still denotes the same regular input, then validate the package normally.
func reopenConsumed(original *os.File, path string) (*os.File, error) {
	before, e := original.Stat()
	if e != nil {
		return nil, e
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	after, e := f.Stat()
	if e != nil || !before.Mode().IsRegular() || !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		f.Close()
		return nil, fail("consumed SD input changed")
	}
	return f, nil
}
