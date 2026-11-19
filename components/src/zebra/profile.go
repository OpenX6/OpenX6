package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// Use runtime color/LUT selections rather than assuming a log-looking preview is iLog.
type selection struct {
	Ret  string
	Data struct {
		Color  string `json:"color_mode"`
		Enable *bool  `json:"enable"`
		Shoot  string `json:"shoot_mode"`
	}
}

func queryProfile(ctx context.Context, k string) (selection, bool) {
	var s selection
	b, e := exec.CommandContext(ctx, "ins_bll_cli", `{"cmd":"config","action":"get_general","key":"`+k+`"}`).Output()
	ok := e == nil && json.Unmarshal(b, &s) == nil && s.Ret == "ok"
	return s, ok
}
func logPreview() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()
	m, ok := queryProfile(ctx, "kMediaMode")
	if !ok || (m.Data.Shoot != "kNormalVideo" && m.Data.Shoot != "kFreeFrameVideo") {
		return false
	}
	c, ok := queryProfile(ctx, "kIqColorMode")
	if !ok || c.Data.Color != "kIlog" {
		return false
	}
	l, ok := queryProfile(ctx, "kILogLut")
	return ok && l.Data.Enable != nil && !*l.Data.Enable
}
