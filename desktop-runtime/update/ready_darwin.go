package update

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var codeSignOutput = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "/usr/bin/codesign", args...).CombinedOutput()
}

func readyApp(executable string) string {
	p, err := filepath.EvalSymlinks(executable)
	if err != nil || filepath.Base(p) != "OpenX6" || filepath.Base(filepath.Dir(p)) != "MacOS" {
		return ""
	}
	contents := filepath.Dir(filepath.Dir(p))
	app := filepath.Dir(contents)
	if filepath.Base(contents) != "Contents" || filepath.Base(app) != "OpenX6.app" {
		return ""
	}
	return app
}

// Translocation mounts the same app on another device, so inode identity alone
// cannot confirm startup. Verify both complete sealed bundles before comparing
// their code-directory identity. Matching only the executable would miss changed
// resources or helpers. This does not remove quarantine or bypass Gatekeeper.
func matchingReadyBundle(target, running string) (bool, error) {
	a, b := readyApp(target), readyApp(running)
	if a == "" || b == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	identify := func(app string) (string, error) {
		requirement := "anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists"
		if _, err := codeSignOutput(ctx, "--verify", "--deep", "--strict", "--all-architectures", "-R="+requirement, app); err != nil {
			return "", fmt.Errorf("cannot verify updated app bundle: %w", err)
		}
		out, err := codeSignOutput(ctx, "--display", "--verbose=4", app)
		if err != nil {
			return "", fmt.Errorf("cannot read updated app identity: %w", err)
		}
		fields := map[string]string{}
		for _, line := range strings.Split(string(out), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok || (key != "CDHash" && key != "TeamIdentifier" && key != "Identifier") {
				continue
			}
			if _, duplicate := fields[key]; duplicate {
				return "", fmt.Errorf("duplicate signed app identity field")
			}
			fields[key] = value
		}
		if fields["Identifier"] != "OpenX6" || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(fields["CDHash"]) || !regexp.MustCompile(`^[A-Z0-9]{10}$`).MatchString(fields["TeamIdentifier"]) {
			return "", fmt.Errorf("invalid signed app identity")
		}
		return fields["TeamIdentifier"] + ":" + fields["CDHash"], nil
	}
	x, err := identify(a)
	if err != nil {
		return false, err
	}
	y, err := identify(b)
	if err != nil {
		return false, err
	}
	return x == y, nil
}
