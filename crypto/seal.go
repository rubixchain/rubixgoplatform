package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// Seal will seal data using the AES-GCM
func Seal(key string, data []byte) ([]byte, error) {
	h := sha256.New()
	h.Write([]byte(key))
	k := h.Sum(nil)
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

// UnSeal will open data using the AES-GCM
func UnSeal(key string, data []byte) ([]byte, error) {
	h := sha256.New()
	h.Write([]byte(key))
	k := h.Sum(nil)
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

	nonce, cipher := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, cipher, nil)
}
