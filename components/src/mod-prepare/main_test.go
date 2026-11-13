package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReconstruct(t *testing.T) {
	b := []byte("abcdef")
	v := Variant{BaseSHA: sum(b), Size: 6, SHA: sum([]byte("abXYef")), Edits: []Edit{{2, sum([]byte("cd")), []byte("XY")}}}
	o, e := reconstruct(b, v)
	if e != nil || string(o) != "abXYef" {
		t.Fatal(e)
	}
	bad := v
	bad.BaseSHA = sum([]byte("wrong"))
	if _, e = reconstruct(b, bad); e == nil {
		t.Fatal("wrong base")
	}
	bad = v
	bad.SHA = bad.BaseSHA
	if _, e = reconstruct(b, bad); e == nil {
		t.Fatal("wrong target")
	}
	bad = v
	bad.Edits = append(append([]Edit{}, v.Edits...), v.Edits[0])
	if _, e = reconstruct(b, bad); e == nil {
		t.Fatal("overlap")
	}
	bad = v
	bad.Edits = []Edit{{5, sum([]byte("f")), []byte("XYZ")}}
	if _, e = reconstruct(b, bad); e == nil {
		t.Fatal("bounds")
	}
}
func TestPreferences(t *testing.T) {
	r := t.TempDir() // Test root is beneath /tmp on Linux; avoid requiring its shared parent private.
	for _, p := range preferences {
		path := filepath.Join(r, p.Path)
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte{p.Max}, 0600)
		b, e := read(path, 2)
		if e != nil || len(b) != 1 || b[0] != p.Max {
			t.Fatal(e)
		}
		if e = atomic(path, []byte{p.Default}, 0600, false); e != nil {
			t.Fatal(e)
		}
		b, _ = read(path, 2)
		if b[0] != p.Default {
			t.Fatal("save")
		}
	}
	path := filepath.Join(r, "link")
	os.Symlink(filepath.Join(r, preferences[0].Path), path)
	if _, e := read(path, 2); e == nil {
		t.Fatal("symlink read")
	}
	if e := atomic(path, []byte{0}, 0600, false); e == nil {
		t.Fatal("symlink write")
	}
	unsafe := filepath.Join(r, "unsafe")
	os.WriteFile(unsafe, []byte{0}, 0666)
	os.Chmod(unsafe, 0666)
	if _, e := read(unsafe, 2); e == nil {
		t.Fatal("unsafe mode")
	}
}

func TestFreshAndMigratedPreferences(t *testing.T) {
	r := t.TempDir()
	for _, p := range preferences {
		v, e := preference(r, p)
		if e != nil || v != p.Default {
			t.Fatalf("fresh %v %v", v, e)
		}
		path := filepath.Join(r, p.Path)
		if e = atomic(path, []byte{p.Max}, 0600, false); e != nil {
			t.Fatal(e)
		}
		v, e = preference(r, p)
		want := p.Max
		if p.Path == preferences[1].Path || p.Path == preferences[2].Path { want = 0 }
		if e != nil || v != want {
			t.Fatal("existing preference replaced", e)
		}
		os.WriteFile(path, []byte{p.Max + 1}, 0600)
		if _, e = preference(r, p); e == nil {
			t.Fatal("invalid preference accepted")
		}
		os.Remove(path)
		os.Symlink(filepath.Join(r, "missing"), path)
		if _, e = preference(r, p); e == nil {
			t.Fatal("symlink initialized")
		}
	}
}
func TestStartupLifecycle(t *testing.T) {
	old := manifest
	defer func() { manifest = old }()
	for _, kind := range []string{"fresh", "saved", "bad-base", "bad-pref", "symlink", "mount-failure", "repeat", "platform"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := []byte("source")
			vs := []Variant{}
			for _, r := range []struct {
				base  string
				names []string
				dir   string
			}{
				{"/usr/lib/hw/camera.qcom.so", []string{"native.so", "off.so"}, "/run/x6-hnr-v1"},
				{"/usr/lib/libinsiqsetting.so", []string{"native.so", "medium.so", "low.so", "minimal.so"}, "/run/x6-video-nr-v1"},
				{"/usr/lib/ae1.so", []string{"ae1"}, "/run/x6-stereo-payload"}, {"/usr/lib/ae2.so", []string{"ae2"}, "/run/x6-stereo-payload"}, {"/usr/lib/ae.config", []string{"ae"}, "/run/x6-stereo-payload"},
			} {
				p := filepath.Join(root, r.base)
				os.MkdirAll(filepath.Dir(p), 0700)
				os.WriteFile(p, source, 0644)
				for _, n := range r.names {
					vs = append(vs, Variant{Base: r.base, Runtime: r.dir + "/" + n, BaseSHA: sum(source), SHA: sum(source), Size: len(source)})
				}
			}
			manifest, _ = json.Marshal(vs)
			if kind == "saved" || kind == "bad-pref" {
				for _, p := range preferences {
					path := filepath.Join(root, p.Path)
					os.MkdirAll(filepath.Dir(path), 0700)
					os.WriteFile(path, []byte{p.Max}, 0600)
				}
				if kind == "bad-pref" {
					os.WriteFile(filepath.Join(root, preferences[0].Path), []byte{9}, 0600)
				}
			}
			if kind == "bad-base" {
				os.WriteFile(filepath.Join(root, vs[0].Base), []byte("corrupt"), 0644)
			}
			if kind == "symlink" {
				p := filepath.Join(root, preferences[0].Path)
				os.MkdirAll(filepath.Dir(p), 0700)
				os.Symlink(filepath.Join(root, "outside"), p)
			}
			mounted := map[string]bool{}
			count := 0
			run := func(n string, a ...string) error {
				if n == "/bin/umount" {
					delete(mounted, a[0])
					return nil
				}
				if n != "/bin/mount" {
					t.Fatal("unexpected call", n)
				}
				if a[0] == "--bind" {
					count++
					if kind == "mount-failure" && count == 2 {
						return fmt.Errorf("injected failure")
					}
					if (a[2] == filepath.Join(root, "/usr/lib/hw/camera.qcom.so") || a[2] == filepath.Join(root, "/usr/lib/libinsiqsetting.so")) && filepath.Base(a[1]) != "native.so" { t.Fatal("retired NR applied", a) }
					mounted[a[2]] = true
				}
				return nil
			}
			check := func() error {
				if kind == "platform" {
					return fmt.Errorf("hold absent")
				}
				return nil
			}
			err := prepareAt(root, run, check, false)
			want := kind == "fresh" || kind == "saved" || kind == "repeat"
			if (err == nil) != want {
				t.Fatalf("%s: %v", kind, err)
			}
			_, ready := os.Stat(filepath.Join(root, "/run/x6-mod-controls/ready"))
			if (ready == nil) != want {
				t.Fatal("ready mismatch")
			}
			if !want && len(mounted) != 0 {
				t.Fatal("mount leak")
			}
			if kind == "saved" {
				if len(mounted) != 5 {
					t.Fatal("selection count", mounted)
				}
				for _, p := range preferences {
					// Saved bytes are never rewritten at boot (shared with the other slot);
					// retired controls are only interpreted as native in the runtime state.
					b, e := read(filepath.Join(root, p.Path), 2)
					if e != nil || b[0] != p.Max {
						t.Fatal("saved choice changed")
					}
					want := p.Max
					if p.Path == preferences[1].Path || p.Path == preferences[2].Path {
						want = 0
					}
					s, e := read(filepath.Join(root, p.State), 2)
					if e != nil || s[0] != want {
						t.Fatalf("runtime state for %s: %v %v", p.Path, s, e)
					}
				}
			}
			if kind == "fresh" && len(mounted) != 2 {
				t.Fatal("fresh defaults")
			}
			if kind == "repeat" {
				if e := prepareAt(root, run, check, false); e == nil {
					t.Fatal("repeat unexpectedly changed mounts")
				}
			}
		})
	}
}

