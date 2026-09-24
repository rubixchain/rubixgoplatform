package core

import (
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// testCID returns a well-formed 46-character Qm CID that differs by the seed.
func testCID(seed byte) string {
	return "Qm" + strings.Repeat(string(seed), 44)
}

func TestValidateRequestTokenIDs(t *testing.T) {
	scA, scB := testCID('a'), testCID('b')
	nftA, nftB := testCID('c'), testCID('d')

	t.Run("distinct NFTs and contracts pass", func(t *testing.T) {
		req := &models.TransactionRequest{Tokens: models.TransactionTokenDetails{
			NFT:           []models.NFTInfo{{NFTId: nftA}, {NFTId: nftB}},
			SmartContract: []models.SmartContractInfo{{SmartContractId: scA}, {SmartContractId: scB}},
		}}
		if err := validateRequestTokenIDs(req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty request passes", func(t *testing.T) {
		if err := validateRequestTokenIDs(&models.TransactionRequest{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// The CI shape: one contract listed twice in a single request.
	t.Run("same smart contract twice is rejected", func(t *testing.T) {
		req := &models.TransactionRequest{Tokens: models.TransactionTokenDetails{
			SmartContract: []models.SmartContractInfo{{SmartContractId: scA}, {SmartContractId: scA}},
		}}
		err := validateRequestTokenIDs(req)
		if err == nil {
			t.Fatal("expected error for duplicated smart contract")
		}
		if !strings.Contains(err.Error(), scA) || !strings.Contains(err.Error(), "more than once") {
			t.Fatalf("error should name the contract and the defect, got: %v", err)
		}
	})

	t.Run("same NFT twice is rejected", func(t *testing.T) {
		req := &models.TransactionRequest{Tokens: models.TransactionTokenDetails{
			NFT: []models.NFTInfo{{NFTId: nftA}, {NFTId: nftB}, {NFTId: nftA}},
		}}
		if err := validateRequestTokenIDs(req); err == nil {
			t.Fatal("expected error for duplicated NFT")
		}
	})

	t.Run("same ID as NFT and as contract is allowed here", func(t *testing.T) {
		// Different token types cannot share an ID in practice; the payload check covers it if they do.
		req := &models.TransactionRequest{Tokens: models.TransactionTokenDetails{
			NFT:           []models.NFTInfo{{NFTId: nftA}},
			SmartContract: []models.SmartContractInfo{{SmartContractId: nftA}},
		}}
		if err := validateRequestTokenIDs(req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed IDs are still rejected", func(t *testing.T) {
		req := &models.TransactionRequest{Tokens: models.TransactionTokenDetails{
			SmartContract: []models.SmartContractInfo{{SmartContractId: "not-a-cid"}},
		}}
		if err := validateRequestTokenIDs(req); err == nil || !strings.Contains(err.Error(), "invalid CID format") {
			t.Fatalf("expected CID format error, got: %v", err)
		}
		req = &models.TransactionRequest{Tokens: models.TransactionTokenDetails{
			NFT: []models.NFTInfo{{NFTId: ""}},
		}}
		if err := validateRequestTokenIDs(req); err == nil || !strings.Contains(err.Error(), "required") {
			t.Fatalf("expected empty-id error, got: %v", err)
		}
	})
}
