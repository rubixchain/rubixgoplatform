package crypto

import (
	"crypto/rand"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func BIPtest(t *testing.T, mnemonic string, pwd string) {

	masterKey, err := BIPGenerateMasterKeyFromMnemonic(mnemonic, pwd)
	if err != nil {
		t.Fatal("failed to generate key pair", "err", err)
	}

	priv, pub, err := BIPGenerateChild(string(masterKey), 0, pwd)
	if err != nil {
		t.Fatal("failed to generate child", "err", err)
	}

	privkey, pubkey, err := DecodeBIPKeyPair(pwd, priv, pub)
	if err != nil {
		t.Fatal("failed to decode key pair", "err", err)
	}

	data, err := GetRandBytes(rand.Reader, 20)
	if err != nil {
		t.Fatal("failed to generate random number", "err", err)
	}

	privkeyback := secp256k1.PrivKeyFromBytes(privkey)
	privKeySer := privkeyback.ToECDSA()
	pubkeyback, _ := secp256k1.ParsePubKey(pubkey)
	pubKeySer := pubkeyback.ToECDSA()

	sig, err := BIPSign(privKeySer, data)
	if err != nil {
		t.Fatal("failed to do signature", "err", err)
	}

	if !BIPVerify(pubKeySer, data, sig) {
		t.Fatal("failed to do verify signature", "err", err)
	}
}
func TestBIPKeyGeneration(t *testing.T) {
	BIPtest(t, "cup symbol flee find decline market tube border artist clever make plastic unfold chaos float artwork sustain suspect risk process fox decrease west seven", "test")
	BIPtest(t, "cup symbol flee find decline market tube border artist clever make plastic unfold chaos float artwork sustain suspect risk process fox decrease west seven", "test")
}
