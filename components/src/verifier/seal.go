package main

import (
	"io"
	"os"
	"path/filepath"
)

// SealSingle embeds an already-issued authorization. It never reads a private
// key and cannot issue authorization. Output must be in the trusted build root.
func SealSingle(source *os.File, auth, pub []byte, p policy, output string) error {
	if filepath.Dir(output) != p.Boundary || filepath.Base(output) != "single.bin" {
		return fail("invalid build output")
	}
	if err := parents(output, p); err != nil {
		return err
	}
	m, err := authenticate(auth, pub, p)
	if err != nil {
		return err
	}
	if m.Unsigned {
		return fail("seal requires an owner-signed authorization")
	}
	if err = validate(source, m); err != nil {
		return err
	}
	at, err := embeddedLocation(source)
	if err != nil {
		return err
	}
	empty, err := readAt(source, at, embeddedSize)
	if err != nil || !zero(empty) {
		return fail("authorization slot is not empty")
	}
	f, err := os.CreateTemp(p.Boundary, ".single-seal-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if _, err = io.CopyN(f, io.NewSectionReader(source, 0, int64(m.Size)), int64(m.Size)); err != nil {
		return err
	}
	if _, err = f.WriteAt(auth, at); err != nil {
		return err
	}
	if _, _, _, err = verifyEmbeddedSigned(f, pub, p); err != nil {
		return err
	}
	if err = f.Chmod(0400); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Link(tmp, output); err != nil {
		return err
	}
	if err = os.Remove(tmp); err != nil {
		return err
	}
	d, err := os.Open(p.Boundary)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func sealDirectory(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return fail("private absolute build directory required")
	}
	p := policy{Package: filepath.Join(root, "canonical.bin"), Public: filepath.Join(root, "owner.pub"), Boundary: root, Project: "c9", Base: "1.2.36", UID: uint32(os.Geteuid())}
	pub, err := readSmall(p.Public, p, false, 32)
	if err != nil {
		return err
	}
	auth, err := readSmall(p.Package+".owner-auth", p, false, 200)
	if err != nil {
		return err
	}
	f, err := openChecked(p.Package, p, false)
	if err != nil {
		return err
	}
	defer f.Close()
	return SealSingle(f, auth, pub, p, filepath.Join(root, "single.bin"))
}
