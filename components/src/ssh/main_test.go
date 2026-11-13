package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := ssh.NewSignerFromKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func testConfig(t *testing.T) (*ssh.ServerConfig, ssh.Signer, ssh.PublicKey, string, string) {
	t.Helper()
	d := t.TempDir()
	os.Chmod(d, 0700)
	owner := testSigner(t)
	p := filepath.Join(d, "owner.pub")
	if e := os.WriteFile(p, ssh.MarshalAuthorizedKey(owner.PublicKey()), 0600); e != nil {
		t.Fatal(e)
	}
	h := filepath.Join(d, "host", "key")
	cfg, e := serverConfig(p, h)
	if e != nil {
		t.Fatal(e)
	}
	host, e := loadHostKey(h)
	if e != nil {
		t.Fatal(e)
	}
	return cfg, owner, host.PublicKey(), p, h
}
func startTestServer(t *testing.T, cfg *ssh.ServerConfig) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go handle(c, cfg)
		}
	}()
	return l.Addr().String()
}
func dial(address, user string, host ssh.PublicKey, auth ...ssh.AuthMethod) (*ssh.Client, error) {
	return ssh.Dial("tcp", address, &ssh.ClientConfig{User: user, Auth: auth, HostKeyCallback: ssh.FixedHostKey(host), Timeout: 3 * time.Second})
}
func TestAuthenticationAndExec(t *testing.T) {
	cfg, key, host, _, _ := testConfig(t)
	addr := startTestServer(t, cfg)
	for _, tc := range []struct {
		name, user string
		auth       []ssh.AuthMethod
	}{
		{"wrong-key", "root", []ssh.AuthMethod{ssh.PublicKeys(testSigner(t))}},
		{"no-auth", "root", nil}, {"password", "root", []ssh.AuthMethod{ssh.Password("irrelevant")}},
		{"wrong-user", "shell", []ssh.AuthMethod{ssh.PublicKeys(key)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, e := dial(addr, tc.user, host, tc.auth...)
			if e == nil {
				c.Close()
				t.Fatal("unauthorized authentication accepted")
			}
		})
	}
	c, e := dial(addr, "root", host, ssh.PublicKeys(key))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	t.Run("actual-host-uid", func(t *testing.T) {
		s, e := c.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		out, e := s.Output("id -u")
		if e != nil || strings.TrimSpace(string(out)) != strconv.Itoa(os.Geteuid()) {
			t.Fatalf("%s %v", out, e)
		}
		t.Logf("exec host euid=%d; not a camera/root verification", os.Geteuid())
	})
	t.Run("stdin-disposable-write-read-delete", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "probe")
		s, e := c.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		s.Stdin = strings.NewReader("x6-owner-write-probe\n")
		// Temp paths on this test host have no single quotes; explicitly guard it.
		if strings.Contains(p, "'") {
			t.Fatal("unexpected path")
		}
		out, e := s.Output(fmt.Sprintf("cat > '%s' && cat '%s' && rm '%s'", p, p, p))
		if e != nil || string(out) != "x6-owner-write-probe\n" {
			t.Fatalf("%q %v", out, e)
		}
		if _, e = os.Stat(p); !os.IsNotExist(e) {
			t.Fatal("probe not removed")
		}
	})
	t.Run("exit-status-and-stderr", func(t *testing.T) {
		s, e := c.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		out, e := s.CombinedOutput("printf diagnostic >&2; exit 13")
		var x *ssh.ExitError
		if !errors.As(e, &x) || x.ExitStatus() != 13 || string(out) != "diagnostic" {
			t.Fatalf("%q %v", out, e)
		}
	})
	t.Run("no-interactive-shell", func(t *testing.T) {
		s, e := c.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		if s.Shell() == nil {
			t.Fatal("unexpected shell request accepted")
		}
	})
	t.Run("no-port-forwarding", func(t *testing.T) {
		ch, _, e := c.OpenChannel("direct-tcpip", nil)
		if e == nil {
			ch.Close()
			t.Fatal("forward accepted")
		}
	})
	t.Run("oversize-command", func(t *testing.T) {
		s, e := c.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		if s.Run(strings.Repeat("x", 4097)) == nil {
			t.Fatal("oversize command accepted")
		}
	})
}
func TestHostIdentityPersistsAcrossRestart(t *testing.T) {
	cfg, key, host, p, h := testConfig(t)
	for i := 0; i < 2; i++ {
		addr := startTestServer(t, cfg)
		c, e := dial(addr, "root", host, ssh.PublicKeys(key))
		if e != nil {
			t.Fatal(e)
		}
		c.Close()
		cfg, e = serverConfig(p, h)
		if e != nil {
			t.Fatal(e)
		}
	}
	h2, e := loadHostKey(h)
	if e != nil || !bytes.Equal(host.Marshal(), h2.PublicKey().Marshal()) {
		t.Fatal("host identity changed")
	}
}

// Owner credentials still fail closed: without one trusted owner key there is
// nothing safe to authenticate against.
func TestInvalidOwnerCredentialsFailClosed(t *testing.T) {
	for _, kind := range []string{"owner-world-writable", "multiple-owners", "owner-options", "owner-missing"} {
		t.Run(kind, func(t *testing.T) {
			_, _, _, p, h := testConfig(t)
			switch kind {
			case "owner-world-writable":
				os.Chmod(p, 0666)
			case "multiple-owners":
				b, _ := os.ReadFile(p)
				os.WriteFile(p, append(b, b...), 0600)
			case "owner-options":
				b, _ := os.ReadFile(p)
				os.WriteFile(p, append([]byte("command=\"id\" "), b...), 0600)
			case "owner-missing":
				os.Remove(p)
			}
			if _, e := serverConfig(p, h); e == nil {
				t.Fatal("unsafe owner credentials accepted")
			}
		})
	}
}

