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

// Generate a Bip32 HD wallet for the mnemonic and a user supplied randomness
// here we are reusing the password used for sealing for generating random seed
func BIPGenerateKeyPair(cfg *CryptoConfig) ([]byte, []byte, error) {
	var privateKeyBytes []byte
	var publicKeyBytes []byte

	var err error
	if cfg.Alg == 0 {
		entropy, _ := bip39.NewEntropy(256)
		mnemonic, _ := bip39.NewMnemonic(entropy)
		seed := bip39.NewSeed(mnemonic, cfg.Pwd)
		masterKey, _ := bip32.NewMasterKey(seed)
		privateKey, err := masterKey.NewChildKey(0)
		if err != nil {
			return nil, nil, err
		}
		publicKey := privateKey.PublicKey()

		privateKeyBytes = privateKey.Key
		publicKeyBytes = publicKey.Key
	} else {
		return nil, nil, fmt.Errorf("unsupported algorithm")
	}
	if err != nil {
		return nil, nil, err
	}
	var pemEncPriv []byte
	if cfg.Pwd != "" {
		encBlock, err := Seal(cfg.Pwd, privateKeyBytes)
		if err != nil {
			return nil, nil, err
		}
		_, err = UnSeal(cfg.Pwd, encBlock)
		if err != nil {
			return nil, nil, err
		}
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: encBlock})
	} else {
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})
	}
	pemEncPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyBytes})

	return pemEncPriv, pemEncPub, nil
}

// GenerateKeyPair will generate key pair based on the configuration
func BIPDecodeKeyPair(pwd string, privKey []byte, pubKey []byte) ([]byte, []byte, error) {
	var cryptoPrivKey []byte
	var cryptoPubKey []byte
	var err error
	if privKey != nil {
		pemBlock, _ := pem.Decode(privKey)
		if pemBlock == nil {
			return nil, nil, fmt.Errorf("invalid private key")
		}
		fmt.Println(" pemBlock ", pemBlock.Bytes)
		fmt.Println(" pwd ", pwd)

		if pemBlock.Type == "ENCRYPTED PRIVATE KEY" {
			if pwd == "" {
				return nil, nil, fmt.Errorf("key is encrypted need password to decrypt")
			}
			cryptoPrivKey, err = UnSeal(pwd, pemBlock.Bytes)
			if err != nil {
				return nil, nil, fmt.Errorf("key is invalid or password is wrong")
			}
		}
	}
	if pubKey != nil {
		cryptoPubKey, _ := pem.Decode(pubKey)
		if cryptoPubKey == nil {
			return nil, nil, fmt.Errorf("invalid public key")
		}
	}

	return cryptoPrivKey, cryptoPubKey, nil
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
