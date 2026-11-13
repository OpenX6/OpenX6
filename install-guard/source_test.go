package main

import (
	"strings"
	"testing"
)

func TestUnifiedSources(t *testing.T) {
	for _, slot := range []string{"_a", "_b"} {
		for _, mode := range []string{"first-install", "maintenance"} {
			t.Run(slot+"/"+mode, func(t *testing.T) {
				c, e, s := fixture(slot)
				if mode == "first-install" {
					e.PublicSHA256 = ""
					e.Source = SourceEvidence{Version: c.Sources[0].Version, OTASHA256: c.Sources[0].OTASHA256, Preserved: map[string]string{}}
				} else {
					e.PublicSHA256 = sumBytes([]byte("different device identity"))
					e.Source.Preserved["data/x6-owner-gui/server_ed25519.pub"] = e.PublicSHA256
				}
				s.SourceID = sourceID(e)
				for _, phase := range []string{"inspect", "pre", "post"} {
					if err := validate(c, e, phase, s); err != nil {
						t.Fatal(phase, err)
					}
				}
			})
		}
	}
}
func TestSourceRejections(t *testing.T) {
	cases := map[string]func(*Config, *Evidence){
		"stock-new-version":     func(c *Config, e *Evidence) { e.Source.OTASHA256 = strings.Repeat("f", 64) },
		"version-spoof":         func(c *Config, e *Evidence) { e.Source.Version = "0.8.825" },
		"update-owner-mismatch": func(c *Config, e *Evidence) { e.Source.OwnerPublicSHA256 = strings.Repeat("a", 64) },
		"SSH-owner-malformed": func(c *Config, e *Evidence) {
			e.Source.Preserved["ins/etc/x6_owner_authorized_keys"] = "invalid"
		},
		"SSH-key-missing":      func(c *Config, e *Evidence) { delete(e.Source.Preserved, "data/x6-owner-ssh/host_ed25519") },
		"GUI-private-missing":  func(c *Config, e *Evidence) { delete(e.Source.Preserved, "data/x6-owner-gui/server_ed25519") },
		"GUI-identity-missing": func(c *Config, e *Evidence) { e.PublicSHA256 = "" },
		"ambiguous-allowlist":  func(c *Config, e *Evidence) { c.Sources = append(c.Sources, c.Sources[1]) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c, e, _ := fixture("_a")
			change(&c, &e)
			if validate(c, e, "pre", nil) == nil {
				t.Fatal("accepted")
			}
		})
	}
}
func TestPreservedStatePostMismatch(t *testing.T) {
	for _, p := range []string{"ins/etc/x6_owner_authorized_keys", "data/x6-owner-ssh/host_ed25519", "data/x6-owner-features/raw-nr-v1/level"} {
		t.Run(p, func(t *testing.T) {
			c, e, s := fixture("_b")
			e.Source.Preserved[p] = sumBytes([]byte("before"))
			s.SourceID = sourceID(e)
			e.Source.Preserved[p] = sumBytes([]byte("after"))
			if validate(c, e, "post", s) == nil {
				t.Fatal("preserved data change accepted")
			}
		})
	}
}
