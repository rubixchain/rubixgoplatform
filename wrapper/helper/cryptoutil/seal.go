package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

func SealBytes(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, data, nil)
	out := nonce
	out = append(out, ciphertext...)
	return out, nil
}

func UnSealBytes(key []byte, data []byte) ([]byte, error) {

	if len(data) < 13 {
		return nil, fmt.Errorf("invalid data")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := data[:12]

	cipherData := data[12:]

	plainData, err := aesgcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return nil, err
	}
	return plainData, nil
}
