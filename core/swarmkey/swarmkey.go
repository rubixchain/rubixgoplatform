// Package swarmkey derives the fingerprint of an IPFS swarm key. The
// fingerprint identifies which private network a node has joined, and is used
// to tell a Rubix operated network apart from a custom one.
package swarmkey

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"golang.org/x/crypto/salsa20"
	"golang.org/x/crypto/sha3"
)

// Fingerprints of the swarm keys Rubix operates. A node whose key matches
// neither of these is on a custom network. The mainnet value was read from the
// "Swarm key fingerprint" line a kubo daemon logs at startup, and a test
// rederives both from the checked in key files.
const (
	MainnetFingerprint string = "823fe2e55e7c39f25fd44bcf15c53ed5"
	TestnetFingerprint string = "6e86829be0b445435ed2bf3036e40de5"
)

const (
	base16Line       string = "/base16/"
	fingerprintNonce string = "finprint"
	pskLength        int    = 32
)

// parseKey extracts the pre-shared key from the contents of a swarm key file.
// Line endings are not significant, so a file saved with CRLF parses the same
// as one saved with LF.
func parseKey(data []byte) ([]byte, error) {
	var (
		base16  bool
		payload string
	)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case line == base16Line:
			base16 = true
		case strings.HasPrefix(line, "/"):
		default:
			payload = line
		}
	}
	if !base16 {
		return nil, fmt.Errorf("swarm key is not base16 encoded")
	}
	if payload == "" {
		return nil, fmt.Errorf("swarm key has no key material")
	}

	psk, err := hex.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode swarm key material: %w", err)
	}
	if len(psk) != pskLength {
		return nil, fmt.Errorf("swarm key must be %d bytes, got %d", pskLength, len(psk))
	}
	return psk, nil
}

// fingerprint returns the hex fingerprint of a pre-shared key. It follows the
// same construction as kubo, so the result matches the "Swarm key fingerprint"
// line the IPFS daemon logs on startup: salsa20 over 64 zero bytes keyed by the
// pre-shared key, reduced to 16 bytes with shake128. The key is not hashed
// directly, so a weakness in shake128 would not expose it.
func fingerprint(psk []byte) string {
	var key [32]byte
	copy(key[:], psk)

	encoded := make([]byte, 64)
	salsa20.XORKeyStream(encoded, encoded, []byte(fingerprintNonce), &key)

	out := make([]byte, 16)
	shake := sha3.NewShake128()
	shake.Write(encoded)
	shake.Read(out)

	return hex.EncodeToString(out)
}

// FingerprintFile reads a swarm key file and returns its fingerprint.
func FingerprintFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read swarm key %s: %w", path, err)
	}
	psk, err := parseKey(data)
	if err != nil {
		return "", fmt.Errorf("failed to parse swarm key %s: %w", path, err)
	}
	return fingerprint(psk), nil
}

// IsCanonical reports whether a fingerprint is the one Rubix operates for the
// given network mode. A false result on testnet means the node is on a custom
// network and must not be validated against the Rubix minter allowlist.
// Localnet has no fixed key, so it is never canonical.
func IsCanonical(networkMode string, fingerprint string) bool {
	switch networkMode {
	case constants.NetworkMode_Mainnet:
		return fingerprint == MainnetFingerprint
	case constants.NetworkMode_Testnet:
		return fingerprint == TestnetFingerprint
	default:
		return false
	}
}