// A host key that cannot be loaded must not keep SSH down. The server comes
// up with a temporary key, the owner still authenticates, and the persistent
// file is left exactly as found (never replaced or repaired by the fallback).
func TestUnloadableHostKeyFallsBackWithoutReplacing(t *testing.T) {
	for _, kind := range []string{"corrupt-host", "host-symlink", "host-world-readable", "host-dir-unwritable", "host-dir-is-file"} {
		t.Run(kind, func(t *testing.T) {
			_, owner, persistent, p, h := testConfig(t)
			check := h
			switch kind {
			case "corrupt-host":
				os.WriteFile(h, []byte("bad key"), 0600)
			case "host-symlink":
				os.Rename(h, h+".real")
				os.Symlink(h+".real", h)
			case "host-world-readable":
				os.Chmod(h, 0644)
			case "host-dir-unwritable":
				// Stand-in for an unmounted, read-only shared /data: the key
				// directory does not exist and cannot be created.
				parent := filepath.Join(filepath.Dir(filepath.Dir(h)), "ro")
				os.Mkdir(parent, 0500)
				t.Cleanup(func() { os.Chmod(parent, 0700) })
				h = filepath.Join(parent, "host", "key")
				check = ""
			case "host-dir-is-file":
				d := filepath.Join(filepath.Dir(filepath.Dir(h)), "notdir")
				os.WriteFile(d, []byte("x"), 0600)
				h = filepath.Join(d, "key")
				check = d
			}
			var before []byte
			if check != "" {
				before, _ = os.ReadFile(check)
			}
			signer, temporary, e := hostSigner(h)
			if e != nil || !temporary {
				t.Fatalf("expected temporary host key, got temporary=%v err=%v", temporary, e)
			}
			if bytes.Equal(signer.PublicKey().Marshal(), persistent.Marshal()) {
				t.Fatal("temporary key equals persistent identity")
			}
			cfg, e := serverConfig(p, h)
			if e != nil {
				t.Fatal("SSH refused to start with an unloadable host key:", e)
			}
			addr := startTestServer(t, cfg)
			c, e := ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "root", Auth: []ssh.AuthMethod{ssh.PublicKeys(owner)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 3 * time.Second})
			if e != nil {
				t.Fatal("owner could not authenticate on temporary host key:", e)
			}
			c.Close()
			if check != "" {
				after, _ := os.ReadFile(check)
				if !bytes.Equal(before, after) {
					t.Fatal("persistent identity was modified by the fallback")
				}
			}
			if kind == "host-dir-unwritable" {
				if _, e := os.Lstat(filepath.Dir(h)); !os.IsNotExist(e) {
					t.Fatal("fallback created a persistent key directory")
				}
			}
		})
	}
}

// A healthy persistent key is always preferred over a temporary one.
func TestLoadableHostKeyIsNotTemporary(t *testing.T) {
	_, _, persistent, _, h := testConfig(t)
	signer, temporary, e := hostSigner(h)
	if e != nil || temporary || !bytes.Equal(signer.PublicKey().Marshal(), persistent.Marshal()) {
		t.Fatalf("persistent key not used: temporary=%v err=%v", temporary, e)
	}
}

type migrationRootMeta struct{ ssh.ConnMetadata }

func (migrationRootMeta) User() string { return "root" }
func TestMigratedGeneratedSSHIdentity(t *testing.T) {
	root := os.Getenv("X6_REAL_MIGRATION_ROOT")
	if root == "" {
		t.Skip("explicit migrated fixture required")
	}
	if b, e := os.ReadFile(filepath.Join(root, "migration-test-complete")); e != nil || string(b) != "PASS\n" {
		t.Fatal("migration not complete")
	}
	host := filepath.Join(root, "data/x6-owner-ssh/host_ed25519")
	before, e := os.ReadFile(host)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"ins/etc/x6_owner_authorized_keys", "data/x6-owner-maintenance/owner.pub"} {
		file := filepath.Join(root, name)
		cfg, e := serverConfig(file, host)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := os.ReadFile(file)
		pub, _, _, _, e := ssh.ParseAuthorizedKey(b)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = cfg.PublicKeyCallback(migrationRootMeta{}, pub); e != nil {
			t.Fatal("holder rejected", e)
		}
		if _, e = cfg.PublicKeyCallback(migrationRootMeta{}, testSigner(t).PublicKey()); e == nil {
			t.Fatal("wrong key accepted")
		}
	}
	after, e := os.ReadFile(host)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("host identity rotated")
	}
}

func TestLocalRootProbe(t *testing.T) {
	cfg, owner, host, _, _ := testConfig(t)
	addr := startTestServer(t, cfg)
	if err := probeListener(addr, owner.PublicKey(), host); (err == nil) != (os.Geteuid() == 0) {
		t.Fatalf("uid %d: %v", os.Geteuid(), err)
	}
	cfg2, owner2, host2, _, _ := testConfig(t)
	cfg2.BannerCallback = func(ssh.ConnMetadata) string { return "maintenance uid=0" }
	addr = startTestServer(t, cfg2)
	owner, host = owner2, host2
	if err := probeListener(addr, owner.PublicKey(), host); err != nil {
		t.Fatal(err)
	}
	wrong := testSigner(t).PublicKey()
	if probeListener(addr, wrong, host) == nil {
		t.Fatal("accepted wrong owner")
	}
	if probeListener(addr, owner.PublicKey(), wrong) == nil {
		t.Fatal("accepted wrong host")
	}
}
