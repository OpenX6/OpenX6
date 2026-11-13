package main

import (
	"strings"
	"testing"
)

func TestLocalDeviceBinding(t *testing.T) {
	a := strings.Repeat("a", 64)
	b := strings.Repeat("b", 64)
	for _, c := range []struct {
		s, h string
		ok   bool
	}{{a + "\n", a, true}, {b + "\n", b, true}, {a + "\n", b, false}, {"", a, false}, {a, a, false}, {strings.ToUpper(a) + "\n", strings.ToUpper(a), false}, {a + "\nextra", a, false}} {
		if bindingMatches([]byte(c.s), c.h) != c.ok {
			t.Fatal(c)
		}
	}
}
