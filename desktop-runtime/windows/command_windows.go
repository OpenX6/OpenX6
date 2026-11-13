package main

import (
	"os/exec"
	"syscall"
)

func command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	// Suppress the console only. STARTF_USESHOWWINDOW/SW_HIDE also hides the first WinForms window.
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	return cmd
}
