package main

import (
	"errors"
	"net"
)

// Host test helpers inject their own explicit peer model. No production fallback.
func rootPeer(*net.UnixConn) error { return errors.New("Linux root peer required") }
