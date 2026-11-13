package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInputMutationDuringImport(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	b := embedTest(t, s)
	f := inputFile(t, s, b)
	p := s.p
	p.Package = filepath.Join(p.Boundary, "imported/package.bin")
	err := StageSingle(f, p, func(point string) error {
		if point == "before-copy" {
			b[4864+576] ^= 1
			return os.WriteFile(f.Name(), b, 0600)
		}
		return nil
	})
	if err == nil {
		t.Fatal("changed input accepted")
	}
	if _, err = os.Lstat(filepath.Dir(p.Package)); !os.IsNotExist(err) {
		t.Fatal("published changed input")
	}
}
