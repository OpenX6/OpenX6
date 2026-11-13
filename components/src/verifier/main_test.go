package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func put32(b []byte, o int, v uint32)     { binary.LittleEndian.PutUint32(b[o:o+4], v) }
func setstr(b []byte, o, n int, s string) { clear(b[o : o+n]); copy(b[o:o+n], s) }
func fixture(version string) []byte {
	payload := []byte("OFFLINE INERT PAYLOAD - NOT A ZIP OR FIRMWARE IMAGE\n")
	off, n := 4864, 576+len(payload)
	cert := off + n + 64
	chk := cert + 4160
	b := make([]byte, chk+320)
	copy(b, "MWFI")
	put32(b, 4, 1)
	setstr(b, 8, 16, "c9")
	setstr(b, 24, 64, version)
	put32(b, 92, 3)
	put32(b, 100, 35)
	for i, e := range [][3]int{{1, off, n}, {4096, cert, 4096}, {4097, chk, 256}} {
		for j, x := range e {
			put32(b, 4096+i*256+j*4, uint32(x))
		}
	}
	put32(b, off+64, 1)
	setstr(b, off+68, 64, "qcs8625_ota.zip")
	put32(b, off+132, 4)
	setstr(b, off+136, 16, "full")
	setstr(b, off+152, 32, "host_8625")
	setstr(b, off+184, 64, version)
	copy(b[off+576:], payload)
	h := sha256.Sum256(payload)
	copy(b[off+264:], hex.EncodeToString(h[:]))
	copy(b[cert+64:], "CFSI")
	put32(b, cert+68, 1)
	copy(b[chk+64:], "CWFI")
	put32(b, chk+68, 2)
	repairPrefix(b)
	return b
}
func repairPrefix(b []byte) {
	cert, chk := int(u32(b, 4356)), int(u32(b, 4612))
	h := sha256.Sum256(b[:cert])
	for _, o := range []int{chk + 72, chk + 136} {
		copy(b[o:], hex.EncodeToString(h[:]))
	}
}
func authBytes(b []byte, version string, down bool, key ed25519.PrivateKey) []byte {
	m := make([]byte, manifestSize)
	copy(m, "X6UPD01\x00")
	put32(m, 8, 1)
	if down {
		put32(m, 12, 1)
	}
	binary.LittleEndian.PutUint64(m[16:24], uint64(len(b)))
	h := sha256.Sum256(b)
	copy(m[24:56], h[:])
	setstr(m, 56, 16, "c9")
	setstr(m, 72, 32, version)
	setstr(m, 104, 32, "1.2.36")
	return append(m, ed25519.Sign(key, append([]byte(domain), m...))...)
}

type setup struct {
	p       policy
	b, auth []byte
	key     ed25519.PrivateKey
}

func makeSetup(t *testing.T, version string, down bool) *setup {
	t.Helper()
	root := t.TempDir()
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	p := policy{filepath.Join(root, "private/package.bin"), filepath.Join(root, "owner.pub"), root, "c9", "1.2.36", uint32(os.Geteuid())}
	if e = os.Mkdir(filepath.Dir(p.Package), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p.Public, pub, 0644); e != nil {
		t.Fatal(e)
	}
	s := &setup{p: p, b: fixture(version), key: key}
	s.auth = authBytes(s.b, version, down, key)
	s.write(t)
	return s
}
func (s *setup) write(t *testing.T) {
	t.Helper()
	for path, b := range map[string][]byte{s.p.Package: s.b, s.p.Package + ".owner-auth": s.auth} {
		_ = os.Chmod(path, 0600)
		if e := os.WriteFile(path, b, 0400); e != nil {
			t.Fatal(e)
		}
		if e := os.Chmod(path, 0400); e != nil {
			t.Fatal(e)
		}
	}
}
func (s *setup) resign(version string, down bool) { s.auth = authBytes(s.b, version, down, s.key) }
func (s *setup) signEnvelope() {
	s.auth = append(s.auth[:manifestSize], ed25519.Sign(s.key, append([]byte(domain), s.auth[:manifestSize]...))...)
}

