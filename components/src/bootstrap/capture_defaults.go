package main

import (
	"errors"
	"os"
	"path/filepath"
)

// These are initial preferences for the recording and display controllers.
// A complete or adopted installation never passes through install again.
var captureDefaults = []struct{ path, value string }{
	{"mod-controls-v1/stereo.pref", "\x01"},
	{"stats-v1/enabled.pref", "\x01"},
	{"interval-controls-v1/tl.pref", "\x01"},
	{"proxy-v1/disabled", "\x01"},
	{"flat-mirror-v1/enabled", ""}, // The GUI uses file presence for this switch.
}

// Both paths are on the persistent data filesystem. A racing controller can
// win the rename, but its saved setting can never be overwritten by this seed.
func (e Engine) publishAbsent(stage, name, target string) error {
	parent := filepath.Dir(target)
	if err := e.makeDir(parent); err != nil {
		return err
	}
	src, dst := e.path(stage+"/"+name), e.path(target)
	err := renameAbsent(src, dst)
	if errors.Is(err, os.ErrExist) {
		if err = metadata(dst, false, 0); err != nil {
			return err
		}
		err = os.Remove(src)
	}
	if err != nil {
		return err
	}
	if err = syncDir(e.path(parent)); err != nil {
		return err
	}
	return syncDir(filepath.Dir(src))
}
