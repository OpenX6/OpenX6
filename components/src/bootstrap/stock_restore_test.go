package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stockFixture(t *testing.T, partial bool) (testFixture, string) {
	f, _ := migrationFixture(t)
	f.e.Migration = &MigrationPolicy{LocalHolder: true}
	owners := map[string]string{}
	pins := map[string]string{}
	for _, role := range []string{"adb_keys", "ins/etc/x6_owner_authorized_keys", "ins/etc/x6_owner_gui_ed25519.pub", "ins/etc/x6_owner_factory_ed25519.pub", "ins/etc/x6_owner_update_ed25519.pub"} {
		b, _ := os.ReadFile(f.e.path(role))
		put(t, f.e.path("data/x6-holder-v1/"+filepath.Base(role)), b, 0600)
		pins[filepath.Base(role)] = digest(b)
		owners[role] = digest(b)
	}
	raw, _ := json.Marshal(pins)
	put(t, f.e.path("data/x6-holder-v1/pins.json"), raw, 0600)
	os.Remove(f.e.path(featurePath + "/vr180-v1/enabled"))
	os.Remove(f.e.path(featurePath + "/raw-nr-v1/enabled"))
	for i, v := range f.m.Entries {
		put(t, f.e.path(featurePath+"/"+v.Target), []byte("changed user program"), 0600)
		if partial && i%2 == 0 {
			os.Remove(f.e.path(featurePath + "/" + v.Target))
		}
	}
	if partial {
		os.Remove(f.e.path(featurePath + "/vr180-v1/sync"))
		os.Remove(f.e.path(featurePath + "/raw-nr-v1/level"))
		os.Remove(f.e.path(featurePath + "/complete.json"))
	}
	p := restorePlan{1, f.e.Pin, owners, map[string]restoreFile{}}
	targets := []string{"complete.json", "preserve-native-settings", "vr180-v1/sync", "raw-nr-v1/level"}
	for k := range policies {
		targets = append(targets, k)
	}
	for _, rel := range targets {
		v := restoreFile{}
		b, err := os.ReadFile(f.e.path(featurePath + "/" + rel))
		if err == nil {
			st, _ := os.Stat(f.e.path(featurePath + "/" + rel))
			v = restoreFile{digest(b), int64(len(b)), uint32(st.Mode().Perm())}
		}
		p.Files[rel] = v
	}
	raw, _ = json.Marshal(p)
	id := digest(raw)
	base := "data/x6-stock-restore-" + f.e.Pin
	put(t, f.e.path(base+"/"+id+"/plan.json"), raw, 0600)
	put(t, f.e.path(base+"/active"), []byte(id+".json\n"), 0600)
	return f, id
}
func TestStockRestoreAndResume(t *testing.T) {
	points := []string{"", "stock-restore-ready", "stock-restore-preserved", "stock-restore-complete", "file:done"}
	for rel := range policies {
		points = append(points, "stock-restore-file:"+rel)
	}
	for _, partial := range []bool{false, true} {
		for _, point := range points {
			t.Run(strings.ReplaceAll(point, "/", "_")+map[bool]string{true: "partial", false: "complete"}[partial], func(t *testing.T) {
				f, id := stockFixture(t, partial)
				if point != "" {
					f.e.Fault = func(p string) error {
						if p == point {
							return errors.New("power loss")
						}
						return nil
					}
					if f.e.Run() == nil {
						t.Fatal("fault not reached")
					}
					f.e.Fault = nil
				}
				if err := f.e.Run(); err != nil {
					t.Fatal(err)
				}
				if err := f.e.Run(); err != nil {
					t.Fatal("second boot", err)
				}
				if err := f.e.verifyFinal(f.m); err != nil {
					t.Fatal(err)
				}
				untouched(t, f.e)
				for _, rel := range []string{"vr180-v1/enabled", "raw-nr-v1/enabled"} {
					if _, err := os.Stat(f.e.path(featurePath + "/" + rel)); !os.IsNotExist(err) {
						t.Fatal("Off reset", rel)
					}
				}
				for _, rel := range []string{"vr180-v1/sync", "raw-nr-v1/level"} {
					b, _ := os.ReadFile(f.e.path(featurePath + "/" + rel))
					want := byte(1)
					if rel == "raw-nr-v1/level" {
						want = 2
					}
					if partial {
						want = 0
					}
					if len(b) != 1 || b[0] != want {
						t.Fatal("preference changed", rel, b)
					}
				}
				if _, err := os.Stat(f.e.path("data/.x6-stock-payload-" + id)); !os.IsNotExist(err) {
					t.Fatal("staging retained")
				}
				// A consumed plan must never silently undo subsequent local program edits.
				put(t, f.e.path(featurePath+"/vr180-v1/gui"), []byte("later owner edit"), 0755)
				if err := f.e.Run(); err == nil {
					t.Fatal("consumed recovery plan replayed")
				}
			})
		}
	}
}
func TestStockRestoreRejectsChangedPlanOrState(t *testing.T) {
	for _, kind := range []string{"plan", "program", "owner", "base", "space"} {
		t.Run(kind, func(t *testing.T) {
			f, id := stockFixture(t, true)
			switch kind {
			case "plan":
				put(t, f.e.path("data/x6-stock-restore-"+f.e.Pin+"/"+id+"/plan.json"), []byte("{}"), 0600)
			case "program":
				put(t, f.e.path(featurePath+"/vr180-v1/gui"), []byte("late mutation"), 0755)
			case "owner":
				put(t, f.e.path("data/x6-holder-v1/x6_owner_gui_ed25519.pub"), []byte("different holder"), 0600)
			case "base":
				put(t, f.e.path("usr/bin/ins_gui"), []byte("wrong target base"), 0755)
			case "space":
				f.e.Free = func(string) (uint64, error) { return 1, nil }
			}
			if f.e.Run() == nil {
				t.Fatal("bad input accepted")
			}
			untouched(t, f.e)
		})
	}
}