func TestAppendedCode(t *testing.T) {
	b := []byte("abcdef")
	for _, v := range []Variant{
		{BaseSHA: sum(b), Size: 9, SHA: sum([]byte("abcdefXYZ")), Edits: []Edit{{6, sum(nil), []byte("XYZ")}}},
		{BaseSHA: sum(b), Size: 8, SHA: sum([]byte("abcdeXYZ")), Edits: []Edit{{5, sum([]byte("f")), []byte("XYZ")}}},
	} {
		if _, e := reconstruct(b, v); e != nil {
			t.Fatal(e)
		}
		v.Edits[0].Before = sum([]byte("wrong"))
		if _, e := reconstruct(b, v); e == nil {
			t.Fatal("wrong original append hash accepted")
		}
	}
}
func TestGeneratedVariants(t *testing.T) {
	root := os.Getenv("X6_MOD_BASE_DIR")
	if root == "" {
		t.Skip("generated official-derived fixtures required")
	}
	var vs []Variant
	if e := json.Unmarshal(manifest, &vs); e != nil {
		t.Fatal(e)
	}
	if len(vs) != 9 {
		t.Fatal("expected 9 production variants", len(vs))
	}
	for _, v := range vs {
		b, e := os.ReadFile(filepath.Join(root, v.Base))
		if e != nil {
			t.Fatal(e)
		}
		out, e := reconstruct(b, v)
		if e != nil {
			t.Fatalf("%s: %v", v.Runtime, e)
		}
		if sum(out) != v.SHA {
			t.Fatal("target hash")
		}
		t.Log(v.Runtime, v.SHA)
	}
}

func TestRetiredVideoPreferencesAllValues(t *testing.T) {
 for _, p := range preferences[1:3] {
  for old := byte(0); old <= p.Max; old++ {
   root := t.TempDir(); path := filepath.Join(root,p.Path)
   os.MkdirAll(filepath.Dir(path),0700);os.WriteFile(path,[]byte{old},0600)
   got,err:=preference(root,p);if err!=nil||got!=0{t.Fatalf("%s/%d: %d %v",p.Path,old,got,err)}
   // Interpreted as native, but the owner's saved byte is not rewritten: the
   // release in the other slot must still find its own value after a fallback.
   saved,err:=os.ReadFile(path);if err!=nil||len(saved)!=1||saved[0]!=old{t.Fatalf("saved preference rewritten: %v %v",saved,err)}
   if got,err=preference(root,p);err!=nil||got!=0{t.Fatal("not idempotent")}
  }
 }
}
