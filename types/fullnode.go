package types

import "encoding/json"

// SyncTransactionInfoFromFullnodeRequest is the request body for the
// sync-txn-info-chain endpoint. Offset paginates the per-token chain:
// for each requested token the server returns entries [offset, offset+K),
// where K is chosen server-side so the total marshalled response stays
// under a size budget (~2 MB).
type SyncTransactionInfoFromFullnodeRequest struct {
	TokenIDs []string `json:"token_ids"`
	Offset   int      `json:"offset,omitempty"`
}

// SyncTransactionInfoFromFullnodeResult is the BasicResponse.Result payload.
// AdvancedBy is the K the server applied — the explorer must use it to
// compute the next request's offset (offset += AdvancedBy). HasMore is true
// if any requested token has more entries past offset+AdvancedBy.
type SyncTransactionInfoFromFullnodeResult struct {
	Data       map[string][]SyncedTxn `json:"data"`
	HasMore    bool                   `json:"has_more"`
	AdvancedBy int                    `json:"advanced_by"`
}

// SyncedTxn is one entry of a token's transaction chain.
//
// Info is carried as json.RawMessage so the server can stream the stored
// transaction-info bytes through without an unmarshal+remarshal round trip.
// The wire format is unchanged — clients still see a JSON object that
// matches TransactionInfo.
type SyncedTxn struct {
	ID                    string          `json:"id"`
	Role                  int16           `json:"role"`
	PreviousTransactionID string          `json:"previous_transaction_id"`
	Info                  json.RawMessage `json:"info"`
}
