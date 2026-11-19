// Package update installs stable desktop releases into the current user's application directory.
package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The packager sets both fields together for a release. Offline builds have no feed.
var Repository string
var Version = "0.0.0"
var repoName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*/[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var semver = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

const maxArchive = 2 << 30

func Identity() string { return Repository + "@" + Version }
func newer(next, base string) bool {
	if !semver.MatchString(next) || !semver.MatchString(base) {
		return false
	}
	a, b := strings.Split(next, "."), strings.Split(base, ".")
	for i := range a {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return false
}

type Asset struct {
	Name   string
	Size   int64
	Digest string
	State  string
	URL    string `json:"browser_download_url"`
}
type release struct {
	Tag               string `json:"tag_name"`
	Draft, Prerelease bool
	Assets            []Asset
}
type candidate struct {
	Version string
	Asset
}

func platform() (archive, folder, entry string) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return "OpenX6-macOS-arm64.zip", "OpenX6.app", "Contents/MacOS/OpenX6"
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return "OpenX6-Windows11-x64-native.zip", "OpenX6-Windows11-x64", "OpenX6.exe"
	}
	return "", "", ""
}
func selectAsset(r release, repo, current, name string) (candidate, error) {
	v, has := strings.CutPrefix(r.Tag, "v")
	if r.Draft || r.Prerelease || !has || !newer(v, current) {
		return candidate{}, nil
	}
	var found []Asset
	for _, a := range r.Assets {
		if a.Name == name {
			found = append(found, a)
		}
	}
	if len(found) != 1 {
		return candidate{}, errors.New("release is missing a unique desktop package")
	}
	a := found[0]
	digest, ok := strings.CutPrefix(a.Digest, "sha256:")
	d, e := hex.DecodeString(digest)
	if !ok || e != nil || len(d) != 32 || a.Size <= 0 || a.Size > maxArchive || a.State != "uploaded" || a.URL != "https://github.com/"+repo+"/releases/download/"+r.Tag+"/"+name {
		return candidate{}, errors.New("invalid release asset or SHA-256")
	}
	return candidate{v, a}, nil
}
func allowedURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}

var client = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) > 5 || !allowedURL(req.URL) {
		return errors.New("unexpected release redirect")
	}
	return nil
}}

