package wallet

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// StatePinnedInfo is a stub type representing a pinned token state record.
type StatePinnedInfo struct {
	TokenStateHash string
}

// GetStatePinnedInfo returns pin info for a token state hash, or nil if not pinned.
func (w *Wallet) GetStatePinnedInfo(tokenStateHash string) (*StatePinnedInfo, error) {
	// TODO(phase07): query token_state_pins table for this hash
	return nil, nil
}

// RemoveStalePeerDID removes a stale peer DID/peerID pair from the PeerDIDTable.
func (w *Wallet) RemoveStalePeerDID(did string, peerID string) error {
	// TODO(phase07): delete from peer_did table where did=$1 and peer_id=$2
	return nil
}

// GetPendingFTTokensOlderThan returns map of txID → []tokenIDs for FT tokens older than timeout.
func (w *Wallet) GetPendingFTTokensOlderThan(timeout time.Duration) (map[string][]string, error) {
	// TODO(phase07): query ft tokens with pending status older than now()-timeout
	return nil, nil
}

// ConfirmPendingFTTokens confirms pending FT tokens for a given transaction.
func (w *Wallet) ConfirmPendingFTTokens(txID string, tokenIDs []string) error {
	// TODO(phase07): update ft token status to confirmed for given txID and tokenIDs
	return nil
}

// GetPendingTokensOlderThan returns map of txID → []tokenIDs for RBT tokens older than timeout.
func (w *Wallet) GetPendingTokensOlderThan(timeout time.Duration) (map[string][]string, error) {
	// TODO(phase07): query tokens with pending status older than now()-timeout
	return nil, nil
}

// ConfirmPendingTokens confirms pending RBT tokens for a given transaction.
func (w *Wallet) ConfirmPendingTokens(txID string, tokenIDs []string) error {
	// TODO(phase07): update token status to confirmed for given txID and tokenIDs
	return nil
}

// GetTransactionHistoryChunk returns a paginated slice of RBT transaction history.
func (w *Wallet) GetTransactionHistoryChunk(batchSize int, offset int) ([]model.TransactionDetails, error) {
	// TODO(phase07): query transaction_history table with LIMIT/OFFSET
	return nil, nil
}

// GetFTTransactionHistoryChunk returns a paginated slice of FT transaction history.
func (w *Wallet) GetFTTransactionHistoryChunk(batchSize int, offset int) ([]model.TransactionDetails, error) {
	// TODO(phase07): query ft_transaction_history table with LIMIT/OFFSET
	return nil, nil
}
