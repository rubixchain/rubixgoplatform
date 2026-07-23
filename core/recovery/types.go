package recovery

import (
	"encoding/json"
)

// RecoverFromFullnodeRequest is the body of a recover-from-fullnode call. The
// caller sends only its DID; ownership is proven by a signed nonce in the
// X-Rubix-Recovery-* headers, not this body. Recovery is always a full pull.
// Pagination is a two-phase, size-limited cursor (see RecoveryCursor); the
// first request leaves Cursor zero-valued.
type RecoverFromFullnodeRequest struct {
	DID    string         `json:"did"`
	Cursor RecoveryCursor `json:"cursor"`
}

// RecoveryCursor is the server-defined pagination cursor. The client echoes it
// back unchanged on the next request and does not interpret it. Recovery streams
// the tokens phase first (each owned token plus its chain structure, keyed on
// token_id and position), then the transactions phase (the referenced
// transaction blobs, keyed on transaction id). The server sets Phase; an empty
// Phase is the start of the tokens phase.
type RecoveryCursor struct {
	Phase        string `json:"phase,omitempty"`
	LastTokenID  string `json:"last_token_id,omitempty"`
	LastPosition int64  `json:"last_position,omitempty"`
	LastTxID     string `json:"last_tx_id,omitempty"`
}

// Recovery pagination phases carried in RecoveryCursor.Phase. An empty phase is
// treated as PhaseTokens.
const (
	PhaseTokens = "tokens"
	PhaseTx     = "tx"
)

// RecoverChallengeRequest asks the fullnode to mint a one-time nonce the caller
// signs to prove ownership of DID before recovery.
type RecoverChallengeRequest struct {
	DID string `json:"did"`
}

// RecoverChallengeResult carries the minted nonce back to the caller. The nonce
// is single-use and bound to the requested DID for one recovery session.
type RecoverChallengeResult struct {
	Nonce string `json:"nonce"`
}

// RecoverFromFullnodeResult is the BasicResponse.Result payload for one page. A
// page carries either tokens (Phase=tokens) or transaction blobs (Phase=tx), so
// a transaction shared by many tokens is sent once for the whole run. HasMore
// and NextCursor drive the client loop.
type RecoverFromFullnodeResult struct {
	Phase        string                 `json:"phase"`
	Tokens       []RecoveredToken       `json:"tokens,omitempty"`
	Transactions []RecoveredTransaction `json:"transactions,omitempty"`

	HasMore    bool           `json:"has_more"`
	NextCursor RecoveryCursor `json:"next_cursor"`
}

// RecoveredToken is one token in the tokens phase: its current state plus its
// chain structure (transaction ids in position order, without the blobs). A
// token whose chain spans pages repeats with more Chain rows; CurrentState is
// the same each time and the client applies it once.
type RecoveredToken struct {
	TokenID      string              `json:"token_id"`
	TokenType    string              `json:"token_type"` // "rbt" | "ft" | "nft"
	CurrentState RecoveredTokenState `json:"current_state"`
	Chain        []ChainRef          `json:"chain"`
}

// ChainRef is one entry in a token's chain: a transaction id plus the row
// metadata needed to write the local tokenchain row. The client orders a token's
// chain by Position and resolves TxID against the transaction blobs.
type ChainRef struct {
	TxID     string `json:"tx_id"`
	Position int64  `json:"position"`
	PrevTxID string `json:"prev_tx_id,omitempty"`
	Role     int16  `json:"role"`
}

// RecoveredTransaction is one transaction blob from the transactions phase.
// Info and Signature are passed through as raw JSON. Each transaction is sent
// once per run regardless of how many tokens reference it.
type RecoveredTransaction struct {
	ID        string          `json:"id"`
	Info      json.RawMessage `json:"info"`
	Signature json.RawMessage `json:"signature"`
}

// RecoveredTokenState mirrors the token state columns from the fullnode state
// tables. ParentTokenID is only set for RBT.
type RecoveredTokenState struct {
	DID            string  `json:"did"`
	TokenStatus    int16   `json:"token_status"`
	TokenValue     float64 `json:"token_value"`
	TokenStateHash string  `json:"token_state_hash"`
	TransactionID  string  `json:"transaction_id"`
	LatestPosition int64   `json:"latest_position"`
	LatestRole     int16   `json:"latest_role"`
	ParentTokenID  string  `json:"parent_token_id,omitempty"`
}
