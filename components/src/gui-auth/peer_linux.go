package main

import (
	"errors"
	"net"
	"syscall"
)

func rootPeer(c *net.UnixConn) error {
	raw, e := c.SyscallConn()
	if e != nil {
		return e
	}
	var check error
	e = raw.Control(func(fd uintptr) {
		p, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err != nil {
			check = err
		} else if p.Uid != 0 {
			check = errors.New("non-root bridge")
		}
	})
	if e != nil {
		return e
	}
	return check
}
