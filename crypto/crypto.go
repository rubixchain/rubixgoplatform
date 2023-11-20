package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// PublicKey represents a public key using an unspecified algorithm.
type PublicKey interface{}

// PrivateKey represents a private key using an unspecified algorithm.
type PrivateKey interface{}

// CryptoAlgType is algorithm type
type CryptoAlgType int

const (
	RSA2048 CryptoAlgType = iota
	ECDSAP256
)

// CryptoConfig is configuration for the crypto
type CryptoConfig struct {
	Alg CryptoAlgType
	Pwd string
}

// GenerateKeyPair will generate key pair based on the configuration
func GenerateKeyPair(cfg *CryptoConfig) ([]byte, []byte, error) {
	var privKey interface{}
	var pubKey interface{}
	var err error
	switch cfg.Alg {
	case RSA2048:
		privKey, err = rsa.GenerateKey(rand.Reader, 2048)
		pubKey = privKey.(*rsa.PrivateKey).PublicKey
	case ECDSAP256:
		privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		pubKey = &privKey.(*ecdsa.PrivateKey).PublicKey
	default:
		return nil, nil, fmt.Errorf("unsupported algorithm")
	}
	if err != nil {
		return nil, nil, err
	}
	x509Priv, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}
	x509Pub, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return nil, nil, err
	}
	var pemEncPriv []byte
	if cfg.Pwd != "" {
		encBlock, err := Seal(cfg.Pwd, x509Priv)
		if err != nil {
			return nil, nil, err
		}
		_, err = UnSeal(cfg.Pwd, encBlock)
		if err != nil {
			return nil, nil, err
		}
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: encBlock})
	} else {
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Priv})
	}
	pemEncPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509Pub})

	return pemEncPriv, pemEncPub, nil
}

// GenerateKeyPair will generate key pair based on the configuration
func DecodeKeyPair(pwd string, privKey []byte, pubKey []byte) (PrivateKey, PublicKey, error) {
	var cryptoPrivKey interface{}
	var cryptoPubKey interface{}
	var err error
	if privKey != nil {
		pemBlock, _ := pem.Decode(privKey)
		if pemBlock == nil {
			return nil, nil, fmt.Errorf("invalid private key")
		}
		if pemBlock.Type == "ENCRYPTED PRIVATE KEY" {
			if pwd == "" {
				return nil, nil, fmt.Errorf("key is encrypted need password to decrypt")
			}
			decData, err := UnSeal(pwd, pemBlock.Bytes)
			if err != nil {
				return nil, nil, fmt.Errorf("key is invalid or password is wrong")
			}
			cryptoPrivKey, err = x509.ParsePKCS8PrivateKey(decData)
			if err != nil {
				return nil, nil, err
			}
		} else {
			cryptoPrivKey, err = x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if pubKey != nil {
		pemBlock, _ := pem.Decode(pubKey)
		if pemBlock == nil {
			return nil, nil, fmt.Errorf("invalid public key")
		}
		cryptoPubKey, err = x509.ParsePKIXPublicKey(pemBlock.Bytes)
		if err != nil {
			return nil, nil, err
		}
	}

	return cryptoPrivKey, cryptoPubKey, nil
}

func Sign(priv PrivateKey, data []byte) ([]byte, error) {
	return priv.(crypto.Signer).Sign(rand.Reader, data, crypto.SHA256)
}

func Verify(pub PublicKey, data []byte, sig []byte) bool {
	switch pub.(type) {
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(pub.(*ecdsa.PublicKey), data, sig)
	default:
		return false
	}
}
