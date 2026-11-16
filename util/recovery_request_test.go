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

func TestRecoveryRequestSignAndVerify(t *testing.T) {
	master, err := crypto.BIPGenerateMasterKeyFromMnemonic(crypto.BIPGenerateMnemonic())
	if err != nil {
		t.Fatal(err)
	}
	privateKey, publicKey, err := crypto.BIPGenerateChild(string(master), 0, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "user-did")
	if err := os.Mkdir(keyDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{constants.PvtKeyFileName: privateKey, constants.PubKeyFileName: publicKey} {
		if err := os.WriteFile(filepath.Join(keyDir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	signer := did.InitDIDLiteWithPassword("user-did", dir, "test-password")
	verifier := did.InitDIDLite("user-did", dir, nil)
	request := types.RecoveryRequest{DID: "user-did", Network: "testnet", Purpose: types.RecoveryRequestPurpose, Epoch: 1794700800}
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