func get(ctx context.Context, address string) (io.ReadCloser, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, e
	}
	if !allowedURL(req.URL) {
		return nil, errors.New("invalid release URL")
	}
	req.Header.Set("User-Agent", "OpenX6-Desktop")
	res, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("GitHub returned %s", res.Status)
	}
	return res.Body, nil
}
func lookup() (candidate, error) {
	if Repository == "" {
		return candidate{}, nil
	}
	name, _, _ := platform()
	if !repoName.MatchString(Repository) || name == "" || !semver.MatchString(Version) {
		return candidate{}, errors.New("invalid desktop release configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	body, e := get(ctx, "https://api.github.com/repos/"+Repository+"/releases/latest")
	if e != nil {
		return candidate{}, e
	}
	defer body.Close()
	data, e := io.ReadAll(io.LimitReader(body, 2<<20))
	if e != nil {
		return candidate{}, e
	}
	var r release
	if e = json.Unmarshal(data, &r); e != nil {
		return candidate{}, e
	}
	return selectAsset(r, Repository, Version, name)
}
func Check() (string, error) { c, e := lookup(); return c.Version, e }
func download(dst io.Writer, src io.Reader, a Asset) error {
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, a.Size+1))
	if e != nil {
		return e
	}
	if n != a.Size || "sha256:"+hex.EncodeToString(h.Sum(nil)) != a.Digest {
		return errors.New("desktop download failed its size or SHA-256 check")
	}
	return nil
}
func store() (string, error) {
	if !repoName.MatchString(Repository) {
		return "", errors.New("desktop update source is not configured")
	}
	p, e := os.UserConfigDir()
	if runtime.GOOS == "windows" {
		p, e = os.UserCacheDir()
	}
	if e != nil {
		return "", e
	}
	return filepath.Join(p, "OpenX6", "DesktopUpdates", strings.ToLower(Repository)), nil
}
func activeVersion(dir string) string {
	entries, _ := os.ReadDir(dir)
	var best string
	for _, entry := range entries {
		v := entry.Name()
		if !entry.IsDir() || !semver.MatchString(v) || (best != "" && !newer(v, best)) {
			continue
		}
		marker, err := os.ReadFile(filepath.Join(dir, v, "ready"))
		if err == nil && string(marker) == v {
			best = v
		}
	}
	return best
}
func binary(dir, v string) string {
	_, folder, entry := platform()
	return filepath.Join(dir, v, folder, filepath.FromSlash(entry))
}
func Active() string {
	dir, e := store()
	if e != nil {
		return ""
	}
	v := activeVersion(dir)
	if !newer(v, Version) {
		return ""
	}
	target := binary(dir, v)
	info, e := os.Stat(target)
	if e != nil || !info.Mode().IsRegular() {
		return ""
	}
	return target
}
func validateZip(files []*zip.File, folder string, mac bool) error {
	names := make(map[string]bool)
	links := make(map[string]bool)
	var total uint64
	for _, f := range files {
		n := strings.TrimSuffix(f.Name, "/")
		inside := (!mac && folder == "") || n == folder || strings.HasPrefix(n, folder+"/")
		note := mac && n == "RELEASE_NOTES.md" && f.Mode().IsRegular()
		if !filepath.IsLocal(filepath.FromSlash(n)) || strings.ContainsAny(n, "\\:\x00") || path.Clean(n) != n || (!inside && !note) {
			return errors.New("unexpected path in release")
		}
		for _, component := range strings.Split(n, "/") {
			if component == "" || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
				return errors.New("ambiguous release path")
			}
		}
		key := strings.ToLower(n)
		if names[key] {
			return errors.New("duplicate file in release")
		}
		names[key] = true
		if f.UncompressedSize64 > 6<<30 || total+f.UncompressedSize64 > 8<<30 || len(files) > 150000 {
			return errors.New("release archive exceeds size limit")
		}
		total += f.UncompressedSize64
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 && mac {
			r, e := f.Open()
			if e != nil {
				return e
			}
			b, e := io.ReadAll(io.LimitReader(r, 4097))
			r.Close()
			if e != nil {
				return e
			}
			link := string(b)
			// Only leading parent components are needed by bundle framework links.
			tail := strings.TrimPrefix(link, "./")
			for strings.HasPrefix(tail, "../") {
				tail = strings.TrimPrefix(tail, "../")
			}
			if tail == "" || tail == "." || tail == ".." || path.Clean(tail) != tail || strings.Contains("/"+tail+"/", "/../") {
				return errors.New("ambiguous release symlink")
			}
			links[key] = true
			dest := path.Join(path.Dir(n), link)
			if link == "" || len(b) > 4096 || path.IsAbs(link) || strings.ContainsAny(link, "\\:\x00") || !strings.HasPrefix(dest, folder+"/") {
				return errors.New("unsafe release symlink")
			}
		} else if !mode.IsRegular() && !mode.IsDir() {
			return errors.New("unsupported release file")
		}
	}
	for name := range names {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if links[parent] {
				return errors.New("release entry traverses a symlink")
			}
		}
	}
	return nil
}
func unpack(archive, stage, folder string) error {
	z, e := zip.OpenReader(archive)
	if e != nil {
		return e
	}
	defer z.Close()
	if runtime.GOOS == "windows" {
		return unpackWindows(z.File, filepath.Join(stage, folder))
	}
	if e = validateZip(z.File, folder, true); e != nil {
		return e
	}
	if e = exec.Command("/usr/bin/ditto", "-x", "-k", archive, stage).Run(); e != nil {
		return e
	}
	app := filepath.Join(stage, folder)
	if e = exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", app).Run(); e != nil {
		return errors.New("macOS application signature verification failed")
	}
	// Let Launch Services apply its normal downloaded-app assessment.
	return exec.Command("/usr/bin/xattr", "-w", "com.apple.quarantine", fmt.Sprintf("0081;%x;OpenX6;", time.Now().Unix()), app).Run()
}

