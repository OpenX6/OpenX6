package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/crypto/ssh"
)

// SSH is the floor: a persistent host key that cannot be loaded (shared /data
// not mounted, corrupt, unsafe) must not keep owner SSH down. Serve this boot
// with a temporary in-memory key instead; the owner sees a host-key change.
// The persistent identity is never replaced or rewritten by this fallback.
func hostSigner(path string) (ssh.Signer, bool, error) {
	host, err := loadHostKey(path)
	if err == nil {
		return host, false, nil
	}
	log.Printf("OWNER_SSH_TEMPORARY_HOST_KEY: persistent host key unavailable: %v", err)
	_, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return nil, false, e
	}
	temp, e := ssh.NewSignerFromKey(priv)
	if e != nil {
		return nil, false, e
	}
	log.Printf("OWNER_SSH_TEMPORARY_HOST_KEY: this boot only, fingerprint %s", ssh.FingerprintSHA256(temp.PublicKey()))
	return temp, true, nil
}

// Deployment paths and their ancestors must be controlled by root. Refuse
// symlinks, permissive key files and corrupt existing host keys; never replace
// an existing identity merely because it cannot be loaded.
func checkOwnedFile(path string, private bool) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 ||
		(private && st.Mode().Perm()&0077 != 0) {
		return fmt.Errorf("unsafe key type or permissions: %s", path)
	}
	if s, ok := st.Sys().(*syscall.Stat_t); !ok || s.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("unexpected key owner: %s", path)
	}
	return nil
}

func loadHostKey(path string) (ssh.Signer, error) {
	dir := filepath.Dir(path)
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("host-key directory must be private: %s", dir)
	}
	if s, ok := st.Sys().(*syscall.Stat_t); !ok || s.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("unexpected host-key directory owner")
	}
	_, err = os.Lstat(path)
	if os.IsNotExist(err) {
		_, priv, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return nil, e
		}
		der, e := x509.MarshalPKCS8PrivateKey(priv)
		if e != nil {
			return nil, e
		}
		f, e := os.CreateTemp(dir, ".host-key-")
		if e != nil {
			return nil, e
		}
		defer os.Remove(f.Name())
		_, e = f.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return nil, e
		}
		// Hard-link publication is atomic and cannot overwrite a concurrent creator.
		e = os.Link(f.Name(), path)
		if e != nil && !os.IsExist(e) {
			return nil, e
		}
		d, e := os.Open(dir)
		if e != nil {
			return nil, e
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return nil, e
		}
	} else if err != nil {
		return nil, err
	}
	if err = checkOwnedFile(path, true); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(data)
}
