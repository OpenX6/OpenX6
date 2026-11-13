// Windows front end for the bundled, offline WSL2 build environment.
package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"openx6.local/desktop/update"
	"os"
	"path/filepath"
	"strings"
)

// Set by the packager after verifying the build runtime.
var runtimeHash string
var workerVersion string

var appHash string

//go:embed upgrade.py
var bootstrap string

//go:embed builder.ps1
var builderWindow string

func checkFile(path, expected string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err == nil && hex.EncodeToString(h.Sum(nil)) == expected {
		if _, err = f.Seek(0, 0); err == nil {
			return f, nil
		}
	}
	f.Close()
	return nil, fmt.Errorf("checksum or read failure for %s; extract the complete release again", filepath.Base(path))
}

// WSL keeps registrations in the current user's Lxss key. A broken registered
// worker must never be mistaken for a fresh installation.
const registeredScript = `$ErrorActionPreference='Stop'
$key='HKCU:\Software\Microsoft\Windows\CurrentVersion\Lxss'
if (Test-Path -LiteralPath $key) {
  foreach ($sub in Get-ChildItem -LiteralPath $key) {
    if ($sub.GetValue('DistributionName') -eq 'OpenX6-Builder-v1') { [Console]::Write('yes'); exit 0 }
  }
}
[Console]::Write('no')`

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	expected, err := hex.DecodeString(runtimeHash)
	if err != nil || len(expected) != sha256.Size {
		return errors.New("Development launcher only. A verified Windows runtime is required.")
	}
	sys := filepath.Join(os.Getenv("SystemRoot"), "System32")
	shell := filepath.Join(sys, "WindowsPowerShell", "v1.0", "powershell.exe")
	wsl := filepath.Join(sys, "wsl.exe")
	if len(os.Args) == 2 && os.Args[1] == "--desktop-ready" {
		return update.Ready(exe)
	}
	if len(os.Args) == 2 && os.Args[1] == "--desktop-check" {
		version, err := update.Check()
		if err == nil {
			fmt.Print(version)
		}
		return err
	}
	launch := func(file string) error {
		// Use the Windows shell so downloaded executables receive the normal OS assessment.
		child := command(shell, "-NoProfile", "-Command", "$ErrorActionPreference='Stop'; Start-Process -FilePath $env:OPENX6_NEXT")
		child.Env = append(os.Environ(), "OPENX6_NEXT="+file)
		return child.Run()
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--ui-build":
			return build(exe, shell, wsl, []string{os.Getenv("OPENX6_FW"), os.Getenv("OPENX6_MODULES"), os.Getenv("OPENX6_OUTPUT")})
		case "--ui-update":
			target, err := update.Install(os.Getenv("OPENX6_UPDATE"))
			if err != nil {
				return err
			}
			return launch(target)
		case "--ui-inspect", "--ui-export", "--ui-deploy":
			return readerAction(exe, os.Args[1])
		default:
			return errors.New("Unknown desktop operation")
		}
	}
	if active := update.Active(); active != "" && launch(active) == nil {
		return nil
	}
	// Send the UTF-8 script as data, including for installations under non-ASCII paths.
	form := command(shell, "-NoProfile", "-NonInteractive", "-STA", "-Command",
		"$reader=[IO.StreamReader]::new([Console]::OpenStandardInput(),[Text.Encoding]::UTF8); & ([ScriptBlock]::Create($reader.ReadToEnd()))")
	form.Stdin = strings.NewReader(builderWindow)
	form.Env = append(os.Environ(), "OPENX6_SELF="+exe, "OPENX6_REPOSITORY="+update.Repository)
	out, err := form.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Could not open OpenX6: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readerAction(exe, action string) error {
	dir := filepath.Join(filepath.Dir(exe), "Reader")
	script := "export_modules.py"
	args := []string{os.Getenv("OPENX6_FW")}
	switch action {
	case "--ui-inspect":
		args = append(args, "--inspect")
	case "--ui-deploy":
		script = "deploy_sd.py"
		args = []string{filepath.Join(os.Getenv("OPENX6_OUTPUT"), "OpenX6 SD Build"), os.Getenv("OPENX6_DIGEST")}
	}
	child := command(filepath.Join(dir, "Python", "python.exe"), append([]string{"-IB", "-X", "utf8", filepath.Join(dir, script)}, args...)...)
	child.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "OPENX6_FORM=1")
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	return child.Run()
}

func build(exe, shell, wsl string, selected []string) error {
	app, err := checkFile(filepath.Join(filepath.Dir(exe), "OpenX6-app.zip"), appHash)
	if err != nil {
		return err
	}
	defer app.Close()
	found, err := command(shell, "-NoProfile", "-NonInteractive", "-Command", registeredScript).Output()
	if err != nil || (string(found) != "yes" && string(found) != "no") {
		return errors.New("cannot determine current user's OpenX6 WSL registration")
	}
	mode, err := prepare(string(found) == "yes", workerVersion, func() (string, error) {
		data, err := command(wsl, "-d", distro, "-u", "openx6", "--exec", "/bin/cat", "/opt/openx6-worker-version").Output()
		return string(data), err
	}, func() error {
		tar := filepath.Join(filepath.Dir(exe), "OpenX6-runtime.tar")
		checked, err := checkFile(tar, runtimeHash)
		if err != nil {
			return err
		}
		defer checked.Close()
		base, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		loc := filepath.Join(base, "OpenX6", "Builder-v1")
		files, err := os.ReadDir(loc)
		if (err != nil && !os.IsNotExist(err)) || len(files) > 0 {
			return errors.New("unregistered WSL data already exists; preserve the original distribution and Holder")
		}
		if err = os.MkdirAll(loc, 0700); err != nil {
			return err
		}
		cmd := command(wsl, "--import", distro, loc, tar, "--version", "2")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("WSL 2 import failed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	args := []string{"-d", distro, "-u", "openx6", "--exec", "/usr/bin/python3", "-IB", "-c", bootstrap, appHash, mode}
	cmd := command(wsl, append(args, selected...)...)
	cmd.Stdin = app
	cmd.Stderr = os.Stderr
	var footer verifiedFooter
	cmd.Stdout = io.MultiWriter(os.Stdout, &footer)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build stopped; see the message above: %w", err)
	}
	dir := filepath.Join(selected[2], "OpenX6 SD Build")
	fmt.Println("Output:", filepath.Join(dir, "Insta360X6FW.bin"))
	hash, err := footer.digest()
	if err != nil {
		return err
	}
	fmt.Println("OpenX6 verified " + hash)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if len(os.Args) > 1 {
			os.Exit(1)
		}
		shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		alert := command(shell, "-NoProfile", "-STA", "-Command", "Add-Type -AssemblyName System.Windows.Forms; [void][Windows.Forms.MessageBox]::Show($env:OPENX6_ERROR,'OpenX6')")
		alert.Env = append(os.Environ(), "OPENX6_ERROR="+err.Error())
		_ = alert.Run()
		os.Exit(1)
	}
}
