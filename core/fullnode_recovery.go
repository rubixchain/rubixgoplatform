package core

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// renderGzipJSON writes a gzip-compressed JSON response over the libp2p
// HTTP stream. Used by the recovery handler because individual chain
// entries can carry very large transaction info blobs (>200 KB) that
// exceed the underlying libp2p stream's effective throughput window
// when shipped raw. Go's HTTP transport on the client transparently
// decompresses any response carrying `Content-Encoding: gzip`, so no
// matching client change is needed.
func renderGzipJSON(req *ensweb.Request, body interface{}, status int) *ensweb.Result {
	w := req.GetHTTPWritter()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(status)

	gz := gzip.NewWriter(w)
	enc := json.NewEncoder(gz)
	_ = enc.Encode(body)
	_ = gz.Close()

	return &ensweb.Result{Status: status, Done: true}
}

// Page size / safety constants for the recover-from-fullnode endpoint.
//
// Pagination is page-number based, mirroring sync-txn-info-chain. Page size
// is a server constant (clamped from the request to [1, recoverMaxPageSize])
// so TotalPages stays a meaningful index the client can use for gap detection
// across an entire recovery run.
const (
	// recoverDefaultPageSize controls chain entries per page. Chain entries
	// vary widely in `info` size (a tx referencing many tokens / quorums can
	// be 50–100 KB on its own). Aggregating multiple per page risks blowing
	// past libp2p stream buffer limits and the response gets truncated mid-
	// flight (manifests as `unexpected EOF` on the client). We ship one
	// entry per page during testing to stay safely under the threshold; bump
	// up once a size-aware chunking strategy lands.
	recoverDefaultPageSize     = 1
	recoverMaxPageSize         = 1000
	recoverMaxRequestBodyBytes = 1 * 1024 * 1024 // 1 MB to accommodate large known_tokens maps
	recoverMaxOffsetRows       = 100_000_000
)

// registerRecoveryRoute is called from SubscribeTxnSetup when the node is a
// fullnode; the endpoint is meaningless on a non-fullnode (no fullnode_* tables
// to read from).
func (c *Core) registerRecoveryRoute() {
	c.l.AddRoute(setup.APIRecoverFromFullnode, "POST", c.recoverFromFullnodeHandler)
}

