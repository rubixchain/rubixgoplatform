package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	sealVersion1  byte = 1
	argon2SaltLen      = 16
	argon2Time         = 1
	argon2Memory       = 64 * 1024
	argon2Threads      = 4
	argon2KeyLen       = 32
)

func deriveKeyArgon2(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

func deriveKeySHA256(password string) []byte {
	h := sha256.New()
	h.Write([]byte(password))
	return h.Sum(nil)
}

// Seal encrypts data using AES-256-GCM with an argon2id-derived key.
// Output format: [version=1][16-byte salt][nonce + ciphertext]
func Seal(key string, data []byte) ([]byte, error) {
	salt, err := GetRandBytes(rand.Reader, argon2SaltLen)
	if err != nil {
		return nil, err
	}
	k := deriveKeyArgon2(key, salt)
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonce, err := GetRandBytes(rand.Reader, gcm.NonceSize())
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	out := make([]byte, 0, 1+argon2SaltLen+len(ciphertext))
	out = append(out, sealVersion1)
	out = append(out, salt...)
	out = append(out, ciphertext...)
	return out, nil
}

// UnSeal decrypts data produced by Seal.
// It auto-detects the format: version 1 uses argon2id; anything else
// falls back to the legacy sha256 derivation for backward compatibility.
func UnSeal(key string, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("invalid data")
	}

	if data[0] == sealVersion1 {
		return unsealV1(key, data)
	}
	return unsealLegacy(key, data)
}

func unsealV1(key string, data []byte) ([]byte, error) {
	minLen := 1 + argon2SaltLen + 1
	if len(data) < minLen {
		return nil, fmt.Errorf("invalid v1 sealed data: too short")
	}
	salt := data[1 : 1+argon2SaltLen]
	payload := data[1+argon2SaltLen:]

	k := deriveKeyArgon2(key, salt)
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return nil, fmt.Errorf("invalid v1 sealed data: payload too short")
	}
	nonce, ciphertext := payload[:nonceSize], payload[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func unsealLegacy(key string, data []byte) ([]byte, error) {
	k := deriveKeySHA256(key)
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("invalid data")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// SealLegacy encrypts using the old sha256-based key derivation.
// Exposed only for testing backward compatibility.
func SealLegacy(key string, data []byte) ([]byte, error) {
	k := deriveKeySHA256(key)
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonce, err := GetRandBytes(rand.Reader, gcm.NonceSize())
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}
