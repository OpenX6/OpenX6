package main

import (
	"errors"
	"strings"
	"testing"
)

func TestImportSpaceRecheck(t *testing.T) {
	for _, c := range []struct {
		name          string
		before, after uint64
		fail, ok      bool
	}{{"enough", 100, 100, false, true}, {"reclaimed", 0, 100, false, true}, {"insufficient", 0, 99, false, false}, {"refused", 0, 100, true, false}, {"no-progress", 0, 0, false, false}} {
		t.Run(c.name, func(t *testing.T) {
			n := 0
			free := c.before
			e := spaceCheck(100, func() (uint64, error) { return free, nil }, func() error {
				n++
				free = c.after
				if c.fail {
					return errors.New("busy")
				}
				return nil
			})
			if (e == nil) != c.ok {
				t.Fatal(e)
			}
			if c.before >= 100 && n != 0 {
				t.Fatal("unneeded deletion")
			}
			if n > 1 {
				t.Fatal("cleanup loop")
			}
			if c.after < 100 && !strings.Contains(e.Error(), "shortfall") {
				t.Fatal(e)
			}
		})
	}
	called := false
	e := spaceCheck(100, func() (uint64, error) { return 0, errors.New("statfs") }, func() error { called = true; return nil })
	if e == nil || called {
		t.Fatal("statfs failure ignored")
	}
}
