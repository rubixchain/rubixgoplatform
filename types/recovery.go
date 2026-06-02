package types

import (
	"encoding/json"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// RecoverFromFullnodeRequest is the body of a recover-from-fullnode call.
//
// The caller is a normal node whose local DB was lost (cold restore) or
// partially out of sync (warm restore). It identifies itself with `DID`.
// Authentication is provided by the libp2p connection identity layer — no
// extra signature scheme on this body.
//
// `KnownTokens` is a per-token "I already have up to here" filter. Each entry
// carries position + tx_id so the server can detect chain divergence (the
// position the client claims must match what the server has at that
// position). A divergent token is reported back in Result.DivergentTokens
// and its chain is shipped from position 0.
//
// Pagination is by absolute page number — same model as sync-txn-info-chain.
// Pagination unit is one chain entry, NOT one token. A single token's full
// chain may span multiple pages, and a single page may carry entries from
// several tokens.
//
// `PageSize` is optional; the server clamps it to [1, MaxPageSize] and falls
// back to DefaultPageSize when zero. PageSize MUST stay constant across a
// single recovery run for TotalPages to remain stable.
type RecoverFromFullnodeRequest struct {
	DID         string                   `json:"did"`
	KnownTokens map[string]TokenChainTip `json:"known_tokens,omitempty"`
	PageNumber  int                      `json:"page_number,omitempty"`
	PageSize    int                      `json:"page_size,omitempty"`
}

// RecoverFromFullnodeResult is the BasicResponse.Result payload.
//
// `Tokens` carries this page's chain entries grouped by token. Multiple
// pages may carry entries for the same token (when its chain is long).
//
// `DivergentTokens` lists tokens whose KnownTokens claim did not match the
// fullnode's chain at the claimed position. Server treats them as
// "no known state" and ships their chain from position 0; client should
// discard any local data for those tokens and replace with what arrives.
//
// `PageNumber`, `TotalPages`, `PageSize`, `TotalItems` together let the
// client detect gaps after a recovery run and re-fetch any specific missing
// page by its number. TotalItems is the total chain entries the server has
// queued to send for this DID, given the KnownTokens filter.
type RecoverFromFullnodeResult struct {
	Tokens          []RecoveredToken `json:"tokens"`
	DivergentTokens []string         `json:"divergent_tokens,omitempty"`
	PageNumber      int              `json:"page_number"`
	TotalPages      int              `json:"total_pages"`
	PageSize        int              `json:"page_size"`
	TotalItems      int              `json:"total_items"`
}

// RecoveredToken is one token entry on a recovery page.
//
// `TxnInfos` carries the chain entries for this token included on THIS page.
// A token's full chain may span multiple pages — the client appends entries
// from later pages onto its local chain in order. The server emits entries
// in chronological (position-ascending) order.
//
// `CurrentState` is the fullnode's authoritative current view: status, value,
// latest_position, latest_role, etc. It is the same across all pages that
// carry entries for this token; the client overwrites the local token state
// with this on every page (idempotent).
type RecoveredToken struct {
	TokenID      string                 `json:"token_id"`
	TokenType    string                 `json:"token_type"` // "rbt" | "ft" | "nft"
	CurrentState RecoveredTokenState    `json:"current_state"`
	TxnInfos     []RecoveredTransaction `json:"txn_infos"`
}

// RecoveredTransaction carries the bytes the client must INSERT into its
// `transactions` table, plus the chain-row metadata for the corresponding
// `tokenchain` entry. Info / Signature are passed through as raw JSON to
// avoid an unmarshal+remarshal cycle on the server.
//
// ChainEntry preserves the global position and previous_transaction_id from
// the fullnode — the client writes the chain row verbatim with those values.
// This is what enables multi-tx-per-token recovery: positions stay aligned
// with the fullnode's chain, future operations work off the highest position.
type RecoveredTransaction struct {
	ID         string            `json:"id"`
	Info       json.RawMessage   `json:"info"`
	Signature  json.RawMessage   `json:"signature"`
	ChainEntry models.TokenChain `json:"chain_entry"`
}

// RecoveredTokenState mirrors the relevant columns of the fullnode token state
// tables. ParentTokenID is only meaningful for RBT and is omitted otherwise.
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
