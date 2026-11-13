package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const phoneSocket = "/run/openx6-phone-ota/upload.sock"
const phoneLimit = 8 << 30

type phoneFile struct {
	f      *os.File
	size   int64
	digest [32]byte
}
type phoneEndpoint struct {
	mu        sync.Mutex
	cooldown  time.Time
	connected func(string) bool
	spool     func(int64) (*os.File, error)
	question  func(byte, func() bool) bool
	ready     func() bool
	install   func(phoneFile) error
}

// The public proxy supplies the address after performing the stock auth_check.
// The control connection must belong to this client as well.
type phoneConnKey struct{}

func phoneRequestContext(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, phoneConnKey{}, conn)
}

// net/http may not detect a peer closing until we start consuming the body.
// Poll the nginx socket without reading any firmware before local consent.
func phoneRequestLive(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	conn, ok := ctx.Value(phoneConnKey{}).(syscall.Conn)
	if !ok {
		return ctx.Value(phoneConnKey{}) == nil
	}
	raw, e := conn.SyscallConn()
	if e != nil {
		return false
	}
	alive := false
	e = raw.Control(func(fd uintptr) {
		mask := int16(unix.POLLHUP | unix.POLLERR | unix.POLLNVAL)
		if runtime.GOOS == "linux" {
			mask |= 0x2000
		} // POLLRDHUP
		fds := []unix.PollFd{{Fd: int32(fd), Events: mask}}
		for {
			_, err := unix.Poll(fds, 0)
			if err == unix.EINTR {
				continue
			}
			alive = err == nil && fds[0].Revents&mask == 0
			break
		}
	})
	return e == nil && alive
}

func phoneConnected(addr string) bool {
	ip := net.ParseIP(addr).To4()
	if ip == nil {
		return false
	}
	peer := fmt.Sprintf("%02X%02X%02X%02X", ip[3], ip[2], ip[1], ip[0])
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 4 || f[3] != "01" || !strings.HasSuffix(f[1], ":1A0A") {
				continue
			}
			a, _, _ := strings.Cut(f[2], ":")
			if a == peer || a == "0000000000000000FFFF0000"+peer {
				return true
			}
		}
	}
	return false
}

func phoneSpool(size int64) (*os.File, error) {
	mounts, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return nil, e
	}
	for _, dir := range []string{"/ins/media_rw/emulated", "/mnt/sdcard"} {
		mounted := false
		for _, line := range strings.Split(string(mounts), "\n") {
			f := strings.Fields(line)
			if len(f) > 8 && f[3] == "/" && f[4] == dir && member("rw", strings.Split(f[5], ",")) {
				mounted = true
			}
		}
		free, e := freeBytes(dir)
		if !mounted || e != nil || free < uint64(size)+64<<20 {
			continue
		}
		f, e := os.CreateTemp(dir, ".openx6-phone-")
		if e != nil {
			continue
		}
		// No user-visible or boot-scannable file survives process or power loss.
		if e = os.Remove(f.Name()); e != nil {
			f.Close()
			return nil, e
		}
		return f, nil
	}
	return nil, fail("insufficient recording storage for firmware upload")
}

func receiveFactory(r *http.Request, boundary string, f *os.File) (phoneFile, error) {
	var pkg phoneFile
	body := &io.LimitedReader{R: r.Body, N: r.ContentLength}
	reader := multipart.NewReader(body, boundary)
	part, e := reader.NextRawPart()
	if e != nil {
		return pkg, e
	}
	if part.FileName() == "" || part.Header.Get("Content-Transfer-Encoding") != "" {
		return pkg, fail("expected a firmware file")
	}
	in := bufio.NewReader(part)
	head, e := in.Peek(104)
	if e != nil || !factoryHeader(head) {
		return pkg, fail("expected encrypted c9 factory firmware")
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), in)
	if e != nil {
		return pkg, e
	}
	if _, e = reader.NextRawPart(); e != io.EOF {
		return pkg, fail("incomplete or multiple multipart files")
	}
	if _, e = io.Copy(io.Discard, body); e != nil || body.N != 0 {
		return pkg, fail("truncated HTTP request")
	}
	// The last two stock records are the signing footer. The native updater
	// authenticates the contents; this check detects incomplete transfers.
	if n < 4480 {
		return pkg, fail("short factory container")
	}
	for _, v := range []struct {
		offset int64
		magic  string
	}{{n - 4416, "CFSI"}, {n - 256, "CWFI"}} {
		b, e := readAt(f, v.offset, 4)
		if e != nil || string(b) != v.magic {
			return pkg, fail("incomplete factory signing footer")
		}
	}
	pkg.f = f
	pkg.size = n
	copy(pkg.digest[:], h.Sum(nil))
	return pkg, nil
}