func TestOwnerPolicy(t *testing.T) {
	for _, c := range []struct {
		name, version string
		down          bool
		code          int
		ok            bool
	}{
		{"same", "1.2.36", false, 0, true}, {"higher", "1.2.37", false, 0, true},
		{"lower-no-permission", "1.1.7", false, 64, false}, {"lower-authorized", "1.1.7", true, 10, true},
		{"old-root-authorized", "0.8.825", true, 10, true}, {"four-parts", "1.2.36.0", false, 0, true},
		{"invalid-version", "../1.2.36", true, 64, false}, {"leading-zero", "01.2.36", true, 64, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := makeSetup(t, c.version, c.down)
			code, e := verify(s.p, s.p.Package, c.version, true)
			if (e == nil) != c.ok || code != c.code {
				t.Fatalf("%d %v", code, e)
			}
			if c.ok {
				code, e = verify(s.p, s.p.Package, "", false)
				if e != nil || code != 0 {
					t.Fatal(code, e)
				}
			}
		})
	}
}
func TestNegativeCases(t *testing.T) {
	cases := map[string]func(*setup){
		"wrong-owner":                func(s *setup) { pub, _, _ := ed25519.GenerateKey(rand.Reader); os.WriteFile(s.p.Public, pub, 0644) },
		"signature-tamper":           func(s *setup) { s.auth[manifestSize] ^= 1 },
		"permission-bit-tamper":      func(s *setup) { s.auth[12] ^= 1 },
		"unknown-signed-flags":       func(s *setup) { put32(s.auth, 12, 2); s.signEnvelope() },
		"wrong-signed-model":         func(s *setup) { setstr(s.auth, 56, 16, "other"); s.signEnvelope() },
		"wrong-signed-base":          func(s *setup) { setstr(s.auth, 104, 32, "0.8.825"); s.signEnvelope() },
		"wrong-signed-version":       func(s *setup) { setstr(s.auth, 72, 32, "1.2.37"); s.signEnvelope() },
		"wrong-signed-size":          func(s *setup) { s.auth[16] ^= 1; s.signEnvelope() },
		"wrong-signed-hash":          func(s *setup) { s.auth[24] ^= 1; s.signEnvelope() },
		"missing-signature":          func(s *setup) { s.auth = s.auth[:manifestSize] },
		"trailing-signature":         func(s *setup) { s.auth = append(s.auth, 0) },
		"model-tamper-repair-digest": func(s *setup) { setstr(s.b, 8, 16, "c8"); repairPrefix(s.b); s.resign("1.2.36", false) },
		"unsupported-format":         func(s *setup) { put32(s.b, 4, 2); repairPrefix(s.b); s.resign("1.2.36", false) },
		"encrypted-owner-file":       func(s *setup) { put32(s.b, 96, 1); repairPrefix(s.b); s.resign("1.2.36", false) },
		"unknown-encryption":         func(s *setup) { put32(s.b, 96, 2); repairPrefix(s.b); s.resign("1.2.36", false) },
		"extra-module":               func(s *setup) { put32(s.b, 92, 4); repairPrefix(s.b); s.resign("1.2.36", false) },
		"offset-overflow":            func(s *setup) { put32(s.b, 4100, 0xfffffff0); s.resign("1.2.36", false) },
		"overlap":                    func(s *setup) { put32(s.b, 4356, 4864); s.resign("1.2.36", false) },
		"module-index":               func(s *setup) { put32(s.b, 4864+64, 2); repairPrefix(s.b); s.resign("1.2.36", false) },
		"MCU-DevId":                  func(s *setup) { put32(s.b, 4864+132, 1); repairPrefix(s.b); s.resign("1.2.36", false) },
		"module-name-traversal": func(s *setup) {
			setstr(s.b, 4864+68, 64, "../../etc/shadow")
			repairPrefix(s.b)
			s.resign("1.2.36", false)
		},
		"module-hash":                      func(s *setup) { s.b[4864+264] ^= 1; repairPrefix(s.b); s.resign("1.2.36", false) },
		"module-payload":                   func(s *setup) { s.b[4864+576] ^= 1; repairPrefix(s.b); s.resign("1.2.36", false) },
		"FwChk":                            func(s *setup) { s.b[int(u32(s.b, 4612))+72] ^= 1; s.resign("1.2.36", false) },
		"CFSI":                             func(s *setup) { s.b[int(u32(s.b, 4356))+64] ^= 1; s.resign("1.2.36", false) },
		"unsupported-cert-method":          func(s *setup) { put32(s.b, int(u32(s.b, 4356))+72, 99); s.resign("1.2.36", false) },
		"CWFI":                             func(s *setup) { s.b[int(u32(s.b, 4612))+64] ^= 1; s.resign("1.2.36", false) },
		"owner-hash-catches-excluded-tail": func(s *setup) { s.b[len(s.b)-1] ^= 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			mutate(s)
			s.write(t)
			code, e := verify(s.p, s.p.Package, "", false)
			if e == nil || code != 64 {
				t.Fatalf("accepted: %d %v", code, e)
			}
		})
	}
}
func TestPathPermissionsAndRecheck(t *testing.T) {
	for _, name := range []string{"wrong-path", "dot-path", "public-staging", "package-writable", "key-writable", "auth-readable", "symlink", "hardlink", "wrong-uid", "native-version-mismatch", "auth-missing-after-entry"} {
		t.Run(name, func(t *testing.T) {
			s := makeSetup(t, "1.2.36", false)
			path, ver, stage := s.p.Package, "", false
			switch name {
			case "wrong-path":
				path = "/mnt/sdcard/Insta360X6FW.bin"
			case "dot-path":
				path = filepath.Dir(path) + "/../private/package.bin"
			case "public-staging":
				os.Chmod(filepath.Dir(path), 0755)
			case "package-writable":
				os.Chmod(path, 0600)
			case "key-writable":
				os.Chmod(s.p.Public, 0666)
			case "auth-readable":
				os.Chmod(path+".owner-auth", 0644)
			case "symlink":
				os.Rename(path, path+".real")
				os.Symlink(path+".real", path)
			case "hardlink":
				os.Link(path, path+".alias")
			case "wrong-uid":
				s.p.UID++
			case "native-version-mismatch":
				ver = "0.8.825"
				stage = true
			case "auth-missing-after-entry":
				if _, e := verify(s.p, path, "", false); e != nil {
					t.Fatal(e)
				}
				os.Remove(path + ".owner-auth")
				stage = true
				ver = "1.2.36"
			}
			if code, e := verify(s.p, path, ver, stage); e == nil || code != 64 {
				t.Fatal("accepted", code, e)
			}
		})
	}
}

