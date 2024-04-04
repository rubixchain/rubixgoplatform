package crypto

import (
	"crypto/rand"
	"testing"
)

func testKeyGeneration(t *testing.T, alg CryptoAlgType, pwd string) {
	var privKey PrivateKey
	var pubKey PublicKey
	priv, pub, err := GenerateKeyPair(&CryptoConfig{Alg: alg, Pwd: pwd})
	if err != nil {
		t.Fatal("failed to generate key pair", "err", err)
	}
	privKey, pubKey, err = DecodeKeyPair(pwd, priv, pub)
	if err != nil {
		t.Fatal("failed to decode key pair", "err", err)
	}
	data, err := GetRandBytes(rand.Reader, 20)
	if err != nil {
		t.Fatal("failed to generate random number", "err", err)
	}
	sig, err := Sign(privKey, data)
	if err != nil {
		t.Fatal("failed to do signature", "err", err)
	}

	if !Verify(pubKey, data, sig) {
		t.Fatal("failed to do verify signature", "err", err)
	}
}

func TestKeyGeneration(t *testing.T) {
	testKeyGeneration(t, ECDSAP256, "")
	testKeyGeneration(t, ECDSAP256, "TestPassword")
}
