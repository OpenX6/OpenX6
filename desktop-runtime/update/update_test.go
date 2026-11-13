package update

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixture(tag, name string) release {
	return release{Tag: "v" + tag, Assets: []Asset{{Name: name, Size: 3, State: "uploaded", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("abc"))), URL: "https://github.com/TestOrg/OpenX6/releases/download/v" + tag + "/" + name}}}
}
func TestSelect(t *testing.T) {
	const name = "OpenX6-Windows11-x64-native.zip"
	for _, tc := range []struct {
		name, version   string
		change          func(*release)
		available, fail bool
	}{
		{"upgrade", "1.9.9", nil, true, false}, {"same", "1.10.0", nil, false, false}, {"older", "2.0.0", nil, false, false},
		{"preview", "1.0.0", func(r *release) { r.Prerelease = true }, false, false},
		{"draft", "1.0.0", func(r *release) { r.Draft = true }, false, false},
		{"nonrelease", "1.0.0", func(r *release) { r.Tag = "main" }, false, false},
		{"duplicate", "1.0.0", func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) }, false, true},
		{"missing platform", "1.0.0", func(r *release) { r.Assets = nil }, false, true},
		{"wrong repo", "1.0.0", func(r *release) { r.Assets[0].URL = strings.ReplaceAll(r.Assets[0].URL, "TestOrg", "Elsewhere") }, false, true},
		{"missing hash", "1.0.0", func(r *release) { r.Assets[0].Digest = "" }, false, true},
		{"incomplete", "1.0.0", func(r *release) { r.Assets[0].State = "new" }, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixture("1.10.0", name)
			if tc.change != nil {
				tc.change(&r)
			}
			c, e := selectAsset(r, "TestOrg/OpenX6", tc.version, name)
			if (e != nil) != tc.fail || (c.Version != "") != tc.available {
				t.Fatalf("candidate=%v err=%v", c, e)
			}
		})
	}
}
func TestDownload(t *testing.T) {
	a := fixture("1.0.0", "x").Assets[0]
	for _, s := range []string{"", "ab", "abcd", "xxx", "abc"} {
		if (download(io.Discard, strings.NewReader(s), a) == nil) != (s == "abc") {
			t.Fatalf("size or digest check for %q", s)
		}
	}
	for _, s := range []string{"http://github.com/x", "https://example.org/x", "https://user@github.com/x", "https://github.com:444/x"} {
		u, _ := url.Parse(s)
		if allowedURL(u) {
			t.Fatal(s)
		}
	}
}

type zipEntry struct{ name, target string }

