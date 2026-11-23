package util_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
)

// testRecoveryDID creates a DID key pair named name and returns a signing and
// a verifying handle for it.
func testRecoveryDID(t *testing.T, name string) (signer, verifier types.DIDCrypto) {
	t.Helper()
	master, err := crypto.BIPGenerateMasterKeyFromMnemonic(crypto.BIPGenerateMnemonic())
	if err != nil {
		t.Fatal(err)
	}
	privateKey, publicKey, err := crypto.BIPGenerateChild(string(master), 0, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keyDir := filepath.Join(dir, name)
	if err := os.Mkdir(keyDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{constants.PvtKeyFileName: privateKey, constants.PubKeyFileName: publicKey} {
		if err := os.WriteFile(filepath.Join(keyDir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return did.InitDIDLiteWithPassword(name, dir, "test-password"), did.InitDIDLite(name, dir, nil)
}

func TestRecoveryRequestSignAndVerify(t *testing.T) {
	signer, verifier := testRecoveryDID(t, "user-did")
	request := types.RecoveryRequest{DID: "user-did", Network: "testnet", Purpose: constants.RecoveryRequest_Purpose, Epoch: 1794700800}
	if err := util.SignRecoveryRequest(signer, &request); err != nil {
		t.Fatal(err)
	}
	if err := util.VerifyRecoveryRequest(verifier, &request, "testnet"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		change  func(*types.RecoveryRequest)
		network string
	}{
		{"changed epoch", func(r *types.RecoveryRequest) { r.Epoch++ }, "testnet"},
		{"changed network", func(r *types.RecoveryRequest) { r.Network = "mainnet" }, "mainnet"},
		{"wrong node network", func(r *types.RecoveryRequest) {}, "mainnet"},
		{"wrong DID", func(r *types.RecoveryRequest) { r.DID = "other-did" }, "testnet"},
		{"wrong purpose", func(r *types.RecoveryRequest) { r.Purpose = "transfer" }, "testnet"},
		{"bad signature", func(r *types.RecoveryRequest) { r.Signature = "!" }, "testnet"},
		{"missing signature", func(r *types.RecoveryRequest) { r.Signature = "" }, "testnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := request
			tc.change(&changed)
			if err := util.VerifyRecoveryRequest(verifier, &changed, tc.network); err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}
}

func TestRecoveryChainsNoticeSignAndVerify(t *testing.T) {
	signer, verifier := testRecoveryDID(t, "quorum-did")
	notice := types.RecoveryChainsNotice{
		RequestID:       "req",
		CommitterDID:    "quorum-did",
		CommitterType:   "quorum",
		CommitterPeerID: "peer",
		Tokens:          []types.RecoveryTokenCommitment{{TokenID: "T1", ChainRoot: "root"}},
	}
	if err := util.SignRecoveryChainsNotice(signer, &notice); err != nil {
		t.Fatal(err)
	}
	if err := util.VerifyRecoveryChainsNotice(verifier, &notice); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*types.RecoveryChainsNotice)
	}{
		{"changed peer", func(n *types.RecoveryChainsNotice) { n.CommitterPeerID = "attacker" }},
		{"changed tokens", func(n *types.RecoveryChainsNotice) {
			n.Tokens = []types.RecoveryTokenCommitment{{TokenID: "T2", ChainRoot: "root"}}
		}},
		{"other committer DID", func(n *types.RecoveryChainsNotice) { n.CommitterDID = "other-did" }},
		{"changed committer type", func(n *types.RecoveryChainsNotice) { n.CommitterType = "fullnode" }},
		{"missing signature", func(n *types.RecoveryChainsNotice) { n.Signature = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := notice
			tc.change(&changed)
			if err := util.VerifyRecoveryChainsNotice(verifier, &changed); err == nil {
				t.Fatal("invalid notice was accepted")
			}
		})
	}
}
