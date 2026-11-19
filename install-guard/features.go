package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var featurePaths = []string{
	"data/x6-owner-features/vr180-v1/gui", "data/x6-owner-features/vr180-v1/setup", "data/x6-owner-features/vr180-v1/sync.so",
	"data/x6-owner-features/vr180-v1/backup/gui", "data/x6-owner-features/vr180-v1/backup/setup", "data/x6-owner-features/vr180-v1/backup/sync.so",
	"data/x6-owner-features/raw-nr-v1/medium.so", "data/x6-owner-features/raw-nr-v1/low.so", "data/x6-owner-features/raw-nr-v1/minimal.so",
}

func checkFeatureFilesAt(root string, sets []map[string]string) (map[string]string, error) {
	if len(sets) == 0 {
		return nil, fmt.Errorf("missing feature layouts")
	}
	actual := map[string]string{}
	for _, p := range featurePaths {
		h, e := checkedHash(root, p, false)
		if e != nil {
			return nil, e
		}
		actual[p] = h
	}
	matches := 0
	for _, s := range sets {
		if len(s) != len(actual) {
			return nil, fmt.Errorf("bad feature layout")
		}
		ok := true
		for p, h := range actual {
			if !isHash(s[p]) {
				return nil, fmt.Errorf("bad feature digest")
			}
			if s[p] != h {
				ok = false
			}
		}
		if ok {
			matches++
		}
	}
	if matches != 1 {
		return nil, fmt.Errorf("unknown, mixed or ambiguous feature layout")
	}
	if e := checkFeaturePreferencesAt(root); e != nil {
		return nil, e
	}
	return actual, nil
}

var featurePreferences = map[string][]byte{
 "data/x6-owner-features/mod-controls-v1/stereo.pref": {0,1}, "data/x6-owner-features/mod-controls-v1/hnr.pref": {0,1}, "data/x6-owner-features/mod-controls-v1/vnr.pref": {0,1,2,3}, "data/x6-owner-features/proxy-v1/disabled": {0,1},
	"data/x6-owner-features/vr180-v1/enabled": {'1'}, "data/x6-owner-features/vr180-v1/sync": {0, 1},
	"data/x6-owner-features/raw-nr-v1/enabled": {'1'}, "data/x6-owner-features/raw-nr-v1/level": {0, 1, 2, 3},
}

func checkFeaturePreferencesAt(root string) error {
	for p, allowed := range featurePreferences {
		h, e := checkedHash(root, p, strings.HasSuffix(p, "/enabled") || strings.Contains(p,"/mod-controls-v1/") || strings.Contains(p,"/proxy-v1/"))
		if e != nil {
			return e
		}
		if h == "" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(root, p))
		if e != nil || len(b) != 1 {
			return fmt.Errorf("invalid feature preference")
		}
		ok := false
		for _, v := range allowed {
			if b[0] == v {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("unsupported preference")
		}
	}
	return nil
}
