package swarmkey

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// TestCanonicalFingerprintsMatchCheckedInKeys guards the constants against
// drifting away from the keys they name, and pins them to what kubo derives.
// A rotated key file fails here, at the commit that rotates it.
func TestCanonicalFingerprintsMatchCheckedInKeys(t *testing.T) {
	cases := []struct {
		network  string
		keyFile  string
		expected string
	}{
		{constants.NetworkMode_Mainnet, "swarm.key", MainnetFingerprint},
		{constants.NetworkMode_Testnet, "testnetswarm.key", TestnetFingerprint},
	}

	for _, c := range cases {
		t.Run(c.network, func(t *testing.T) {
			got, err := FingerprintFile(filepath.Join("..", "..", c.keyFile))
			if err != nil {
				t.Fatalf("FingerprintFile: %v", err)
			}
			if got != c.expected {
				t.Fatalf("%s derives to %s, but the constant says %s", c.keyFile, got, c.expected)
			}
			if !IsCanonical(c.network, got) {
				t.Fatalf("IsCanonical(%s, %s) = false", c.network, got)
			}
		})
	}
}

// TestParseKeyIgnoresLineEndings covers a key file saved with different line
// endings, which is what happens when the same key is checked out on Windows
// and on Linux. The material is unchanged, so the fingerprint must not move.
func TestParseKeyIgnoresLineEndings(t *testing.T) {
	const payload = "7a705eda2340e025bcf51899516aba7b30fb5a4c0c7fdbdbd5cb2bfacaddcc9d"
	lf := "/key/swarm/psk/1.0.0/\n/base16/\n" + payload + "\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")

	lfKey, err := parseKey([]byte(lf))
	if err != nil {
		t.Fatalf("parseKey(lf): %v", err)
	}
	crlfKey, err := parseKey([]byte(crlf))
	if err != nil {
		t.Fatalf("parseKey(crlf): %v", err)
	}
	if string(lfKey) != string(crlfKey) {
		t.Fatal("line endings changed the parsed key material")
	}
	if fingerprint(lfKey) != fingerprint(crlfKey) {
		t.Fatal("line endings changed the fingerprint")
	}
	if fingerprint(lfKey) != MainnetFingerprint {
		t.Fatalf("payload is the mainnet key, expected %s, got %s", MainnetFingerprint, fingerprint(lfKey))
	}
}

func TestParseKeyRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"no encoding line": "/key/swarm/psk/1.0.0/\ndeadbeef\n",
		"no key material":  "/key/swarm/psk/1.0.0/\n/base16/\n",
		"not hex":          "/key/swarm/psk/1.0.0/\n/base16/\nnothexatall\n",
		"wrong length":     "/key/swarm/psk/1.0.0/\n/base16/\ndeadbeef\n",
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseKey([]byte(data)); err == nil {
				t.Fatal("expected an error, got none")
			}
		})
	}
}

// TestIsCanonicalRejectsCustomNetwork is the property the minter allowlist gate
// depends on: an unknown key must never be reported as a Rubix network.
func TestIsCanonicalRejectsCustomNetwork(t *testing.T) {
	const custom = "00000000000000000000000000000000"

	for _, network := range []string{
		constants.NetworkMode_Mainnet,
		constants.NetworkMode_Testnet,
		constants.NetworkMode_Localnet,
	} {
		if IsCanonical(network, custom) {
			t.Fatalf("IsCanonical(%s, custom) = true", network)
		}
	}
}
