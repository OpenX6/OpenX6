package main

import (
	"encoding/hex"
	"strings"
)

func bindingMatches(b []byte, h string) bool {
	if len(b) != 65 || b[64] != 10 || string(b[:64]) != h || h != strings.ToLower(h) {
		return false
	}
	d, e := hex.DecodeString(h)
	return e == nil && len(d) == 32
}
