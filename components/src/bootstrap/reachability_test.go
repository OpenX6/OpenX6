package main

import (
	"os"
	"strings"
	"testing"
)

// The native settings that keep the camera reachable over SSH must not depend
// on the feature seed: a failed or skipped seed still yields Wi-Fi always on.
func TestNativeSettingsApplyWhenFeatureSeedFails(t *testing.T) {
	f := fixture(t)
	put(t, f.e.path(seedPath+"/manifest.json"), []byte("{corrupt"), 0644)
	if f.e.Run() == nil {
		t.Fatal("corrupt seed must fail the feature layer")
	}
	m := configFixture()
	if err := f.e.Bootstrap(m); err != nil {
		t.Fatal("settings blocked by the feature layer:", err)
	}
	if string(m.data["kNetdConfig"]["wifi_enable_strategy"]) != `"kAlwaysOn"` || string(m.data["kSystemAutoShutdown"]["auto_power_off_time"]) != "0" {
		t.Fatalf("reachability settings not applied: %v", m.data)
	}
	if _, err := os.Lstat(f.e.path(featurePath)); !os.IsNotExist(err) {
		t.Fatal("failed feature seed left a feature namespace")
	}
	untouched(t, f.e)
}

func TestNativeSettingsApplyWhenBreakerSkipsFeatures(t *testing.T) {
	f := fixture(t)
	m := configFixture()
	if err := f.e.BootstrapWithoutFeatures(m); err != nil {
		t.Fatal(err)
	}
	if string(m.data["kNetdConfig"]["wifi_enable_strategy"]) != `"kAlwaysOn"` {
		t.Fatal("reachability settings not applied")
	}
	if _, err := os.Lstat(f.e.path(featurePath)); !os.IsNotExist(err) {
		t.Fatal("feature seed ran although the breaker skipped features")
	}
	if _, err := os.Lstat(f.e.path(seedPath + "/manifest.json")); err != nil {
		t.Fatal("seed inputs must stay for a later boot")
	}
	// Once features are allowed again the seed is applied, settings untouched.
	m.data["kSystemAutoShutdown"]["auto_power_off_time"] = []byte(`300`)
	if err := f.e.Bootstrap(m); err != nil {
		t.Fatal(err)
	}
	if err := f.e.verifyFinal(f.m); err != nil {
		t.Fatal("feature seed not applied once allowed:", err)
	}
	if string(m.data["kSystemAutoShutdown"]["auto_power_off_time"]) != "300" {
		t.Fatal("user choice reset")
	}
	untouched(t, f.e)
}

// An adoption receipt from another release still means "preserve".
func TestPreservationReceiptFromAnotherReleaseStillPreserves(t *testing.T) {
	f := fixture(t)
	if err := f.e.makeDir(featurePath); err != nil {
		t.Fatal(err)
	}
	put(t, f.e.path(featurePath+"/preserve-native-settings"), []byte(strings.Repeat("9", 64)+"\n"), 0600)
	m := configFixture()
	for _, run := range []func(ConfigIO) error{f.e.BootstrapWithoutFeatures, f.e.Bootstrap} {
		if err := run(m); err != nil {
			t.Fatal(err)
		}
		if m.journal != nil || string(m.data["kNetdConfig"]["wifi_enable_strategy"]) != `"kAuto"` {
			t.Fatal("preserved settings were initialized")
		}
	}
}

func TestPreservationReceiptMustBeAReceipt(t *testing.T) {
	for _, body := range []string{"", "yes\n", strings.Repeat("9", 63) + "\n", strings.Repeat("9", 64) + "\n" + strings.Repeat("8", 64) + "\n"} {
		f := fixture(t)
		if err := f.e.makeDir(featurePath); err != nil {
			t.Fatal(err)
		}
		put(t, f.e.path(featurePath+"/preserve-native-settings"), []byte(body), 0600)
		m := configFixture()
		if f.e.BootstrapWithoutFeatures(m) == nil {
			t.Fatalf("malformed receipt %q accepted", body)
		}
		if m.journal != nil {
			t.Fatal("settings initialized despite a malformed receipt")
		}
	}
}

func TestBootGuardMarkerPathMatchesBootGuard(t *testing.T) {
	b, err := os.ReadFile("../boot-guard/state.go")
	if err != nil {
		t.Skip("boot-guard source not beside bootstrap")
	}
	if !strings.Contains(string(b), `runDir      = "/run/x6-boot"`) || !strings.Contains(string(b), `allowName   = "features-allowed"`) {
		t.Fatal("boot-guard marker path changed; update bootGuardAllow")
	}
	if bootGuardAllow != "run/x6-boot/features-allowed" {
		t.Fatal("bootstrap marker path differs from boot-guard")
	}
}
