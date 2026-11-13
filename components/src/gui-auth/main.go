package main

import (
	"crypto/ed25519"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"openx6.local/components/holder"
	"os"
	"syscall"
	"time"
)

func trustedFile(path string, private bool) ([]byte, error) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.Mode().IsRegular() || st.Size() != 32 || s.Uid != 0 || st.Mode().Perm()&0022 != 0 || (private && st.Mode().Perm()&0077 != 0) {
		return nil, errors.New("untrusted key file")
	}
	b := make([]byte, 32)
	_, e = io.ReadFull(f, b)
	return b, e
}
func main() {
	bind := flag.String("listen", "192.168.42.1:56444", "camera Wi-Fi address only")
	flag.Parse()
	if os.Geteuid() != 0 {
		log.Fatal("root required")
	}
	pub, e := holder.Read("/", holder.GUI)
	if e != nil {
		log.Fatal(e)
	}
	seed, e := trustedFile("/data/x6-owner-gui/server_ed25519", true)
	if e != nil {
		log.Fatal(e)
	}
	st, e := os.Lstat("/run/x6-owner-gui")
	if e != nil {
		log.Fatal(e)
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.IsDir() || s.Uid != 0 || st.Mode().Perm() != 0700 {
		log.Fatal("untrusted runtime directory")
	}
	syscall.Umask(0077)
	authln, e := net.Listen("unix", "/run/x6-owner-gui/auth.sock")
	if e != nil {
		log.Fatal(e)
	}
	defer authln.Close()
	a := newAuth(pub)
	slots := make(chan struct{}, 8)
	go func() {
		for {
			c, e := authln.Accept()
			if e != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				go func() { defer func() { <-slots }(); a.handle(c) }()
			default:
				c.Close()
			}
		}
	}()
	cfg, e := serverTLS(ed25519.NewKeyFromSeed(seed), pub)
	if e != nil {
		log.Fatal(e)
	}
	f := newFrontend(cfg, "/run/x6-owner-gui-bridge/dispatch.sock", rootPeer)
	// Wi-Fi may appear after GUI start; neither service depends on SSH.
	for {
		ln, e := net.Listen("tcp", *bind)
		if e != nil {
			time.Sleep(time.Second)
			continue
		}
		for {
			c, e := ln.Accept()
			if e != nil {
				break
			}
			go f.handle(c)
		}
		ln.Close()
		time.Sleep(time.Second)
	}
}
