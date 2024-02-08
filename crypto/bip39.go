package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/pem"
	"fmt"

	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const (
	BIP39 = iota
)

// Generate child private and public keys for BIP39 masterkey and given path
// The child private key is regenerated on demand from master key hence never stored
// The child public key need to shared with other peers for verification
// Make sure the path of child is also stored along with public key
func BIPGenerateChild(masterKey string, childPath int) ([]byte, []byte, error) {
	var privateKeyBytes []byte
	var publicKeyBytes []byte
	masterKeybip32, err := bip32.B58Deserialize(masterKey)
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := masterKeybip32.NewChildKey(uint32(childPath))
	if err != nil {
		return nil, nil, err
	}
	publicKey := privateKey.PublicKey()
	privateKeyBytes = privateKey.Key
	publicKeyBytes = publicKey.Key
	return privateKeyBytes, publicKeyBytes, nil
}

// Generate BIPMasterKey from Mnemonic and user provided password
// Useful in key recovery / device migration through mnemonics
func BIPGenerateMasterKeyFromMnemonic(mnemonic string, pwd string) ([]byte, error) {
	var masterkeySeralise string
	seed := bip39.NewSeed(mnemonic, pwd)
	masterKey, _ := bip32.NewMasterKey(seed)
	masterkeySeralise = masterKey.B58Serialize()
	var pemEncPriv []byte
	if pwd != "" {
		encBlock, err := Seal(pwd, []byte(masterkeySeralise))
		if err != nil {
			return nil, err
		}
		_, err = UnSeal(pwd, encBlock)
		if err != nil {
			return nil, err
		}
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: encBlock})
	} else {
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte(masterkeySeralise)})
	}
	return pemEncPriv, nil
}

// Generate a random BIP mnemonic in rubix
func BIPGenerateMnemonic() string {
	entropy, _ := bip39.NewEntropy(256)
	mnemonic, _ := bip39.NewMnemonic(entropy)
	return mnemonic
}

// Generate a Bip32 HD wallet MasteKey for the mnemonic and a user provided randomness
// here we are reusing the password used for sealing masterkey as source of randomness also
func BIPGenerateMasterKey(cfg *CryptoConfig) ([]byte, error) {
	var pemEncPriv []byte
	var err error
	if cfg.Alg == 0 {
		mnemonic := BIPGenerateMnemonic()
		pemEncPriv, err = BIPGenerateMasterKeyFromMnemonic(mnemonic, cfg.Pwd)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unsupported algorithm")
	}
	return pemEncPriv, nil
}

// Decode the master public key
func BIPDecodeMasterKey(pwd string, privKey []byte) ([]byte, error) {
	var cryptoPrivKey []byte
	var err error
	if privKey != nil {
		pemBlock, _ := pem.Decode(privKey)
		if pemBlock == nil {
			return nil, fmt.Errorf("invalid private key")
		}
		if pemBlock.Type == "ENCRYPTED PRIVATE KEY" {
			if pwd == "" {
				return nil, fmt.Errorf("key is encrypted need password to decrypt")
			}
			cryptoPrivKey, err = UnSeal(pwd, pemBlock.Bytes)
			if err != nil {
				return nil, fmt.Errorf("key is invalid or password is wrong")
			}
		}
	}
	return cryptoPrivKey, nil
}

func BIPSign(priv PrivateKey, data []byte) ([]byte, error) {
	return priv.(crypto.Signer).Sign(rand.Reader, data, crypto.SHA256)
}

func BIPVerify(pub PublicKey, data []byte, sig []byte) bool {
	switch pub.(type) {
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(pub.(*ecdsa.PublicKey), data, sig)
	default:
		return false
	}
}