// recoverFromFullnodeHandler serves the libp2p endpoint normal nodes call to
// rebuild their wallet from a fullnode. Authentication is provided at the
// libp2p connection layer — no extra signature scheme on the request body.
//
// The handler returns chain entries (transactions + tokenchain rows) for
// every token currently held by DID across RBT / FT / NFT, regardless of
// token_status (Free, Pledged, Burnt, Committed, etc.). Pagination is by
// page_number with per-token (position, tx_id) cursors for incremental
// re-sync. See types.RecoverFromFullnodeRequest for the full contract.
func (c *Core) recoverFromFullnodeHandler(req *ensweb.Request) *ensweb.Result {
	c.log.Info("recoverFromFullnodeHandler: HIT")
	if httpReq := req.GetHTTPRequest(); httpReq != nil && httpReq.Body != nil {
		httpReq.Body = http.MaxBytesReader(req.GetHTTPWritter(), httpReq.Body, recoverMaxRequestBodyBytes)
	}

	var recReq types.RecoverFromFullnodeRequest
	if err := c.l.ParseJSON(req, &recReq); err != nil {
		c.log.Warn("recoverFromFullnodeHandler: parse request body failed", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	c.log.Info("recoverFromFullnodeHandler: parsed",
		"did", recReq.DID,
		"page_number", recReq.PageNumber,
		"page_size", recReq.PageSize,
		"known_count", len(recReq.KnownTokens))
	if recReq.DID == "" {
		c.log.Info("recoverFromFullnodeHandler: empty DID, returning early")
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "did is required"}, http.StatusOK)
	}

	// Clamp page size; default when zero. Must stay constant across a single
	// recovery run for TotalPages to remain stable.
	pageSize := recReq.PageSize
	if pageSize <= 0 {
		pageSize = recoverDefaultPageSize
	}
	if pageSize > recoverMaxPageSize {
		pageSize = recoverMaxPageSize
	}

	// Default to page 1 when the request omits page_number.
	pageNumber := recReq.PageNumber
	if pageNumber <= 0 {
		pageNumber = 1
	}

	// Divergence detection: tokens whose KnownTokens claim doesn't match the
	// fullnode's chain at the claimed position. These get their full chain
	// back (threshold = -1) and are surfaced to the client so it can discard
	// its stale data for them.
	divergent, err := c.w.DetectDivergentRecoveryTokens(c.w.Ctx, recReq.DID, recReq.KnownTokens)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: divergence check failed",
			"did", recReq.DID, "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "fullnode divergence check failed"}, http.StatusOK)
	}
	divergentSet := make(map[string]struct{}, len(divergent))
	for _, t := range divergent {
		divergentSet[t] = struct{}{}
	}
	// Build the threshold map the count/page queries consume. Divergent
	// tokens are LEFT OUT (defaults to -1 → full chain). Non-divergent
	// tokens get their claimed position.
	thresholds := make(map[string]int64, len(recReq.KnownTokens))
	for tokenID, tip := range recReq.KnownTokens {
		if _, isDivergent := divergentSet[tokenID]; isDivergent {
			continue
		}
		thresholds[tokenID] = tip.Position
	}

	c.log.Info("recoverFromFullnodeHandler: about to count chain entries", "did", recReq.DID)
	totalItems, err := c.w.CountRecoverableChainEntries(c.w.Ctx, recReq.DID, thresholds)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: count failed",
			"did", recReq.DID, "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "fullnode count failed"}, http.StatusOK)
	}
	c.log.Info("recoverFromFullnodeHandler: count returned",
		"did", recReq.DID, "total_items", totalItems)
	totalPages := 0
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}

	if totalItems == 0 {
		c.log.Info("recoverFromFullnodeHandler: returning empty (no owned tokens)",
			"did", recReq.DID, "page_number", pageNumber)
		result := types.RecoverFromFullnodeResult{
			Tokens:          []types.RecoveredToken{},
			DivergentTokens: divergent,
			PageNumber:      pageNumber,
			TotalPages:      0,
			PageSize:        pageSize,
			TotalItems:      0,
		}
		return c.l.RenderJSON(req, &model.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
	}

	if pageNumber > totalPages {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: fmt.Sprintf("page_number %d exceeds total_pages %d", pageNumber, totalPages)}, http.StatusOK)
	}

	offset := (pageNumber - 1) * pageSize
	if offset > recoverMaxOffsetRows {
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: fmt.Sprintf("page offset would exceed safety cap (%d rows)", recoverMaxOffsetRows)}, http.StatusOK)
	}

	chainRows, err := c.w.GetRecoverableChainPageByOffset(c.w.Ctx, recReq.DID, thresholds, offset, pageSize)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: page fetch failed",
			"did", recReq.DID, "page_number", pageNumber, "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "fullnode read failed"}, http.StatusOK)
	}

	// Look up the current state for the set of tokens appearing on this page.
	// One round trip via the existing ListOwnedTokensByDID — we then filter
	// to just the tokens we need for this page (which is a small subset of
	// the DID's full holdings, but the alternative is a per-token state
	// query). For now, pull the full owned-token list once and index it.
	ownedAll, err := c.w.ListOwnedTokensByDID(c.w.Ctx, recReq.DID)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: load owned-token states failed",
			"did", recReq.DID, "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "fullnode read failed"}, http.StatusOK)
	}
	stateByToken := make(map[string]*wallet.RecoverableToken, len(ownedAll))
	for i := range ownedAll {
		stateByToken[ownedAll[i].TokenID] = &ownedAll[i]
	}

	// Group chain rows by token, preserving their position-ascending order
	// (the query already sorts by token_id then position).
	type tokenAccumulator struct {
		token *wallet.RecoverableToken
		txns  []types.RecoveredTransaction
	}
	accumulators := make(map[string]*tokenAccumulator)
	orderedTokens := make([]string, 0)
	for i := range chainRows {
		row := &chainRows[i]
		acc, ok := accumulators[row.TokenID]
		if !ok {
			st, found := stateByToken[row.TokenID]
			if !found {
				// Owned-state vanished between the chain query and the state
				// query (extremely unlikely, but possible under heavy churn).
				// Skip rather than fail the whole page.
				c.log.Warn("recoverFromFullnodeHandler: owned state missing for token on this page; skipping",
					"did", recReq.DID, "tokenID", row.TokenID)
				continue
			}
			acc = &tokenAccumulator{token: st}
			accumulators[row.TokenID] = acc
			orderedTokens = append(orderedTokens, row.TokenID)
		}
		acc.txns = append(acc.txns, types.RecoveredTransaction{
			ID:        row.TransactionID,
			Info:      json.RawMessage(row.Info),
			Signature: json.RawMessage(row.Signature),
			ChainEntry: models.TokenChain{
				TokenID:               row.TokenID,
				TransactionID:         row.TransactionID,
				PreviousTransactionID: row.PreviousTransactionID,
				Role:                  row.Role,
				Position:              row.Position,
			},
		})
	}

	out := make([]types.RecoveredToken, 0, len(orderedTokens))
	for _, tokenID := range orderedTokens {
		acc := accumulators[tokenID]
		out = append(out, types.RecoveredToken{
			TokenID:   acc.token.TokenID,
			TokenType: acc.token.TokenType,
			CurrentState: types.RecoveredTokenState{
				DID:            acc.token.DID,
				TokenStatus:    acc.token.TokenStatus,
				TokenValue:     acc.token.TokenValue,
				TokenStateHash: acc.token.TokenStateHash,
				TransactionID:  acc.token.TransactionID,
				LatestPosition: acc.token.LatestPosition,
				LatestRole:     acc.token.LatestRole,
				ParentTokenID:  acc.token.ParentTokenID,
			},
			TxnInfos: acc.txns,
		})
	}

	result := types.RecoverFromFullnodeResult{
		Tokens:          out,
		DivergentTokens: divergent,
		PageNumber:      pageNumber,
		TotalPages:      totalPages,
		PageSize:        pageSize,
		TotalItems:      totalItems,
	}
	// Log response size so we can correlate against libp2p tunnel behavior.
	// Temporary diagnostic — remove once transport is confirmed stable.
	if respBytes, mErr := json.Marshal(result); mErr == nil {
		c.log.Info("recoverFromFullnodeHandler: response size",
			"did", recReq.DID, "bytes_uncompressed", len(respBytes))
	}
	c.log.Info("recoverFromFullnodeHandler: returning",
		"did", recReq.DID,
		"page_number", pageNumber,
		"total_pages", totalPages,
		"total_items", totalItems,
		"tokens_in_response", len(out))
	// Gzip the success-path response — single chain entries can exceed
	// what the libp2p stream ships reliably (>~200 KB consistently EOFs
	// the client). JSON compresses 5–10× here so even a 300 KB raw entry
	// fits comfortably on the wire.
	return renderGzipJSON(req, &model.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}
