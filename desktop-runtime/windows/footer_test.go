package main

import (
	"strings"
	"testing"
)

func TestBuildDigestReceipt(t *testing.T) {
	hash := strings.Repeat("bc", 32)
	for _, suffix := range []string{"Verified: " + hash + "\r\n", "Verified: " + hash + " trailing", "Verified: " + hash + "\nfailed\n", "Verified: " + strings.ToUpper(hash), hash} {
		var f verifiedFooter
		for _, p := range []string{strings.Repeat("x", 700) + "\n", suffix[:4], suffix[4:]} {
			f.Write([]byte(p))
		}
		h, err := f.digest()
		if suffix == "Verified: "+hash+"\r\n" {
			if err != nil || h != hash {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("invalid receipt accepted")
		}
	}
}
