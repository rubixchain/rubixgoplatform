package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncrypt(t *testing.T) {
	key := make([]byte, 16)

	raw := make([]byte, 16)

	rand.Read(key)

	rand.Read(raw)

	encStr, err := Encrypt(raw, key)

	if err != nil {
		t.Fatal("Error in Encryption")
	}

	decStr, err := Decrypt(encStr, key)

	if err != nil {
		t.Fatal("Error in Decryption")
	}

	if bytes.Compare(raw, []byte(decStr)) != 0 {
		t.Fatal("Error in Encryption & Decryption")
	}

}
