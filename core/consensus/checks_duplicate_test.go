package consensus

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func TestValidateNoDuplicateTokens(t *testing.T) {
	t.Run("nil transaction info errors", func(t *testing.T) {
		if err := ValidateNoDuplicateTokens(nil); err == nil {
			t.Fatal("expected error for nil transaction info")
		}
	})

	// A split-then-transfer plus an SC deploy: burnt parents and collateral are committed, children and the SC are transferred.
	t.Run("legitimate multi-asset payload passes", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens.RBT = tokenList("child-0.005", "child-0.995")
		tx.Tokens.FT = tokenList("ft-1", "ft-2")
		tx.Tokens.NFT = tokenList("nft-parent", "nft-child-1", "nft-child-2")
		tx.Tokens.SmartContract = tokenList("sc-1", "sc-2")
		tx.CommittedTokens = tokenList("parent-1", "collateral-1")
		if err := ValidateNoDuplicateTokens(&tx); err != nil {
			t.Fatalf("unexpected error on a legitimate payload: %v", err)
		}
	})

	t.Run("nil and empty entries are ignored", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens.SmartContract = []*models.TokenInfo{nil, {TokenID: ""}, {TokenID: ""}}
		if err := ValidateNoDuplicateTokens(&tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nil tokens with committed only passes", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens = nil
		tx.CommittedTokens = tokenList("parent-1")
		if err := ValidateNoDuplicateTokens(&tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// The shape seen in CI: the same contract listed twice in one request.
	t.Run("same smart contract twice is rejected", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens.SmartContract = tokenList("sc-1", "sc-1")
		err := ValidateNoDuplicateTokens(&tx)
		if err == nil {
			t.Fatal("expected error for duplicated smart contract")
		}
		if !strings.Contains(err.Error(), "sc-1") || !strings.Contains(err.Error(), "more than once") {
			t.Fatalf("error should name the token and the defect, got: %v", err)
		}
	})

	t.Run("same NFT twice is rejected", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens.NFT = tokenList("nft-1", "nft-1")
		if err := ValidateNoDuplicateTokens(&tx); err == nil {
			t.Fatal("expected error for duplicated NFT")
		}
	})

	t.Run("same RBT token twice is rejected", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens.RBT = tokenList("1_1", "1_1")
		if err := ValidateNoDuplicateTokens(&tx); err == nil {
			t.Fatal("expected error for duplicated RBT token")
		}
	})

	t.Run("token in two different lists is rejected and both roles are named", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens.RBT = tokenList("tok-1")
		tx.CommittedTokens = tokenList("tok-1")
		err := ValidateNoDuplicateTokens(&tx)
		if err == nil {
			t.Fatal("expected error for a token that is both transferred and committed")
		}
		if !strings.Contains(err.Error(), "transferred RBT") || !strings.Contains(err.Error(), "committed") {
			t.Fatalf("error should name both roles, got: %v", err)
		}
	})

	// Pledge tokens are the quorum's own and are covered by ValidatePledgeTransferDisjoint, not here.
	t.Run("pledge tokens are not part of the check", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Quorums = []*models.QuorumInfo{
			{Did: validDID('q'), Tokens: tokenList("q-1", "q-1")},
		}
		if err := ValidateNoDuplicateTokens(&tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// A duplicated token must be refused before signature or chain work, on quorum and fullnode alike.
func TestValidateTransaction_FailsOnDuplicateToken(t *testing.T) {
	tx := baseValidTxnInfo()
	tx.Tokens.SmartContract = tokenList("sc-1", "sc-1")

	infoBytes, _ := json.Marshal(&tx)
	sigBytes, _ := json.Marshal(&models.Signature{})
	storedTx := &models.Transactions{ID: "anything", Info: infoBytes, Signature: sigBytes}

	for _, isFullnode := range []bool{false, true} {
		_, err := ValidateTransaction(
			storedTx,
			isFullnode,
			nil,
			nil,
			&stubDIDCrypto{Verify: true},
			map[string]types.DIDCrypto{},
			false, false, false,
			func(string, string) error { return nil },
			func(string, []string, map[string]string, []string) error { return nil },
			func([]string) (map[string]string, error) { return map[string]string{}, nil },
			func(string) (*models.TransactionInfo, error) { return nil, nil },
			func(string) (string, bool, error) { return "", false, nil },
			func(string, string) (*models.Transactions, error) { return nil, nil },
			false,
		)
		if err == nil {
			t.Fatalf("isFullnode=%v: expected error for duplicated token, got nil", isFullnode)
		}
		if !strings.Contains(err.Error(), "more than once") {
			t.Fatalf("isFullnode=%v: expected duplicate-token error, got: %v", isFullnode, err)
		}
	}
}
