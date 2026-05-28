package types

import "github.com/rubixchain/rubixgoplatform/types/models"

// SyncedTxn is one entry of the per-token chain returned by the
// sync-token-chain API. The explorer uses Role + PreviousTransactionID to
// validate chain contiguity (the latter is essential for unpledge entries,
// whose previous-tx pointer is NOT recoverable from Info alone — see
// core/unpledge_v2.go).
type SyncedTxn struct {
	ID                    string                  `json:"id"`
	Role                  int16                   `json:"role"`
	PreviousTransactionID string                  `json:"previous_transaction_id"`
	Info                  *models.TransactionInfo `json:"info"`
}
