package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestDualAuthorityEmbedded(t *testing.T) {
	s := makeSetup(t, "1.2.36", false)
	project, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, stranger, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := projectUpdatePublic
	projectUpdatePublic = project
	defer func() { projectUpdatePublic = previous }()
	owner := s.key.Public().(ed25519.PublicKey)
	for _, tc := range []struct {
		name string
		key  ed25519.PrivateKey
		pass bool
	}{
		{"owner", s.key, true}, {"project", signing, true}, {"unknown", stranger, false}, {"unsigned card", nil, true},
	} {
		b := append([]byte(nil), s.b...)
		if tc.key != nil {
			a := authBytes(b, "1.2.36", false, tc.key)
			at := int(u32(b, 4356)) + int(embeddedOffset)
			copy(b[at:], a)
		}
		f := inputFile(t, s, b)
		m, _, _, err := verifyEmbedded(f, owner, s.p)
		if err == nil && m.Unsigned != (tc.key == nil) {
			t.Fatalf("%s: wrong attribution", tc.name)
		}
		f.Close()
		if (err == nil) != tc.pass {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	if m, err := authenticate(nullSigned(s, s.b, "1.2.36", false), owner, s.p); err != nil || !m.Unsigned {
		t.Fatal("unsigned staging receipt lost its attribution", err)
	}
	b := append([]byte(nil), s.b...)
	b[5300] ^= 1
	if _, _, _, err := verifyEmbedded(inputFile(t, s, b), owner, s.p); err == nil {
		t.Fatal("accepted corrupted unsigned payload")
	}
}
