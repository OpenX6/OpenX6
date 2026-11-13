// x6-owner-maintenance is a minimal key-only SSH command endpoint for the
// owner's camera. It intentionally has no password, shell, forwarding or SFTP.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"openx6.local/components/holder"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

func main() {
	listen := flag.String("listen", ":2222", "TCP listen address")
	pubFile := flag.String("authorized-keys", holder.Dir+"/"+holder.SSH, "single owner public key file")
	hostFile := flag.String("host-key", "/data/x6-owner-ssh/host_ed25519", "device-specific SSH host key")
	showDigest := flag.Bool("holder-digest", false, "check the installed holder identity")
	probe := flag.Bool("probe-root", false, "check the local root maintenance listener using the installed public key")
	flag.Parse()
	if *probe {
		if err := probeRoot(*listen, *hostFile); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *showDigest {
		b, e := holder.Read("/", holder.SSH)
		if e != nil {
			log.Fatal(e)
		}
		fmt.Printf("%x\n", sha256.Sum256(b))
		return
	}
	if os.Geteuid() != 0 {
		log.Fatal("owner maintenance must be started as root")
	}
	if err := serve(*listen, *pubFile, *hostFile); err != nil {
		log.Fatal(err)
	}
}

func serve(listen, pubFile, hostFile string) error {
	config, err := serverConfig(pubFile, hostFile)
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	defer l.Close()
	log.Printf("owner maintenance listening on %s", l.Addr())
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go handle(c, config)
	}
}

func serverConfig(pubFile, hostFile string) (*ssh.ServerConfig, error) {
	return serverConfigAt("/", pubFile, hostFile)
}

func serverConfigAt(root, pubFile, hostFile string) (*ssh.ServerConfig, error) {
	if pubFile != holder.Dir+"/"+holder.SSH {
		if err := checkOwnedFile(pubFile, false); err != nil {
			return nil, err
		}
	}
	var err error
	var pubData []byte
	if pubFile == holder.Dir+"/"+holder.SSH {
		pubData, err = holder.Read(root, holder.SSH)
	} else {
		pubData, err = os.ReadFile(pubFile)
	}
	if err != nil {
		return nil, fmt.Errorf("owner key: %w", err)
	}
	owner, _, options, rest, err := ssh.ParseAuthorizedKey(pubData)
	if err != nil || len(options) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("owner key invalid, has unsupported options, or multiple keys supplied")
	}
	host, _, err := hostSigner(hostFile)
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}
	config := &ssh.ServerConfig{
		MaxAuthTries: 2,
		BannerCallback: func(meta ssh.ConnMetadata) string {
			addr, ok := meta.RemoteAddr().(*net.TCPAddr)
			if !ok || !addr.IP.IsLoopback() || string(meta.ClientVersion()) != probeVersion {
				return ""
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "/bin/sh", "-c", "id -u").Output()
			if err != nil {
				return ""
			}
			return "maintenance uid=" + string(bytes.TrimSpace(out))
		},
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() == "root" && bytes.Equal(key.Marshal(), owner.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unauthorized")
		},
	}
	config.AddHostKey(host)
	return config, nil
}

func handle(raw net.Conn, config *ssh.ServerConfig) {
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(15 * time.Second))
	conn, channels, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = conn.Wait(); cancel() }()
	_ = raw.SetDeadline(time.Time{})
	go ssh.DiscardRequests(requests)
	for ch := range channels {
		if ch.ChannelType() != "session" {
			_ = ch.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		channel, reqs, err := ch.Accept()
		if err != nil {
			continue
		}
		go session(ctx, channel, reqs)
	}
}

func session(ctx context.Context, channel ssh.Channel, reqs <-chan *ssh.Request) {
	defer channel.Close()
	for req := range reqs {
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil || len(payload.Command) > 4096 || bytes.IndexByte([]byte(payload.Command), 0) >= 0 {
			_ = req.Reply(false, nil)
			return
		}
		_ = req.Reply(true, nil)
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", payload.Command)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = time.Second
		cmd.Stdin = channel
		cmd.Stdout = channel
		cmd.Stderr = channel.Stderr()
		code := uint32(0)
		if err := cmd.Run(); err != nil {
			code = 255
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				if status, ok := exit.Sys().(syscall.WaitStatus); ok {
					code = uint32(status.ExitStatus())
				}
			} else {
				_, _ = fmt.Fprintln(channel.Stderr(), err)
			}
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
		return
	}
}

const probeVersion = "SSH-2.0-OpenX6LocalCheck"

// The SSH client asks the server whether it accepts this public key for root.
// It requests a signature only after SSH_MSG_USERAUTH_PK_OK. Stop at that point.
type publicKeyProbe struct {
	key      ssh.PublicKey
	accepted bool
}

func (p *publicKeyProbe) PublicKey() ssh.PublicKey { return p.key }
func (p *publicKeyProbe) Sign(io.Reader, []byte) (*ssh.Signature, error) {
	p.accepted = true
	return nil, errors.New("public-key probe complete")
}

func probeRoot(addr, hostFile string) error {
	if addr != "127.0.0.1:2222" && addr != "127.0.0.1:2223" {
		return errors.New("invalid probe endpoint")
	}
	data, err := holder.Read("/", holder.SSH)
	if err != nil {
		return err
	}
	owner, _, _, _, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return err
	}
	if err = checkOwnedFile(hostFile, true); err != nil {
		return err
	}
	raw, err := os.ReadFile(hostFile)
	if err != nil {
		return err
	}
	host, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		return err
	}
	return probeListener(addr, owner, host.PublicKey())
}

func probeListener(addr string, owner, host ssh.PublicKey) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	probe := &publicKeyProbe{key: owner}
	rootConfirmed := false
	client, _, _, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{User: "root", ClientVersion: probeVersion, BannerCallback: func(message string) error {
		rootConfirmed = message == "maintenance uid=0"
		return nil
	}, HostKeyCallback: ssh.FixedHostKey(host), Auth: []ssh.AuthMethod{ssh.PublicKeys(probe)}})
	if err == nil {
		client.Close()
		return errors.New("unexpected unsigned login")
	}
	if !probe.accepted || !rootConfirmed {
		return fmt.Errorf("root key probe: %w", err)
	}
	return nil
}
