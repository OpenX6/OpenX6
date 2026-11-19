package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadySignedBundleCopy(t *testing.T) {
	for _, mode := range []string{"same", "changed-code", "changed-team", "changed-resource", "untrusted", "missing-receipt", "duplicate-identity", "invalid-hash", "wrong-identifier", "wrong-layout"} {
		t.Run(mode, func(t *testing.T) {
			oldRepo, oldVersion, oldSigner := Repository, Version, codeSignOutput
			t.Cleanup(func() { Repository, Version, codeSignOutput = oldRepo, oldVersion, oldSigner })
			t.Setenv("HOME", t.TempDir())
			Repository, Version = "TestOrg/OpenX6", "1.1.0"
			dir, err := store()
			if err != nil {
				t.Fatal(err)
			}
			target := binary(dir, Version)
			running := filepath.Join(t.TempDir(), "AppTranslocation", "session", "d", "OpenX6.app", "Contents", "MacOS", "OpenX6")
			if mode == "wrong-layout" {
				running = filepath.Join(t.TempDir(), "OpenX6")
			}
			for _, p := range []string{target, running} {
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("identical executable bytes"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "missing-receipt" {
				if err := os.WriteFile(filepath.Join(dir, Version, "archive.sha256"), []byte("sha256:download-receipt"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			verifies := 0
			codeSignOutput = func(ctx context.Context, args ...string) ([]byte, error) {
				app := args[len(args)-1]
				other := app != readyApp(target)
				if args[0] == "--verify" {
					verifies++
					joined := strings.Join(args, " ")
					for _, s := range []string{"--deep", "--strict", "--all-architectures", "anchor apple generic", "1.2.840.113635.100.6.1.13"} {
						if !strings.Contains(joined, s) {
							t.Fatalf("missing verification: %s", s)
						}
					}
					if (mode == "changed-resource" && other) || mode == "untrusted" {
						return nil, fmt.Errorf("signature invalid")
					}
					return nil, nil
				}
				hash, team, id := strings.Repeat("a", 40), "ABCDEFGHIJ", "OpenX6"
				if other {
					if mode == "changed-code" {
						hash = strings.Repeat("b", 40)
					}
					if mode == "changed-team" {
						team = "KLMNOPQRST"
					}
					if mode == "invalid-hash" {
						hash = "bad"
					}
					if mode == "wrong-identifier" {
						id = "OtherApp"
					}
				}
				result := "Identifier=" + id + "\nTeamIdentifier=" + team + "\nCDHash=" + hash + "\n"
				if mode == "duplicate-identity" && other {
					result += "CDHash=" + hash + "\n"
				}
				return []byte(result), nil
			}
			err = Ready(running)
			marker, readErr := os.ReadFile(filepath.Join(dir, Version, "ready"))
			if mode == "same" {
				if err != nil || readErr != nil || string(marker) != Version || verifies != 2 {
					t.Fatalf("did not activate verified bundle copy: %v %v %q verifies=%d", err, readErr, marker, verifies)
				}
				codeSignOutput = func(context.Context, ...string) ([]byte, error) {
					t.Fatal("reverified an already activated version")
					return nil, nil
				}
				if err := Ready(running); err != nil {
					t.Fatal(err)
				}
				Version = "1.0.0"
				if Active() != target {
					t.Fatal("old entry did not select updated executable")
				}
			} else if readErr == nil {
				t.Fatalf("activated %s", mode)
			}
		})
	}
}