func zipFixture(t *testing.T, entries ...zipEntry) []*zip.File {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, x := range entries {
		h := &zip.FileHeader{Name: x.name}
		if x.target != "" {
			h.SetMode(os.ModeSymlink | 0777)
		}
		f, e := w.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		io.WriteString(f, x.target)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if e != nil {
		t.Fatal(e)
	}
	return z.File
}
func TestArchivePaths(t *testing.T) {
	for _, tc := range []struct {
		entries    []zipEntry
		mac, valid bool
	}{
		{[]zipEntry{{"App/a", ""}}, false, true}, {[]zipEntry{{"App/sub/link", "../a"}, {"App/a", ""}}, true, true},
		{[]zipEntry{{"App/../holder", ""}}, true, false}, {[]zipEntry{{"__MACOSX/../../bad", ""}}, true, false},
		{[]zipEntry{{"App/a", ""}, {"App/A", ""}}, true, false}, {[]zipEntry{{"App/bad", "../../outside"}}, true, false},
		{[]zipEntry{{"App/l", "a"}, {"App/l/out", ""}}, true, false}, {[]zipEntry{{"RELEASE_NOTES.md", "../outside"}}, true, false},
		{[]zipEntry{{"App/link", "dest"}}, false, false}, {[]zipEntry{{"App/aux:ads", ""}}, false, false},
	} {
		if (validateZip(zipFixture(t, tc.entries...), "App", tc.mac) == nil) != tc.valid {
			t.Fatalf("archive %v", tc.entries)
		}
	}
}

func TestNativeWindowsArchive(t *testing.T) {
	for _, tc := range []struct {
		entries []zipEntry
		valid   bool
	}{
		{[]zipEntry{{"OpenX6.exe", ""}, {"Reader/Python/python.exe", ""}}, true},
		{[]zipEntry{{"../ready", ""}}, false},
		{[]zipEntry{{"Reader/path", ""}, {"reader/PATH", ""}}, false},
		{[]zipEntry{{"Reader/link", "../outside"}}, false},
	} {
		if (unpackWindows(zipFixture(t, tc.entries...), t.TempDir()) == nil) != tc.valid {
			t.Fatalf("unexpected extraction result for %v", tc.entries)
		}
	}
	if archive := os.Getenv("OPENX6_TEST_WINDOWS_ARCHIVE"); archive != "" {
		z, err := zip.OpenReader(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer z.Close()
		dest := t.TempDir()
		if err = unpackWindows(z.File, dest); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"OpenX6.exe", "OpenX6.Desktop.exe", "desktop-release.json", "Native/build_native.py"} {
			if _, err = os.Stat(filepath.Join(dest, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise a fresh desktop ZIP against an in-memory release API.
func TestPackagedUpdate(t *testing.T) {
	key := map[string]string{"darwin": "OPENX6_TEST_MAC_ARCHIVE", "windows": "OPENX6_TEST_WINDOWS_ARCHIVE"}[runtime.GOOS]
	archive := os.Getenv(key)
	if key == "" || archive == "" {
		t.Skip("supply a packaged release for this operating system")
	}
	oldClient, oldRepo, oldVersion := client, Repository, Version
	t.Cleanup(func() { client, Repository, Version = oldClient, oldRepo, oldVersion })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "Local"))
	Repository, Version = "OpenX6/OpenX6", "0.9.0"
	name, _, _ := platform()
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	n, e := io.Copy(h, f)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	r := fixture("1.0.0", name)
	a := &r.Assets[0]
	a.Size = n
	a.Digest = fmt.Sprintf("sha256:%x", h.Sum(nil))
	a.URL = strings.ReplaceAll(a.URL, "TestOrg/", "OpenX6/")
	bad := true
	client = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var body io.ReadCloser
		switch req.URL.String() {
		case "https://api.github.com/repos/OpenX6/OpenX6/releases/latest":
			data, _ := json.Marshal(r)
			body = io.NopCloser(bytes.NewReader(data))
		case a.URL:
			if bad {
				body = io.NopCloser(strings.NewReader("corrupt"))
			} else {
				var err error
				body, err = os.Open(archive)
				if err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: body, Header: http.Header{}, Request: req}, nil
	})}
	config, _ := os.UserConfigDir()
	holder := filepath.Join(config, "OpenX6", "Holder", "identifier")
	if runtime.GOOS == "windows" {
		holder = filepath.Join(home, "Local", "OpenX6", "Native", "Holder", "identifier")
	}
	if e = os.MkdirAll(filepath.Dir(holder), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(holder, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	if v, e := Check(); v != "1.0.0" || e != nil {
		t.Fatal(v, e)
	}
	if _, e = Install("1.0.1"); e == nil {
		t.Fatal("accepted a changed release")
	}
	if _, e = Install("1.0.0"); e == nil || Active() != "" {
		t.Fatal("accepted corrupt archive")
	}
	bad = false
	target, e := Install("1.0.0")
	if e != nil {
		t.Fatal(e)
	}
	if Active() != "" {
		t.Fatal("activated before launch")
	}
	// Launch Services may refuse a quarantined ad-hoc QA bundle. Preserve quarantine
	// and exercise the app-ready callback exactly as the packaged helper calls it.
	if runtime.GOOS == "darwin" {
		app := filepath.Dir(filepath.Dir(filepath.Dir(target)))
		if out, err := exec.Command("/usr/bin/xattr", "-p", "com.apple.quarantine", app).CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	} else {
		if b, err := os.ReadFile(target + ":Zone.Identifier"); err != nil || !bytes.Contains(b, []byte("ZoneId=3")) {
			t.Fatal("download origin missing", err)
		}
	}
	Version = "1.0.0"
	if err := Ready(os.Args[0]); err != nil {
		t.Fatal(err)
	}
	Version = "0.9.0"
	if Active() != "" {
		t.Fatal("foreign binary activated update")
	}
	Version = "1.0.0"
	if runtime.GOOS == "windows" {
		if out, err := exec.Command(target, "--desktop-ready").CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	} else if err := Ready(target); err != nil {
		t.Fatal(err)
	}
	Version = "0.9.0"
	if Active() != target {
		t.Fatal("did not activate downloaded application")
	}
	if b, e := os.ReadFile(holder); e != nil || string(b) != "original" {
		t.Fatal("holder changed")
	}
}
