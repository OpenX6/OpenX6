package main

import (
	"errors"
	"os"
	"testing"
)

func TestPersistentCommitRollback(t *testing.T) {
	for old := byte(0); old < 4; old++ {
		for next := byte(0); next < 4; next++ {
			for fail := 0; fail < 3; fail++ {
				files := map[string]byte{base + "/level": old, runtime + "/state": old}
				calls := 0
				save := func(p string, b []byte, m os.FileMode) error {
					calls++
					if calls == fail {
						return errors.New("injected write failure")
					}
					if len(b) != 1 || m != 0600 {
						t.Fatal("unexpected write")
					}
					files[p] = b[0]
					return nil
				}
				e := commitSelection(next, old, save)
				want := next
				if fail > 0 {
					want = old
				}
				if (e != nil) != (fail > 0) {
					t.Fatal(old, next, fail, e)
				}
				if files[base+"/level"] != want || files[runtime+"/state"] != want {
					t.Fatal("commit not transactional", old, next, fail, files)
				}
			}
		}
	}
}

func TestErrorAfterWriteRestoresBothCopies(t *testing.T) {
	for fail := 1; fail <= 2; fail++ {
		files := map[string]byte{base + "/level": 1, runtime + "/state": 1}
		calls := 0
		save := func(p string, b []byte, m os.FileMode) error {
			calls++
			files[p] = b[0]
			if calls == fail {
				return errors.New("failure after rename")
			}
			return nil
		}
		if e := commitSelection(3, 1, save); e == nil {
			t.Fatal("expected failure")
		}
		if files[base+"/level"] != 1 || files[runtime+"/state"] != 1 {
			t.Fatal("rollback failed", files)
		}
	}
}