// The native Windows ZIP is flat. Extract it inside its version directory,
// keeping package files separate from the download receipt and ready marker.
func unpackWindows(files []*zip.File, stage string) error {
	if err := validateZip(files, "", false); err != nil {
		return err
	}
	for _, f := range files {
		var e error
		dest := filepath.Join(stage, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if e = os.MkdirAll(dest, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		r, e := f.Open()
		if e == nil {
			_, e = io.Copy(out, r)
			r.Close()
		}
		closeErr := out.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func Install(wanted string) (string, error) {
	c, e := lookup()
	if e != nil {
		return "", e
	}
	if c.Version == "" || c.Version != wanted {
		return "", errors.New("release changed; check for updates again")
	}
	dir, e := store()
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	work, e := os.MkdirTemp(dir, ".download-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(work)
	archive := filepath.Join(work, "release.zip")
	f, e := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	body, e := get(ctx, c.URL)
	if e == nil {
		e = download(f, body, c.Asset)
		body.Close()
	}
	ce := f.Close()
	if e != nil {
		return "", e
	}
	if ce != nil {
		return "", ce
	}
	_, folder, _ := platform()
	stage := filepath.Join(work, "unpacked")
	if e = os.Mkdir(stage, 0700); e != nil {
		return "", e
	}
	if e = unpack(archive, stage, folder); e != nil {
		return "", e
	}
	// Both packaging paths embed this identity beside the executable.
	id := filepath.Join(stage, folder, "desktop-release.json")
	if runtime.GOOS == "darwin" {
		id = filepath.Join(stage, folder, "Contents", "Resources", "desktop-release.json")
	}
	b, e := os.ReadFile(id)
	var stamp struct{ Repository, Version string }
	if e != nil || json.Unmarshal(b, &stamp) != nil || stamp.Repository != Repository || stamp.Version != wanted {
		return "", errors.New("downloaded application identity differs from its release")
	}
	target := binary(dir, wanted)
	executable := filepath.Join(stage, folder, filepath.Base(target))
	if runtime.GOOS == "darwin" {
		executable = filepath.Join(stage, folder, "Contents", "MacOS", "OpenX6")
	}
	if st, e := os.Stat(executable); e != nil || !st.Mode().IsRegular() {
		return "", errors.New("release executable missing")
	}
	if runtime.GOOS == "windows" {
		if e = os.WriteFile(executable+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0600); e != nil {
			return "", fmt.Errorf("mark downloaded executable: %w", e)
		}
	}
	// Install beside the old app. Interruptions before activation keep the previous selection.
	receipt := filepath.Join(stage, "archive.sha256")
	if e = os.WriteFile(receipt, []byte(c.Digest), 0600); e != nil {
		return "", e
	}
	if e = os.Rename(stage, filepath.Join(dir, wanted)); e != nil {
		saved, err := os.ReadFile(filepath.Join(dir, wanted, "archive.sha256"))
		if err != nil || string(saved) != c.Digest {
			return "", e
		}
	}
	return target, nil
}

// Only an installed version which actually opens its UI becomes the default.
// Reopening the original still works after a canceled OS launch or a failed startup.
func Ready(executable string) error {
	if Repository == "" {
		return nil
	}
	dir, e := store()
	if e != nil {
		return e
	}
	if !semver.MatchString(Version) {
		return errors.New("invalid running desktop version")
	}
	// A successful first launch already selected this version. Do not repeat
	// complete bundle verification on every later GUI startup.
	if marker, err := os.ReadFile(filepath.Join(dir, Version, "ready")); err == nil && string(marker) == Version {
		return nil
	}
	target := binary(dir, Version)
	same, e := os.Stat(target)
	if e != nil {
		return nil
	}
	current, e := os.Stat(executable)
	if e != nil || !current.Mode().IsRegular() || !same.Mode().IsRegular() {
		return nil
	}
	if !os.SameFile(current, same) {
		matches, err := matchingReadyBundle(target, executable)
		if err != nil {
			return err
		}
		if !matches {
			return nil
		}
	}
	if _, e = os.Stat(filepath.Join(dir, Version, "archive.sha256")); e != nil {
		return e
	}
	// Version-local readiness prevents an older instance from replacing a newer choice.
	return os.WriteFile(filepath.Join(dir, Version, "ready"), []byte(Version), 0600)
}
