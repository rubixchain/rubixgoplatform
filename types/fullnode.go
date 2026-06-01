package types

import "github.com/rubixchain/rubixgoplatform/types/models"

// SyncTransactionInfoFromFullnodeRequest is the request body for the
// sync-token-chain endpoint, served over both HTTP (server/fullnode.go) and
// libp2p (core/fullnode.go).
type SyncTransactionInfoFromFullnodeRequest struct {
	TokenIDs []string `json:"token_ids"`
}

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
