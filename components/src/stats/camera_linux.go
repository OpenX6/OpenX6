package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"time"
)

func recordingSource() (recordingProfile, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var r struct {
		Ret  string
		Data struct {
			Lens string `json:"lens_mode"`
			Mode string `json:"shoot_mode"`
		}
	}
	mode, err := exec.CommandContext(ctx, "/usr/bin/ins_bll_cli", `{"cmd":"config","action":"get_general","key":"kMediaMode"}`).Output()
	if err != nil || json.Unmarshal(mode, &r) != nil || r.Ret != "ok" {
		return recordingProfile{}, false
	}
	info, err := exec.CommandContext(ctx, "/usr/bin/ins_ibus_cli", "--run", "call-method", "kCamera", "kMediad", "Pipeline", "PipelineGetStreamInfo", "StreamTypeMain").Output()
	if err != nil {
		return recordingProfile{}, false
	}
	return pipelineProfile(string(info), r.Data.Lens, r.Data.Mode)
}

type heatReader struct {
	readAt time.Time
	heat   cameraHeat
}

func (h *heatReader) decorate(text string) string {
	_, err := os.Stat(runtimeDir + "/thermal-expanded")
	if err != nil {
		h.readAt = time.Time{}
		return heatText(text, false, cameraHeat{})
	}
	if time.Since(h.readAt) >= 2*time.Second {
		ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
		b, _ := exec.CommandContext(ctx, "/usr/bin/ins_bll_cli", `{"cmd":"share_state","action":"get","type":"hald"}`).Output()
		cancel()
		h.heat = parseCameraHeat(b)
		h.readAt = time.Now()
	}
	return heatText(text, true, h.heat)
}

var temperatures heatReader
