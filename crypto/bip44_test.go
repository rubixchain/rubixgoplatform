package crypto

import (
	"crypto/rand"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/tyler-smith/go-bip32"
)

func TestHDKeyGeneration(t *testing.T) {
	GenerateHDKeysTest(t, "cup symbol flee find decline market tube border artist clever make plastic unfold chaos float artwork sustain suspect risk process fox decrease west seven", "test1", TestRbtcoinType, NonWalletAccount, 0, 0)
	GenerateHDKeysTest(t, "cup symbol flee find decline market tube border artist clever make plastic unfold chaos float artwork sustain suspect risk process fox decrease west seven", "test1", TestRbtcoinType, NonWalletAccount, 0, 1)
}

func GenerateHDKeysTest(t *testing.T, mnemonic string, pwd string, coinType, accountType, change, index uint32) {
	hdWallet, err := NewHDWallet(mnemonic)
	if err != nil {
		t.Fatal("failed to generate HD key pair", "err", err)
	}

	path := []uint32{
		bip32.FirstHardenedChild + Purpose,     // e.g., 44'
		bip32.FirstHardenedChild + coinType,    // e.g., 1001
		bip32.FirstHardenedChild + accountType, // e.g., 0'
		change,                                 // 0 (external) or 1 (change)
		index,                                  // 0, 1, 2, ...
	}
	privateKey, err := hdWallet.DerivePrivateKey(path)
	if err != nil {
		t.Fatal("failed to derive HD priv key", "err", err)
	}

	privKey, pubKey, err := DeriveKeyPair(privateKey, pwd)
	if err != nil {
		t.Fatal("failed to derive HD key pair", "err", err)
	}

	privkey, pubkey, err := DecodeBIPKeyPair(pwd, privKey, pubKey)
	if err != nil {
		t.Fatal("failed to decode key pair", "err", err)
	}

	data, err := GetRandBytes(rand.Reader, 20)
	if err != nil {
		t.Fatal("failed to generate random number", "err", err)
	}

	privkeyback := secp256k1.PrivKeyFromBytes(privkey)
	privKeySer := privkeyback.ToECDSA()
	pubkeyback, err := secp256k1.ParsePubKey(pubkey)
	if err != nil {
		t.Fatal("failed to parse pub key", "err", err)
	}
	pubKeySer := pubkeyback.ToECDSA()

	sig, err := BIPSign(privKeySer, data)
	if err != nil {
		t.Fatal("failed to do signature", "err", err)
	}

	if !BIPVerify(pubKeySer, data, sig) {
		t.Fatal("failed to do verify signature", "err", err)
	}
}
