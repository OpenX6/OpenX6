// SPDX-License-Identifier: Apache-2.0
// Compatibility listener for the public X6 Wi-Fi command client.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	device  = "wlan1"
	address = "192.168.42.1:23456"
	action  = "DeveloperExecuteCommand_Action {\"DeveloperExecuteCommand\":%s} "
)

func command(r io.Reader) (string, error) {
	line, err := bufio.NewReaderSize(r, 1000).ReadSlice('\n')
	if err != nil || len(line) >= 1000 {
		return "", errors.New("incomplete or oversized request")
	}
	payload, ok := strings.CutPrefix(string(line[:len(line)-1]), action)
	if !ok || strings.TrimSpace(payload) == "" || strings.ContainsAny(payload, "\r\x00") {
		return "", errors.New("invalid action")
	}
	return payload, nil
}

func shell(ctx context.Context, r io.Reader) (*exec.Cmd, error) {
	payload, err := command(r)
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", payload), nil
}

func serve(c *net.TCPConn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(4 * time.Second))
	ip := c.RemoteAddr().(*net.TCPAddr).IP.To4()
	if ip == nil || ip[0] != 192 || ip[1] != 168 || ip[2] != 42 || ip[3] < 2 || ip[3] == 255 {
		return
	}
	child, err := shell(context.Background(), c)
	if err != nil {
		return
	}
	// The legacy client receives stdout and the command status on its own callback.
	if child.Start() != nil {
		return
	}
	io.WriteString(c, "STATUS:0 / Pack success\n")
	c.Close()
	child.Wait()
}

func hotspot() (*net.Interface, error) {
	iface, err := net.InterfaceByName(device)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		ip, _, err := net.ParseCIDR(a.String())
		if err == nil && ip.Equal(net.IPv4(192, 168, 42, 1)) && iface.Flags&net.FlagUp != 0 {
			return iface, nil
		}
	}
	return nil, errors.New("hotspot address unavailable")
}

// This local endpoint accepts no command. It exists only for the lifetime of a
// bound hotspot listener, inside the unit's root-only runtime directory.
func rootChecks(tcp *net.TCPListener, iface *net.Interface, slots chan struct{}) (func(), error) {
	const path = "/run/x6-wifi-root/check.sock"
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := l.AcceptUnix()
			if err != nil {
				return
			}
			if len(slots) < cap(slots) {
				live, e := hotspot()
				if e == nil && live.Index == iface.Index && tcp.Addr().String() == address {
					checkCtx, stop := context.WithTimeout(ctx, time.Second)
					cmd, e := shell(checkCtx, strings.NewReader(action+"id -u\n"))
					if e == nil {
						out, e := cmd.Output()
						if e == nil && string(out) == "0\n" {
							c.SetWriteDeadline(time.Now().Add(time.Second))
							io.WriteString(c, "uid=0\n")
						}
					}
					stop()
				}
			}
			c.Close()
		}
	}()
	return func() { cancel(); l.Close(); <-done }, nil
}

func listen(iface *net.Interface, slots chan struct{}) error {
	config := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var sockErr error
		err := raw.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, device)
		})
		if err != nil {
			return err
		}
		return sockErr
	}}
	listener, err := config.Listen(context.Background(), "tcp4", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	tcp := listener.(*net.TCPListener)
	if stop, err := rootChecks(tcp, iface, slots); err == nil {
		defer stop()
	} else {
		fmt.Fprintln(os.Stderr, "local root check unavailable:", err)
	}
	for {
		tcp.SetDeadline(time.Now().Add(2 * time.Second))
		c, err := tcp.AcceptTCP()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				live, lookupErr := hotspot()
				if lookupErr == nil && live.Index == iface.Index {
					continue
				}
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			go func() { defer func() { <-slots }(); serve(c) }()
		default:
			c.Close()
		}
	}
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "requires uid 0")
		os.Exit(1)
	}
	slots := make(chan struct{}, 8)
	for {
		if iface, err := hotspot(); err == nil {
			listen(iface, slots)
		}
		time.Sleep(2 * time.Second)
	}
}
