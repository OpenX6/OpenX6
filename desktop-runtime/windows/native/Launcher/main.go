package main

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"openx6.local/desktop/update"
)

//go:embed runtime.sha256
var manifest string

func verify(root string) error {
	lines := bufio.NewScanner(strings.NewReader(manifest))
	for lines.Scan() {
		hash, name, ok := strings.Cut(lines.Text(), "  ")
		if !ok {
			return errors.New("invalid runtime manifest")
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, file)
		file.Close()
		if err != nil || hex.EncodeToString(h.Sum(nil)) != hash {
			return fmt.Errorf("component checksum mismatch: %s", name)
		}
	}
	return lines.Err()
}

// Let Windows apply its normal downloaded-app assessment.
func launchDesktop(target string) error {
	path, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		0, 0, uintptr(unsafe.Pointer(path)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("Windows could not open the updated desktop (shell code %d)", result)
	}
	return nil
}

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Dir(exe)
	action := ""
	if len(os.Args) > 1 {
		action = os.Args[1]
	}
	switch action {
	case "--desktop-check":
		version, err := update.Check()
		if err == nil {
			fmt.Println(version)
		}
		return err
	case "--desktop-ready":
		// The verified WinForms process sends this when its window opens.
		return update.Ready(exe)
	case "--ui-update":
		target, err := update.Install(os.Getenv("OPENX6_UPDATE"))
		if err != nil {
			return err
		}
		if err = launchDesktop(target); err != nil {
			return err
		}
		// Keep the existing GUI available until the installed GUI is ready.
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			if strings.EqualFold(update.Active(), target) {
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		if os.Getenv("OPENX6_UI_LANG") == "zh" {
			return errors.New("新窗口尚未确认启动，可在文件菜单重试")
		}
		return errors.New("Updated window did not confirm startup; reopen File to retry")
	}
	if action == "" {
		if active := update.Active(); active != "" && launchDesktop(active) == nil {
			return nil
		}
	}
	if err = verify(root); err != nil {
		return err
	}
	if action == "" {
		child := exec.Command(filepath.Join(root, "OpenX6.Desktop.exe"))
		child.Dir = root
		child.Env = append(os.Environ(), "OPENX6_REPOSITORY="+update.Repository)
		return child.Run()
	}
	fw := os.Getenv("OPENX6_FW")
	args := []string{"-IBu", "-X", "utf8"}
	switch action {
	case "--ui-build":
		args = append(args, filepath.Join(root, "Native", "build_native.py"))
	case "--ui-inspect", "--ui-export":
		args = append(args, filepath.Join(root, "Reader", "export_modules.py"), fw)
		if action == "--ui-inspect" {
			args = append(args, "--inspect")
		}
	case "--ui-deploy":
		args = append(args, filepath.Join(root, "Reader", "deploy_sd.py"), filepath.Join(os.Getenv("OPENX6_OUTPUT"), "OpenX6 SD Build"), os.Getenv("OPENX6_DIGEST"))
	default:
		return errors.New("unknown desktop action")
	}
	child := exec.Command(filepath.Join(root, "Reader", "Python", "python.exe"), args...)
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	child.Dir = root
	child.Env = append(os.Environ(), "OPENX6_FORM=1")
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	return child.Run()
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if len(os.Args) == 1 {
			msg, _ := syscall.UTF16PtrFromString(err.Error())
			title, _ := syscall.UTF16PtrFromString("OpenX6")
			syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(title)), 0x10)
		}
		os.Exit(1)
	}
}
