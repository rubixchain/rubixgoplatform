package wallet

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// Token is a legacy stub type matching the old SQLite tokens table schema.
type Token struct {
	TokenID       string
	TokenStatus   int
	TransactionID string
	TokenValue    float64
	DID           string
}

// ── Transaction query stubs ───────────────────────────────────────────────────

// GetTransactionByDID returns all transactions for the given DID.
// TODO(phase09): implement using PostgreSQL transaction_history table.
func (w *Wallet) GetTransactionByDID(did string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// GetTransactionBySender returns transactions where the given DID is the sender.
// TODO(phase09): implement using PostgreSQL transaction_history table.
func (w *Wallet) GetTransactionBySender(did string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// GetTransactionByReceiver returns transactions where the given DID is the receiver.
// TODO(phase09): implement using PostgreSQL transaction_history table.
func (w *Wallet) GetTransactionByReceiver(did string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// GetTransactionByComment returns transactions matching the given comment.
// TODO(phase09): implement using PostgreSQL transaction_history table.
func (w *Wallet) GetTransactionByComment(comment string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// ── FT transaction query stubs ────────────────────────────────────────────────

// GetAllFTTransactionDetailsByDID returns all FT transactions for the given DID.
// TODO(phase09): implement using PostgreSQL ft_transaction_history table.
func (w *Wallet) GetAllFTTransactionDetailsByDID(did string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// GetFTTransactionBySender returns FT transactions where the given DID is the sender.
// TODO(phase09): implement using PostgreSQL ft_transaction_history table.
func (w *Wallet) GetFTTransactionBySender(did string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// GetFTTransactionByReceiver returns FT transactions where the given DID is the receiver.
// TODO(phase09): implement using PostgreSQL ft_transaction_history table.
func (w *Wallet) GetFTTransactionByReceiver(did string) ([]model.TransactionDetails, error) {
	return nil, nil
}

// ── Token status stubs ────────────────────────────────────────────────────────

// GetTokenStatus retrieves the current status of a token.
// TODO(phase09): implement using PostgreSQL tokens table.
func (w *Wallet) GetTokenStatus(did string, token string, tokenType int) (model.TokenStatusResponse, error) {
	return model.TokenStatusResponse{}, nil
}

// ── Pending token rollback stubs ──────────────────────────────────────────────

// RollbackPendingTokens rolls back pending RBT tokens for the given transaction.
// TODO(phase09): implement using PostgreSQL tokens table.
func (w *Wallet) RollbackPendingTokens(txID string, tokenIDs []string) error {
	return nil
}

// RollbackPendingFTTokens rolls back pending FT tokens for the given transaction.
// TODO(phase09): implement using PostgreSQL ft_tokens table.
func (w *Wallet) RollbackPendingFTTokens(txID string, tokenIDs []string) error {
	return nil
}
