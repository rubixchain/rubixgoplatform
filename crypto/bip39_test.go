package crypto

import (
	"crypto/rand"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func BIPtest(t *testing.T, alg CryptoAlgType, pwd string) {

	masterKey, err := BIPGenerateMasterKey(&CryptoConfig{Alg: alg, Pwd: pwd})
	if err != nil {
		t.Fatal("failed to generate key pair", "err", err)
	}

	masterKeyDecoded, err := BIPDecodeMasterKey(pwd, masterKey)
	if err != nil {
		t.Fatal("failed to decode key pair", "err", err)
	}

	priv, _, err := BIPGenerateChild(string(masterKeyDecoded), 0)
	if err != nil {
		t.Fatal("failed to generate child", "err", err)
	}

	data, err := GetRandBytes(rand.Reader, 20)
	if err != nil {
		t.Fatal("failed to generate random number", "err", err)
	}

	privKey := secp256k1.PrivKeyFromBytes(priv)
	privKeySer := privKey.ToECDSA()
	pubKeySer := privKey.PubKey().ToECDSA()
	sig, err := BIPSign(privKeySer, data)
	if err != nil {
		t.Fatal("failed to do signature", "err", err)
	}

	if !BIPVerify(pubKeySer, data, sig) {
		t.Fatal("failed to do verify signature", "err", err)
	}
}
func TestBIPKeyGeneration(t *testing.T) {
	BIPtest(t, BIP39, "test")
}
