package types

import "github.com/rubixchain/rubixgoplatform/types/models"

// SyncTransactionInfoFromFullnodeRequest is the request body for the
// sync-txn-info-chain endpoint.
type SyncTransactionInfoFromFullnodeRequest struct {
	TokenIDs []string `json:"token_ids"`
}

// SyncedTxn is one entry of a token's transaction chain.
type SyncedTxn struct {
	ID                    string                  `json:"id"`
	Role                  int16                   `json:"role"`
	PreviousTransactionID string                  `json:"previous_transaction_id"`
	Info                  *models.TransactionInfo `json:"info"`
}
