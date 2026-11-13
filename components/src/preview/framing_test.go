package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpticalWindow(t *testing.T) {
	root := t.TempDir()
	settings := t.TempDir()
	save := func(name, stringdata string) {
		if err := os.WriteFile(filepath.Join(settings, name), []byte(stringdata), 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(ff, pan string) {
		updateWindows(root, settings, "")
		a, _ := os.Readlink(filepath.Join(root, "ff-window"))
		b, _ := os.Readlink(filepath.Join(root, "pano-window"))
		if a != ff || b != pan {
			t.Fatalf("got %q/%q want %q/%q", a, b, ff, pan)
		}
	}
	save("kSATAngle_kFreeFrameVideo.json", `{"kMediaResRatio":{"media_res":"kRes16K","media_ratio":"kRatio4x3"},"kMediaVideoFps":{"video_fps":"k60"}}`)
	save("kPanorama_kNormalVideo.json", `{"kMediaResRatio":{"media_res":"kRes2_7K","media_ratio":"kRatio1x1"},"kMediaVideoFps":{"video_fps":"k60"}}`)
	check("C", "P")
	save("kPanorama_kNormalVideo.json", "{")
	check("C", "P")
	save("kPanorama_kNormalVideo.json", `{"kMediaResRatio":{"media_res":"kRes4K","media_ratio":"kRatio1x1"},"kMediaVideoFps":{"video_fps":"k60"}}`)
	check("C", "")
}
func TestNative6KFraming(t *testing.T) {
	var v framingConfig
	v.Format.Size = "kRes6K"
	v.Speed.FPS = "k30"
	for _, ratio := range []string{"kRatio16x9", "kRatio9x16", "kRatio1x1", "kRatio4x3", "kRatio3x4", "kRatio235x100", "unknown"} {
		v.Format.Aspect = ratio
		want := "B"
		if ratio == "unknown" {
			want = ""
		}
		if got := frameWindow(v, false); got != want {
			t.Fatalf("%s: %q", ratio, got)
		}
	}
}
func TestFactoryPanoramaTiersAlwaysUseOriginalView(t *testing.T) {
	var v framingConfig
	for _, size := range []string{"kRes4K", "kRes6K"} {
		v.Format.Size = size
		for _, ratio := range []string{"kRatio1x1", "kRatio16x9", "kRatio4x3"} {
			v.Format.Aspect = ratio
			for _, fps := range []string{"k24", "k25", "k30", "k48", "k50", "k60", "k100", "k120", "k200"} {
				v.Speed.FPS = fps
				{
					if frameWindow(v, true) != "" {
						t.Fatalf("changed %s %s %s", size, ratio, fps)
					}
				}
			}
		}
	}
}

func TestLiveModelCropRoutes(t *testing.T) {
	for _, tc := range []struct {
		mode, lens, res, fps, ratio uint32
		window, tag                 string
	}{
		{2, 3, 7, 10, 3, "ff-window", "A"}, {2, 3, 7, 12, 3, "ff-window", "A"},
		{2, 3, 2, 10, 3, "ff-window", "B"}, {2, 3, 2, 12, 3, "ff-window", ""},
		{2, 3, 0, 7, 1, "ff-window", "C"}, {2, 3, 4, 5, 1, "ff-window", "D"},
		{0, 1, 5, 7, 3, "pano-window", "P"}, {0, 1, 7, 10, 3, "pano-window", "Q"},
		{0, 1, 0, 10, 3, "pano-window", "T"}, {0, 1, 2, 10, 3, "pano-window", ""},
		{0, 1, 4, 7, 3, "pano-window", ""}, {0, 3, 5, 7, 3, "", ""},
	} {
		w, v := modelWindow(tc.mode, tc.lens, tc.res, tc.fps, tc.ratio)
		if w != tc.window || v != tc.tag {
			t.Fatalf("%+v returned %q/%q", tc, w, v)
		}
	}
}
