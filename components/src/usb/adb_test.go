package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"math/big"
	"testing"
)

func androidKey(k *rsa.PublicKey) []byte {
	b := make([]byte, 524)
	binary.LittleEndian.PutUint32(b, 64)
	two32 := new(big.Int).Lsh(big.NewInt(1), 32)
	n0 := new(big.Int).Mod(k.N, two32)
	inv := new(big.Int).ModInverse(n0, two32)
	inv.Neg(inv).Mod(inv, two32)
	binary.LittleEndian.PutUint32(b[4:], uint32(inv.Uint64()))
	little := func(v *big.Int, dest []byte) {
		x := v.Bytes()
		for i := range x {
			dest[i] = x[len(x)-1-i]
		}
	}
	little(k.N, b[8:264])
	rr := new(big.Int).Lsh(big.NewInt(1), 4096)
	rr.Mod(rr, k.N)
	little(rr, b[264:520])
	binary.LittleEndian.PutUint32(b[520:], 65537)
	return []byte(base64.StdEncoding.EncodeToString(b) + " owner@offline-test\n")
}
func TestADBKeyAndSignature(t *testing.T) {
	k, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	key := androidKey(&k.PublicKey)
	if e = checkADBKey(key); e != nil {
		t.Fatal(e)
	}
	raw, _ := base64.StdEncoding.DecodeString(string(key[:700])) // first field is 700 chars
	for _, off := range []int{0, 4, 8, 264, 520} {
		b := append([]byte(nil), raw...)
		b[off] ^= 1
		if checkADBKey([]byte(base64.StdEncoding.EncodeToString(b))) == nil {
			t.Fatalf("corrupt key field %d accepted", off)
		}
	}
	if checkADBKey(append(key, key...)) == nil {
		t.Fatal("multiple owners accepted")
	}
	// ADB AUTH uses a random 20-byte token as the SHA-1 digest input to RSA
	// PKCS#1 v1.5 signing. This tests host crypto/format, not the target daemon.
	token := make([]byte, 20)
	rand.Read(token)
	sig, e := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA1, token)
	if e != nil {
		t.Fatal(e)
	}
	if e = rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA1, token, sig); e != nil {
		t.Fatal(e)
	}
	sig[0] ^= 1
	if rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA1, token, sig) == nil {
		t.Fatal("invalid signature accepted")
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	sig, _ = rsa.SignPKCS1v15(rand.Reader, other, crypto.SHA1, token)
	if rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA1, token, sig) == nil {
		t.Fatal("wrong signer accepted")
	}
}
