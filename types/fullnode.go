package types

import "encoding/json"

// SyncTransactionInfoFromFullnodeRequest is the body of a sync-txn-info-chain
// call. The caller (typically an explorer) names the tokens it wants to sync
// and which page of chain entries to fetch.
//
// KnownPositions lets the caller say "for token X, I already have entries up
// to position P, and my tx at P is T". The server uses this to skip entries
// the caller already has, and to detect chain divergence (if the server's tx
// at P differs from T, the token is divergent — see DivergentTokens in the
// result).
//
// PageNumber is 1-indexed. If omitted, the server returns page 1. Page size
// is a server constant clamped from PageSize (0 -> default) so total_pages
// stays a stable index across the run.
type SyncTransactionInfoFromFullnodeRequest struct {
	TokenIDs       []string                 `json:"token_ids"`
	KnownPositions map[string]TokenChainTip `json:"known_positions,omitempty"`
	PageNumber     int                      `json:"page_number,omitempty"`
	PageSize       int                      `json:"page_size,omitempty"`
}

// TokenChainTip is the (position, tx_id) pair the caller uses to say "I have
// this token's chain up to here". The server checks BOTH fields — a position
// match alone could miss a chain that diverged at that position.
type TokenChainTip struct {
	Position      int64  `json:"position"`
	TransactionID string `json:"transaction_id"`
}

// SyncTransactionInfoFromFullnodeResult is the response body.
//
// Data groups this page's chain entries by token_id, with entries inside each
// list ordered by position ascending.
//
// DivergentTokens lists tokens whose KnownPositions claim didn't match the
// server's chain at that position. The server sends their full chain (from
// position 0) and the caller should treat any prior local data for those
// tokens as stale and replace it.
//
// PageNumber, TotalPages, PageSize and TotalItems together let the caller
// detect gaps after a run (e.g. "I have pages 1, 2, 4 of 5 — page 3 is
// missing") and re-fetch a specific page by its number. Loop while
// PageNumber < TotalPages.
type SyncTransactionInfoFromFullnodeResult struct {
	Data            map[string][]SyncedTxn `json:"data"`
	DivergentTokens []string               `json:"divergent_tokens,omitempty"`
	PageNumber      int                    `json:"page_number"`
	TotalPages      int                    `json:"total_pages"`
	PageSize        int                    `json:"page_size"`
	TotalItems      int                    `json:"total_items"`
}

// SyncedTxn is one entry on a token's chain.
//
// Info is the stored transaction-info JSON, passed through as raw bytes (no
// unmarshal+remarshal on the server). Position is the entry's chain position
// on the fullnode — the caller uses it to update KnownPositions for the next
// request.
//
// NOTE (replayed-split check, Option A — DISABLED): SyncedTxn intentionally does
// NOT carry the transaction Signature. The replayed-split verifier
// (ValidateSplitParentsAgainstFullnode) consumes this endpoint but only needs
// each ancestor's burn-tx ID, which it reads straight from these entries (id +
// role) WITHOUT persisting the chain — so no signature is required. If a future
// caller needs to APPLY the synced chain into a local wallet (whose transactions
// table has signature NOT NULL), Option A would add a `Signature json.RawMessage
// json:"signature"` field here, mirroring the recovery path's
// RecoveredTransaction.Signature, and populate it in the sync-serve query +
// handler (see core/wallet/fullnode.go GetFullNodeSyncedChainPageByOffset and
// core/sync.go syncTokensFromFullnode's commented Option-A block).
type SyncedTxn struct {
	ID                    string          `json:"id"`
	Role                  int16           `json:"role"`
	Position              int64           `json:"position"`
	PreviousTransactionID string          `json:"previous_transaction_id"`
	Info                  json.RawMessage `json:"info"`
}
