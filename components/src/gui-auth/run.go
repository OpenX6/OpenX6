package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type limitedOutput struct {
	mu        sync.Mutex
	b         []byte
	truncated bool
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	remaining := 384 - len(w.b)
	if len(p) > remaining {
		p = p[:remaining]
		w.truncated = true
	}
	w.b = append(w.b, p...)
	return n, nil
}
func runCommand(command []byte) result {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c := exec.CommandContext(ctx, "/bin/sh", "-c", string(command))
	c.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "LANG=C"}
	c.Dir = "/"
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		e := syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		if e == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return e
	}
	c.WaitDelay = 100 * time.Millisecond
	out := &limitedOutput{}
	c.Stdout = out
	c.Stderr = out
	e := c.Run()
	code := 0
	if e != nil {
		code = -1
		var ee *exec.ExitError
		if errors.As(e, &ee) {
			code = ee.ExitCode()
		}
	}
	// No post-Wait PID-based killing: the numeric PID could have been reused.
	// This bounds a foreground command, not intentionally detached descendants.
	return result{code, out.b, ctx.Err() != nil, out.truncated}
}
