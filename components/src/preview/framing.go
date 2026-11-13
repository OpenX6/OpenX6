package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// The native view picker owns these markers. The preview nodes consume the
// atomic link only while the corresponding Fish view is selected.
const fishRoot = "/run/x6-nativefish"
const mediaSettings = "/ins/setting/dynamic/user/user_config/kMedia"

type framingConfig struct {
	Format struct {
		Size   string `json:"media_res"`
		Aspect string `json:"media_ratio"`
	} `json:"kMediaResRatio"`
	Speed struct {
		FPS string `json:"video_fps"`
	} `json:"kMediaVideoFps"`
}

func frameWindow(v framingConfig, panoramic bool) string {
	size, aspect, fps := v.Format.Size, v.Format.Aspect, v.Speed.FPS
	if panoramic {
		if aspect != "kRatio1x1" {
			return ""
		}
		switch {
		case size == "kRes16K" && fps == "k30":
			return "T"
		case size == "kRes2_7K" && fps == "k60":
			return "P"
		case size == "kRes1080P" && fps == "k30":
			return "Q"
		}
		return ""
	}
	switch {
	case size == "kRes1080P" && aspect == "kRatio1x1" && (fps == "k24" || fps == "k25" || fps == "k30"):
		return "A"
	case size == "kRes6K" && (fps == "k25" || fps == "k30") && nativeFreeFrameRatio(aspect):
		return "B"
	case size == "kRes16K" && aspect == "kRatio4x3" && fps == "k60":
		return "C"
	case size == "kRes4K" && aspect == "kRatio4x3" && fps == "k120":
		return "D"
	}
	return ""
}

// Each node reads its own preset, including the first frame after a mode switch.
// Eligibility is checked in the renderer. An inactive Fish view uses neither window.
func updateWindows(root, settings, liveWindow string) {
	for _, preset := range []struct {
		name, window string
		panoramic    bool
	}{
		{"kPanorama_kNormalVideo.json", "pano-window", true},
		{"kSATAngle_kFreeFrameVideo.json", "ff-window", false},
	} {
		if preset.window == liveWindow {
			continue
		}
		f, err := os.Open(filepath.Join(settings, preset.name))
		if err != nil {
			continue
		}
		var v framingConfig
		err = json.NewDecoder(io.LimitReader(f, 16384)).Decode(&v)
		f.Close()
		if err != nil || v.Format.Size == "" || v.Format.Aspect == "" || v.Speed.FPS == "" {
			continue
		}
		publishWindow(root, preset.window, frameWindow(v, preset.panoramic))
	}
}
func publishWindow(root, window, want string) {
	path := filepath.Join(root, window)
	if want == "" {
		if _, err := os.Lstat(path); err == nil {
			os.Remove(path)
		}
		return
	}
	if old, _ := os.Readlink(path); old == want {
		return
	}
	temp := path + ".next"
	os.Remove(temp)
	if os.Symlink(want, temp) == nil {
		if os.Rename(temp, path) != nil {
			os.Remove(temp)
		}
	}
}
func nativeFreeFrameRatio(ratio string) bool {
	switch ratio {
	case "kRatio1x1", "kRatio16x9", "kRatio9x16", "kRatio4x3", "kRatio3x4", "kRatio235x100":
		return true
	}
	return false
}
