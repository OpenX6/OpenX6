package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type cameraHeat struct{ Sensor1, Sensor2, Battery string }

func socDegrees(value float64) string {
	if value > -20 && value < 150 {
		return fmt.Sprintf("%.0f", value)
	}
	return "--"
}

func unknownHeat() cameraHeat { return cameraHeat{"--", "--", "--"} }
func degrees(v *float64, zeroInactive bool) string {
	if v == nil || *v < -35 || *v > 125 || (zeroInactive && *v == 0) {
		return "--"
	}
	return fmt.Sprintf("%.0f", *v)
}
func parseCameraHeat(data []byte) cameraHeat {
	var r struct {
		Ret  string
		Data struct {
			Present bool     `json:"battery_is_present"`
			Battery *float64 `json:"battery_temperature_c"`
			Dev     struct {
				Main   *float64 `json:"main_sensor_temperature_c"`
				Second *float64 `json:"secondary_camera_sensor_temp_c"`
			} `json:"dev_temperature"`
		}
	}
	t := unknownHeat()
	if json.Unmarshal(data, &r) != nil || r.Ret != "ok" {
		return t
	}
	t.Sensor1 = degrees(r.Data.Dev.Main, true)
	t.Sensor2 = degrees(r.Data.Dev.Second, true)
	if r.Data.Present {
		t.Battery = degrees(r.Data.Battery, false)
	}
	return t
}
func heatText(text string, expanded bool, heat cameraHeat) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, "SoC ") {
			continue
		}
		if !expanded {
			lines[i] = l
			break
		}
		lines[i] = l + "\nS1 " + heat.Sensor1 + "  S2 " + heat.Sensor2 + "  Batt " + heat.Battery + " C"
		break
	}
	return strings.Join(lines, "\n")
}
