package core

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// renderGzipFixedLengthJSON writes a gzipped JSON response with an explicit
// Content-Length header. The two properties together (Content-Length set +
// Content-Encoding: gzip) keep Go's HTTP server on identity transfer instead
// of chunked, and keep the wire body small enough to sit under the ~4 KB
// Kubo p2p-forward buffer-flush ceiling that intermittently truncates larger
// responses with "unexpected EOF".
//
// Go's http.Transport on the client transparently decompresses when the
// response carries Content-Encoding: gzip and the request did not set
// Accept-Encoding explicitly — which is the case for ensweb's client. The
// caller therefore sees a normal decompressed body without code changes.
//
// On gzip failure the function falls back to uncompressed identity-encoded
// output so the request still returns a valid error response.
func renderGzipFixedLengthJSON(req *ensweb.Request, body interface{}, status int) *ensweb.Result {
	raw, err := json.Marshal(body)
	if err != nil {
		w := req.GetHTTPWritter()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":false,"message":"marshal failed"}`))
		return &ensweb.Result{Status: http.StatusInternalServerError, Done: true}
	}

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, werr := gz.Write(raw); werr != nil {
		return writeIdentityJSON(req, raw, status)
	}
	if cerr := gz.Close(); cerr != nil {
		return writeIdentityJSON(req, raw, status)
	}

	w := req.GetHTTPWritter()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(compressed.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(compressed.Bytes())
	return &ensweb.Result{Status: status, Done: true}
}

// writeIdentityJSON is the gzip-failure fallback. Same fixed-length /
// identity-encoded shape as the old renderFixedLengthJSON.
func writeIdentityJSON(req *ensweb.Request, raw []byte, status int) *ensweb.Result {
	w := req.GetHTTPWritter()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(status)
	_, _ = w.Write(raw)
	return &ensweb.Result{Status: status, Done: true}
}

// Safety constants for the recover-from-fullnode endpoint.
//
// Pagination is cursor-based on (token_id, position). Each response is
// gzipped with an explicit Content-Length, and the handler stops adding
// chain rows once the accumulated raw JSON size approaches recoverMaxRawBytes.
// At a typical 3–5× compression ratio for chain JSON, that yields a
// compressed body of ~2–3 KB — well under the ~4 KB Kubo p2p-forward
// buffer-flush limit that causes the "unexpected EOF" truncation on larger
// responses.
//
// A single row whose raw JSON already exceeds the budget still ships alone
// (one-row page) — the client-side retry handles the rare case where its
// gzipped form also exceeds the wire ceiling.
const (
	// recoverMaxRawBytes is the soft budget on uncompressed JSON body bytes
	// per page. Chosen so the gzipped wire body stays well under 4 KB at
	// typical compression ratios for chain-row JSON.
	recoverMaxRawBytes = 9 * 1024

	// recoverBatchSize bounds the DB rows fetched per page-build so a single
	// request can't pull tens of MB of chain entries into memory.
	recoverBatchSize = 500

	// recoverMaxRequestBodyBytes caps the incoming request body — large
	// KnownTokens maps push this up.
	recoverMaxRequestBodyBytes = 1 * 1024 * 1024
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
// (token_id, position) cursor; the per-response byte budget keeps the
// gzipped wire body under the p2p-forward truncation ceiling.
// See types.RecoverFromFullnodeRequest for the full contract.
func (c *Core) recoverFromFullnodeHandler(req *ensweb.Request) *ensweb.Result {
	c.log.Info("recoverFromFullnodeHandler: HIT")
	if httpReq := req.GetHTTPRequest(); httpReq != nil && httpReq.Body != nil {
		httpReq.Body = http.MaxBytesReader(req.GetHTTPWritter(), httpReq.Body, recoverMaxRequestBodyBytes)
	}

	var recReq types.RecoverFromFullnodeRequest
	if err := c.l.ParseJSON(req, &recReq); err != nil {
		c.log.Warn("recoverFromFullnodeHandler: parse request body failed", "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	c.log.Info("recoverFromFullnodeHandler: parsed",
		"did", recReq.DID,
		"cursor", fmt.Sprintf("(%s,%d)", recReq.LastTokenID, recReq.LastPosition),
		"known_count", len(recReq.KnownTokens))
	if recReq.DID == "" {
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "did is required"}, http.StatusOK)
	}

	// Divergence detection: tokens whose KnownTokens claim doesn't match the
	// fullnode's chain at the claimed position. These get their full chain
	// back (threshold = -1) and are surfaced to the client so it can discard
	// its stale data for them.
	divergent, err := c.w.DetectDivergentRecoveryTokens(c.w.Ctx, recReq.DID, recReq.KnownTokens)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: divergence check failed",
			"did", recReq.DID, "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode divergence check failed"}, http.StatusOK)
	}
	divergentSet := make(map[string]struct{}, len(divergent))
	for _, t := range divergent {
		divergentSet[t] = struct{}{}
	}
	// Build the threshold map the cursor query consumes. Divergent tokens
	// are LEFT OUT (defaults to -1 → full chain). Non-divergent tokens get
	// their claimed position.
	thresholds := make(map[string]int64, len(recReq.KnownTokens))
	for tokenID, tip := range recReq.KnownTokens {
		if _, isDivergent := divergentSet[tokenID]; isDivergent {
			continue
		}
		thresholds[tokenID] = tip.Position
	}

	// Normalize cursor for the first request. An empty LastTokenID with
	// LastPosition=0 is ambiguous (could mean "start fresh" or "after
	// position 0 of a token whose id sorts to '' ") — interpret an empty
	// cursor token as "start fresh" by forcing position to -1.
	cursorTokenID := recReq.LastTokenID
	cursorPosition := recReq.LastPosition
	if cursorTokenID == "" {
		cursorPosition = -1
	}

	chainRows, err := c.w.GetRecoverableChainPageByCursor(c.w.Ctx, recReq.DID, thresholds, cursorTokenID, cursorPosition, recoverBatchSize)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: page fetch failed",
			"did", recReq.DID, "cursor", fmt.Sprintf("(%s,%d)", cursorTokenID, cursorPosition), "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode read failed"}, http.StatusOK)
	}

	if len(chainRows) == 0 {
		result := types.RecoverFromFullnodeResult{
			Tokens:          []types.RecoveredToken{},
			DivergentTokens: divergent,
			HasMore:         false,
		}
		return renderGzipFixedLengthJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
	}

	// Look up the current state for the tokens appearing on this page.
	// One round trip via the existing ListOwnedTokensByDID — then index by
	// token id.
	ownedAll, err := c.w.ListOwnedTokensByDID(c.w.Ctx, recReq.DID)
	if err != nil {
		c.log.Warn("recoverFromFullnodeHandler: load owned-token states failed",
			"did", recReq.DID, "err", err)
		return c.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode read failed"}, http.StatusOK)
	}
	stateByToken := make(map[string]*wallet.RecoverableToken, len(ownedAll))
	for i := range ownedAll {
		stateByToken[ownedAll[i].TokenID] = &ownedAll[i]
	}

	// Group chain rows by token, preserving their (token_id, position) order
	// (the query already sorts that way). Stop emitting once the running raw
	// byte budget would be exceeded — except for the very first row included,
	// which always ships even if it alone blows the budget.
	type tokenAccumulator struct {
		token *wallet.RecoverableToken
		txns  []types.RecoveredTransaction
	}
	accumulators := make(map[string]*tokenAccumulator)
	orderedTokens := make([]string, 0)

	rawBytesUsed := 0
	rowsIncluded := 0
	hasMore := false
	var nextCursorTokenID string
	var nextCursorPosition int64

	for i := range chainRows {
		row := &chainRows[i]

		// Approximate the JSON byte cost of this row without re-marshalling.
		// Info and Signature are already json.RawMessage and dominate the
		// size; the surrounding fields add a small fixed overhead.
		rowCost := len(row.Info) + len(row.Signature) + len(row.TokenID) + len(row.TransactionID) + 256

		if rowsIncluded > 0 && rawBytesUsed+rowCost > recoverMaxRawBytes {
			// Budget exhausted; remaining batch rows wait for the next page.
			hasMore = true
			break
		}

		st, found := stateByToken[row.TokenID]
		if !found {
			// Owned-state vanished between the chain query and the state
			// query (extremely unlikely, but possible under heavy churn).
			// Skip without consuming budget; cursor advances so we don't
			// loop on the same row forever.
			c.log.Warn("recoverFromFullnodeHandler: owned state missing for token on this page; skipping",
				"did", recReq.DID, "tokenID", row.TokenID)
			nextCursorTokenID = row.TokenID
			nextCursorPosition = row.Position
			continue
		}

		acc, ok := accumulators[row.TokenID]
		if !ok {
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
		rowsIncluded++
		rawBytesUsed += rowCost
		nextCursorTokenID = row.TokenID
		nextCursorPosition = row.Position
	}

	// If we consumed the entire DB batch without hitting the byte budget,
	// there may still be more rows beyond the batch. Promise the client a
	// follow-up so it keeps paginating.
	if !hasMore && len(chainRows) == recoverBatchSize {
		hasMore = true
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
		HasMore:         hasMore,
		NextTokenID:     nextCursorTokenID,
		NextPosition:    nextCursorPosition,
	}
	c.log.Info("recoverFromFullnodeHandler: returning",
		"did", recReq.DID,
		"rows_included", rowsIncluded,
		"raw_bytes_used", rawBytesUsed,
		"has_more", hasMore,
		"next_cursor", fmt.Sprintf("(%s,%d)", nextCursorTokenID, nextCursorPosition),
		"tokens_in_response", len(out))
	return renderGzipFixedLengthJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}
