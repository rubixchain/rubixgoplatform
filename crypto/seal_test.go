package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

func TestSealUnsealRoundTrip(t *testing.T) {
	password := "hunter2"
	plaintext := []byte("hello, argon2id world")

	sealed, err := Seal(password, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if sealed[0] != sealVersion1 {
		t.Fatalf("expected version byte %d, got %d", sealVersion1, sealed[0])
	}

	got, err := UnSeal(password, sealed)
	if err != nil {
		t.Fatalf("UnSeal: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestSealUnsealWrongPassword(t *testing.T) {
	sealed, err := Seal("correct", []byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	_, err = UnSeal("wrong", sealed)
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

func TestSealUniquePerCall(t *testing.T) {
	password := "same-password"
	plaintext := []byte("same-data")

	a, _ := Seal(password, plaintext)
	b, _ := Seal(password, plaintext)
	if bytes.Equal(a, b) {
		t.Fatal("two Seal calls produced identical output; salt/nonce should differ")
	}
}

func TestLegacyBackwardCompat(t *testing.T) {
	password := "legacy-pass"
	plaintext := []byte("legacy data payload")

	legacy, err := sealWithSHA256(password, plaintext)
	if err != nil {
		t.Fatalf("legacy seal: %v", err)
	}

	// First byte of AES-GCM nonce is unlikely to be exactly 1,
	// but more importantly the old format never starts with version 1
	// followed by valid argon2 structure. UnSeal should fall back.
	got, err := UnSeal(password, legacy)
	if err != nil {
		t.Fatalf("UnSeal (legacy): %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("legacy round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestLegacyViaExportedHelper(t *testing.T) {
	password := "test-pass"
	plaintext := []byte("round-trip via SealLegacy")

	sealed, err := SealLegacy(password, plaintext)
	if err != nil {
		t.Fatalf("SealLegacy: %v", err)
	}
	got, err := UnSeal(password, sealed)
	if err != nil {
		t.Fatalf("UnSeal: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("mismatch: got %q, want %q", got, plaintext)
	}
}

func TestUnSealEmptyData(t *testing.T) {
	_, err := UnSeal("key", nil)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
	_, err = UnSeal("key", []byte{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestV1TruncatedData(t *testing.T) {
	short := []byte{sealVersion1, 0x00, 0x01}
	_, err := UnSeal("key", short)
	if err == nil {
		t.Fatal("expected error for truncated v1 data")
	}
}

// sealWithSHA256 replicates the original (pre-argon2) Seal behaviour
// so we can produce genuine legacy ciphertext inside tests.
func sealWithSHA256(password string, data []byte) ([]byte, error) {
	h := sha256.New()
	h.Write([]byte(password))
	k := h.Sum(nil)
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}
