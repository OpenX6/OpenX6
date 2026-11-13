package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
)

// The independent build process prints its verified digest as the final line.
type verifiedFooter []byte

func (f *verifiedFooter) Write(p []byte) (int, error) {
	n := len(p)
	if n > 256 {
		p = p[n-256:]
	}
	b := append(*f, p...)
	if len(b) > 256 {
		b = b[len(b)-256:]
	}
	*f = b
	return n, nil
}
func (f verifiedFooter) digest() (string, error) {
	lines := bytes.Split(bytes.TrimRight(f, "\r\n"), []byte("\n"))
	s := string(lines[len(lines)-1])
	h, ok := strings.CutPrefix(s, "Verified: ")
	b, e := hex.DecodeString(h)
	if !ok || e != nil || len(b) != 32 || h != strings.ToLower(h) {
		return "", errors.New("Missing verified digest from builder; local file kept, deployment refused")
	}
	return h, nil
}