func (g *phoneEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "close")
	// Never drain a denied upload while trying to send the error to the phone.
	_ = http.NewResponseController(w).EnableFullDuplex()
	if r.Method != "POST" || r.URL.Path != "/receive" {
		http.NotFound(w, r)
		return
	}
	addr := r.Header.Get("X-Real-IP")
	if !g.connected(addr) {
		http.Error(w, "Connect the app to the camera first.", 403)
		return
	}
	typ, par, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "multipart/form-data" || par["boundary"] == "" || len(par["boundary"]) > 70 || r.ContentLength < 4480 || r.ContentLength > phoneLimit {
		http.Error(w, "Invalid firmware upload.", 400)
		return
	}
	if !g.mu.TryLock() {
		http.Error(w, "Another firmware request is in progress.", 409)
		return
	}
	defer g.mu.Unlock()
	if time.Now().Before(g.cooldown) || !g.ready() {
		http.Error(w, "Camera unavailable for firmware update.", 403)
		return
	}
	deadline := time.Now().Add(100 * time.Second)
	live := func() bool { return time.Now().Before(deadline) && phoneRequestLive(r.Context()) && g.connected(addr) }
	for _, phase := range []byte{1, 2} {
		if !live() || !g.question(phase, live) || !live() {
			g.cooldown = time.Now().Add(30 * time.Second)
			http.Error(w, "Firmware installation was not confirmed on the camera.", 403)
			return
		}
	}
	f, e := g.spool(r.ContentLength)
	if e != nil {
		http.Error(w, "Insufficient space for firmware upload.", 507)
		return
	}
	defer f.Close()
	r.Body = http.MaxBytesReader(w, r.Body, r.ContentLength)
	pkg, e := receiveFactory(r, par["boundary"], f)
	if e != nil || !phoneRequestLive(r.Context()) {
		http.Error(w, "Incomplete or invalid factory firmware.", 400)
		return
	}
	if e = g.install(pkg); e != nil {
		fmt.Println("Mobile firmware handoff:", e)
		http.Error(w, "Camera could not hand off the firmware.", 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"status":"success"}`)
}

type phoneJob struct {
	pkg    phoneFile
	result chan error
}

func startPhone(jobs chan<- phoneJob, ready func() bool) (func(), error) {
	dir := filepath.Dir(phoneSocket)
	if e := os.Mkdir(dir, 0711); e != nil && !os.IsExist(e) {
		return nil, e
	}
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() || st.Sys().(*syscall.Stat_t).Uid != 0 || st.Mode().Perm()&0022 != 0 {
		return nil, fail("unsafe phone socket directory")
	}
	// The service uses UMask=0077. nginx must be able to traverse the directory.
	if e = os.Chmod(dir, 0711); e != nil {
		return nil, e
	}
	if st, e := os.Lstat(phoneSocket); e == nil {
		if st.Mode()&os.ModeSocket == 0 || st.Sys().(*syscall.Stat_t).Uid != 0 {
			return nil, fail("occupied phone socket")
		}
		if e = os.Remove(phoneSocket); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	l, e := net.Listen("unix", phoneSocket)
	if e != nil {
		return nil, e
	}
	if e = os.Chown(phoneSocket, 0, 996); e == nil {
		e = os.Chmod(phoneSocket, 0660)
	}
	if e != nil {
		l.Close()
		return nil, e
	}
	gate := &phoneEndpoint{connected: phoneConnected, question: stockQuestion, spool: phoneSpool, ready: ready}
	gate.install = func(pkg phoneFile) error {
		j := phoneJob{pkg, make(chan error, 1)}
		select {
		case jobs <- j:
			return <-j.result
		case <-time.After(15 * time.Second):
			return fail("coordinator busy")
		}
	}
	server := &http.Server{Handler: gate, ConnContext: phoneRequestContext, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Minute, MaxHeaderBytes: 16384}
	go func() {
		if e := server.Serve(l); e != nil && e != http.ErrServerClosed {
			fmt.Println("Mobile firmware endpoint:", e)
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}, nil
}
