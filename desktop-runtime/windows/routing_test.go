package main

import (
	"errors"
	"testing"
)

func TestRegisteredWorkerNeverReimported(t *testing.T) {
	for _, tc := range []struct {
		marker string
		ok     bool
	}{{originalBase, true}, {factory827Base, true}, {"unknown", false}, {"", false}} {
		mode, err := prepare(true, factory827Base, func() (string, error) { return tc.marker, nil },
			func() error { t.Fatal("attempted to import over an existing registration"); return nil })
		if (err == nil) != tc.ok || (tc.ok && mode != "existing") {
			t.Fatalf("marker %q: %s, %v", tc.marker, mode, err)
		}
	}
	if _, err := prepare(true, factory827Base, func() (string, error) { return "", errors.New("probe failed") }, func() error { t.Fatal("unexpected import"); return nil }); err == nil {
		t.Fatal("unreadable registration accepted")
	}
}
func TestFreshImportAndFailure(t *testing.T) {
	imported := false
	mode, err := prepare(false, factory827Base, func() (string, error) {
		if !imported {
			t.Fatal("unverified import")
		}
		return factory827Base, nil
	}, func() error { imported = true; return nil })
	if err != nil || mode != "imported" {
		t.Fatal(mode, err)
	}
	if _, err := prepare(false, factory827Base, func() (string, error) { t.Fatal("failed import ran worker"); return "", nil }, func() error { return errors.New("disk full") }); err == nil {
		t.Fatal("failed import accepted")
	}
	if supportsBase(factory827Base, "future-base") {
		t.Fatal("unchecked toolchain accepted")
	}
}

func TestReleaseAcceptsExistingWorkers(t *testing.T) {
	for _, old := range []string{originalBase, factory827Base, releaseBase} {
		mode, err := prepare(true, releaseBase, func() (string, error) { return old, nil },
			func() error { t.Fatal("must reuse registered distribution"); return nil })
		if err != nil || mode != "existing" {
			t.Fatalf("cannot reuse worker %s: %v", old, err)
		}
	}
	if supportsBase("unrecognized", releaseBase) || supportsBase(releaseBase, factory827Base) {
		t.Fatal("unexpected worker compatibility")
	}
}
