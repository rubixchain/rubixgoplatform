package core

import (
	"bytes"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path"
	"strings"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/rubixchain/rubixgoplatform/constants"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// testDID is a DID-shaped string. The exact value is irrelevant to
// GetPubKeyByDID (the handler does the format validation); it only has to be a
// usable directory name.
const testDID = "bafybmiavtestdidavtestdidavtestdidavtestdidavtestdidavtest0"

// newPubKeyPEM returns a "PUBLIC KEY" PEM block holding a real 65-byte
// uncompressed secp256k1 point, plus those raw bytes, exactly as
// crypto.BIPGenerateChild writes pubKey.pem.
func newPubKeyPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	raw := privKey.PubKey().SerializeUncompressed()
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: raw}), raw
}

// newCoreWithDIDDir returns a Core wired up with just enough state for the
// local branch of GetPubKeyByDID: a did directory and no IPFS.
func newCoreWithDIDDir(t *testing.T) *Core {
	t.Helper()
	return &Core{didDir: t.TempDir()}
}

// writeDIDPubKey drops pemBytes at <didDir>/<did>/pubKey.pem.
func writeDIDPubKey(t *testing.T, c *Core, did string, pemBytes []byte) {
	t.Helper()
	dir := path.Join(c.didDir, did)
	if err := os.MkdirAll(dir, os.ModeDir|os.ModePerm); err != nil {
		t.Fatalf("failed to create did dir: %v", err)
	}
	if err := os.WriteFile(path.Join(dir, constants.PubKeyFileName), pemBytes, 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", constants.PubKeyFileName, err)
	}
}

// -----------------------------------------------------------------------------
// GetPubKeyByDID
//
// All of these run offline: the local branch is a plain file read, and the IPFS
// branch is reached only to assert how it fails when there is no IPFS to reach.
// -----------------------------------------------------------------------------

// A locally held DID is answered from disk: the response carries the hex of the
// PEM-decoded key.
func TestGetPubKeyByDIDLocal(t *testing.T) {
	c := newCoreWithDIDDir(t)
	pemBytes, raw := newPubKeyPEM(t)
	writeDIDPubKey(t, c, testDID, pemBytes)

	// No IPFS is wired up on this Core, so reaching FetchDID at all would fail
	// the test -- proving a local hit never consults the network.
	res, err := c.GetPubKeyByDID(testDID)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if res.DID != testDID {
		t.Errorf("did: got %q, want %q", res.DID, testDID)
	}

	// Hex, not base64 -- the same encoding DIDCreate.PubKey takes, so the
	// response can be fed straight back into DID-from-public-key.
	if want := hex.EncodeToString(raw); res.PublicKey != want {
		t.Errorf("public_key: got %q, want %q", res.PublicKey, want)
	}
	if len(res.PublicKey) != 130 {
		t.Errorf("public_key should be 130 hex chars (65 bytes uncompressed), got %d", len(res.PublicKey))
	}

	// The response must be the 65-byte point itself, not the PEM wrapper.
	got, err := hex.DecodeString(res.PublicKey)
	if err != nil {
		t.Fatalf("failed to hex-decode response: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("decoded key does not match the generated key")
	}
	if _, err := secp256k1.ParsePubKey(got); err != nil {
		t.Errorf("returned key does not parse as a secp256k1 point: %v", err)
	}
}

// A pubKey.pem that is present but not valid PEM must fail loudly rather than
// silently falling through to the network.
func TestGetPubKeyByDIDLocalMalformed(t *testing.T) {
	c := newCoreWithDIDDir(t)
	writeDIDPubKey(t, c, testDID, []byte("this is not a pem block"))

	res, err := c.GetPubKeyByDID(testDID)
	if err == nil {
		t.Fatalf("expected an error for a malformed pubKey.pem, got result: %+v", res)
	}
	if !strings.Contains(err.Error(), "failed to decode") {
		t.Errorf("error should name the decode failure, got: %v", err)
	}
}

// With no local copy and no IPFS to fall back to, the DID is unresolvable and
// the error must say so for both routes.
func TestGetPubKeyByDIDNotLocalNoIPFS(t *testing.T) {
	c := newCoreWithDIDDir(t)

	res, err := c.GetPubKeyByDID(testDID)
	if err == nil {
		t.Fatalf("expected an error for an unresolvable DID, got result: %+v", res)
	}
	if !strings.Contains(err.Error(), "not present locally") {
		t.Errorf("error should report the local miss, got: %v", err)
	}
	if !strings.Contains(err.Error(), "IPFS is not initialised") {
		t.Errorf("error should report why IPFS resolution failed, got: %v", err)
	}
}

// A successful fetch caches the DID locally (that is FetchDID's job), but a
// FAILED one must not leave a pubKey.pem behind -- otherwise the next lookup
// would take the local branch and serve a partial or empty key instead of
// retrying the fetch.
func TestGetPubKeyByDIDFailedLookupLeavesNoKey(t *testing.T) {
	c := newCoreWithDIDDir(t)

	if _, err := c.GetPubKeyByDID(testDID); err == nil {
		t.Fatal("expected an error for an unresolvable DID")
	}
	if _, err := os.Stat(path.Join(c.didDir, testDID, constants.PubKeyFileName)); !os.IsNotExist(err) {
		t.Errorf("%s must not exist after a failed lookup, stat err: %v", constants.PubKeyFileName, err)
	}
}