// Host executable bridge used by ARM64 syscall emulator; excluded from production.
func TestDriver(t *testing.T) {
	root := os.Getenv("X6_TEST_ROOT")
	if root == "" {
		t.Skip("emulator driver only")
	}
	p := policy{filepath.Join(root, "private/package.bin"), filepath.Join(root, "owner.pub"), root, "c9", "1.2.36", uint32(os.Geteuid())}
	if os.Getenv("X6_TEST_GUEST_PATH") != packagePath {
		os.Exit(64)
	}
	code, e := verify(p, p.Package, os.Getenv("X6_TEST_VERSION"), os.Getenv("X6_TEST_STAGE") == "version")
	if e != nil {
		os.Exit(64)
	}
	os.Exit(code)
}

// Structural/digest compatibility only; no signing and no staging or mutation.
func TestLockedCandidateStructure(t *testing.T) {
	path := os.Getenv("X6_TEST_LOCKED_CANDIDATE")
	if path == "" {
		t.Skip("explicit read-only candidate check only")
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		t.Fatal(e)
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	if hex.EncodeToString(digest[:]) != "4172bf5707f5f22b6cd06796a630a9217de3f97e0cfd2829f02f2ba02300c001" {
		t.Fatal("not the hash-pinned LOCKED v2 candidate")
	}
	m := manifest{Size: uint64(st.Size()), Digest: digest, Project: "c9", Version: "1.2.36", Base: "1.2.36"}
	if e = validate(f, m); e != nil {
		t.Fatal(e)
	}
	t.Log("Locked candidate layout, module digest, FwChk and whole-file digest pass; no authorization produced")
}

func TestProductionBinPath(t *testing.T) {
	p := productionPolicy()
	if p.Package != "/data/x6-owner-update/package.bin" {
		t.Fatal(p.Package)
	}
	for _, path := range []string{"/data/x6-owner-update/package.mwfi", "/mnt/sdcard/Insta360X6FW.bin", "/data/x6-owner-update/package.bin.evil"} {
		code, err := verify(p, path, "", false)
		if code != 64 || err == nil {
			t.Fatal(path, code, err)
		}
	}
}
