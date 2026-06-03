package core

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	mathrand "math/rand"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

const (
	// Canonical source of active fullnodes per network. Fetched on each
	// recovery call so the list stays fresh without a node restart.
	fullnodesListURL = "https://raw.githubusercontent.com/rubixchain/assets/refs/heads/main/fullnodes.json"

	fullnodesListFetchTimeout = 15 * time.Second
	recoveryRequestTimeout    = 90 * time.Second

	// Hard cap on the page loop so a misbehaving fullnode cannot keep the
	// call spinning forever. With default page_size=100 chain entries this
	// lets a recovering wallet pull up to ~500k chain entries per run, well
	// above realistic single-DID holdings.
	recoveryMaxPageIterations = 5000
)

// fullnodeEntry mirrors one element of the JSON list at fullnodesListURL.
type fullnodeEntry struct {
	PeerID string `json:"peer_id"`
	Status string `json:"status"`
}

// fullnodeRegistry is the top-level shape returned by the list URL: a map of
// network name -> list of fullnodes.
type fullnodeRegistry struct {
	Mainnet []fullnodeEntry `json:"mainnet"`
	Testnet []fullnodeEntry `json:"testnet"`
}

// RecoverWalletResult is the summary the HTTP endpoint hands back to the caller.
type RecoverWalletResult struct {
	DID              string `json:"did"`
	FullnodePeerID   string `json:"fullnode_peer_id"`
	TokensSeen       int    `json:"tokens_seen"`
	ChainEntriesPersisted int `json:"chain_entries_persisted"`
	TokensFailed     int    `json:"tokens_failed"`
	PagesFetched     int    `json:"pages_fetched"`
	DivergentTokens  []string `json:"divergent_tokens,omitempty"`
}

// RecoverWalletFromFullnode rebuilds the local wallet state for `did` by
// pulling chain entries for every token DID owns from an Active fullnode in
// the canonical fullnodes.json. Idempotent: re-running it after a successful
// recovery skips entries that are already in sync via the (position, tx_id)
// cursor the client builds from its local DB.
//
// Flow:
//  1. Pick an Active fullnode peer for the local network.
//  2. Build KnownTokens from the local `tokens` table for `did`.
//  3. Loop pages 1..total_pages. For each page, group chain entries by token
//     and persist via PersistRecoveredTokenChainPage (per-token, multi-tx).
//  4. After the run, surface DivergentTokens so the caller can react to any
//     chain divergence the fullnode reported.
func (c *Core) RecoverWalletFromFullnode(ctx context.Context, did string) (*RecoverWalletResult, error) {
	if did == "" {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: did is required")
	}
	if ctx == nil {
		ctx = c.w.Ctx
	}

	peerID, err := c.selectActiveFullnodePeer(ctx)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: pick fullnode peer: %w", err)
	}
	c.log.Info("RecoverWalletFromFullnode: dialing fullnode", "did", did, "fullnode_peer", peerID)

	peer, err := c.connectPeer(peerID)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: connect to fullnode %s: %w", peerID, err)
	}
	defer peer.Close()

	knownTokens, err := c.w.ReadLocalKnownState(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: read local known state: %w", err)
	}

	result := &RecoverWalletResult{DID: did, FullnodePeerID: peerID}

	// Page-number-driven loop. The server tells us total_pages on every
	// response; we loop while page_number <= total_pages. A crash partway
	// through is harmless: ReadLocalKnownState above re-derives the cursor
	// from the local DB on restart, so already-persisted chain entries
	// naturally filter out on the server side.
	pageNumber := 1
	totalPages := 0
	for ; pageNumber <= recoveryMaxPageIterations; pageNumber++ {
		req := &types.RecoverFromFullnodeRequest{
			DID:         did,
			KnownTokens: knownTokens,
			PageNumber:  pageNumber,
		}

		var resp struct {
			Status  bool                            `json:"status"`
			Message string                          `json:"message"`
			Result  types.RecoverFromFullnodeResult `json:"result"`
		}
		if err := c.fetchRecoveryPageWithDiag(peer, req, &resp, pageNumber); err != nil {
			return result, fmt.Errorf("RecoverWalletFromFullnode: send page %d: %w", pageNumber, err)
		}
		if !resp.Status {
			return result, fmt.Errorf("RecoverWalletFromFullnode: fullnode error on page %d: %s", pageNumber, resp.Message)
		}
		result.PagesFetched++
		totalPages = resp.Result.TotalPages

		// Collect divergent tokens from this page; deduped at the end.
		if len(resp.Result.DivergentTokens) > 0 {
			result.DivergentTokens = appendUnique(result.DivergentTokens, resp.Result.DivergentTokens)
		}

		for i := range resp.Result.Tokens {
			t := &resp.Result.Tokens[i]
			if len(t.TxnInfos) == 0 {
				// Empty txn_infos means nothing new for this token on this
				// page. Should be rare with per-token cursors but harmless.
				continue
			}
			if err := c.persistRecoveredTokenPage(ctx, did, t); err != nil {
				c.log.Warn("RecoverWalletFromFullnode: persist token page failed; continuing",
					"did", did, "tokenID", t.TokenID, "tokenType", t.TokenType,
					"status", t.CurrentState.TokenStatus, "txCount", len(t.TxnInfos), "err", err)
				result.TokensFailed++
				continue
			}
			result.TokensSeen++
			result.ChainEntriesPersisted += len(t.TxnInfos)
		}

		if totalPages == 0 || pageNumber >= totalPages {
			return result, nil
		}
	}
	return result, fmt.Errorf("RecoverWalletFromFullnode: page iteration cap (%d) reached without completion (total_pages=%d)", recoveryMaxPageIterations, totalPages)
}

// fetchRecoveryPageWithDiag is a temporary diagnostic replacement for
// peer.SendJSONRequest used only by the recovery loop. It manually builds
// the HTTP request, reads the response body in full, and logs:
//   - HTTP status
//   - Content-Encoding / Content-Length headers
//   - Actual bytes received (compared against Content-Length)
//   - Head + tail snippet of the body on decode failure
//
// Same 3-attempt retry shape as SendJSONRequest. Once the recovery transport
// is stable, this can be reverted to SendJSONRequest.
func (c *Core) fetchRecoveryPageWithDiag(
	peer *ipfsport.Peer,
	body *types.RecoverFromFullnodeRequest,
	out interface{},
	pageNumber int,
) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		httpReq, err := peer.JSONRequest("POST", setup.APIRecoverFromFullnode, body)
		if err != nil {
			c.log.Error("recovery diag: build request failed",
				"page", pageNumber, "attempt", attempt, "err", err)
			lastErr = err
			continue
		}
		// Intentionally NOT setting httpReq.Close = true.
		//
		// Each page used to open a fresh TCP conn + a fresh libp2p stream
		// (via IPFS p2p-forward). After the handler returned, the server
		// would emit Connection: close and tear down the stream immediately.
		// On responses larger than the http server's 4 KB write buffer that
		// teardown raced the remaining flushes through p2p-forward, so the
		// client saw the first ~4 KB and then unexpected EOF — even though
		// the response was fully marshalled with a correct Content-Length.
		// Reusing the connection across pages keeps the libp2p stream alive
		// for the whole recovery run, which removes the race entirely.

		httpResp, err := peer.Do(httpReq, recoveryRequestTimeout)
		if err != nil {
			c.log.Error("recovery diag: transport error",
				"page", pageNumber, "attempt", attempt, "err", err)
			lastErr = err
			time.Sleep(time.Second * time.Duration(attempt))
			continue
		}

		bodyBytes, readErr := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()

		c.log.Info("recovery diag: response received",
			"page", pageNumber,
			"attempt", attempt,
			"http_status", httpResp.StatusCode,
			"content_encoding", httpResp.Header.Get("Content-Encoding"),
			"content_length_header", httpResp.Header.Get("Content-Length"),
			"bytes_actually_read", len(bodyBytes),
			"read_err", readErr)

		if readErr != nil {
			// Truncated read — log a snippet of what we got before the cut.
			snippet := bodyBytes
			if len(snippet) > 256 {
				snippet = snippet[:256]
			}
			c.log.Error("recovery diag: body read truncated",
				"page", pageNumber, "attempt", attempt,
				"bytes_before_truncation", len(bodyBytes),
				"head", string(snippet))
			lastErr = readErr
			time.Sleep(time.Second * time.Duration(attempt))
			continue
		}

		if httpResp.StatusCode != 200 {
			lastErr = fmt.Errorf("http status %d: %s", httpResp.StatusCode, string(bodyBytes))
			c.log.Error("recovery diag: non-200", "page", pageNumber, "err", lastErr)
			return lastErr
		}

		if err := json.Unmarshal(bodyBytes, out); err != nil {
			headSize := 256
			tailSize := 256
			if len(bodyBytes) < headSize {
				headSize = len(bodyBytes)
			}
			head := string(bodyBytes[:headSize])
			var tail string
			if len(bodyBytes) > tailSize {
				tail = string(bodyBytes[len(bodyBytes)-tailSize:])
			} else {
				tail = "<same as head>"
			}
			c.log.Error("recovery diag: JSON decode failed",
				"page", pageNumber, "attempt", attempt,
				"body_size", len(bodyBytes),
				"err", err,
				"head_256", head,
				"tail_256", tail)
			lastErr = err
			time.Sleep(time.Second * time.Duration(attempt))
			continue
		}

		// Success.
		c.log.Info("recovery diag: response decoded ok",
			"page", pageNumber, "attempt", attempt, "body_size", len(bodyBytes))
		return nil
	}

	return fmt.Errorf("recovery diag: all %d attempts failed: %w", maxRetries, lastErr)
}

// persistRecoveredTokenPage maps the wire-level RecoveredToken into the
// arguments of Wallet.PersistRecoveredTokenChainPage and calls it. One call
// per (token, page) — the helper is idempotent, so the same token appearing
// on multiple pages just appends new chain entries on each call.
func (c *Core) persistRecoveredTokenPage(ctx context.Context, did string, t *types.RecoveredToken) error {
	if t == nil {
		return fmt.Errorf("nil recovered token")
	}
	if t.TokenID == "" {
		return fmt.Errorf("recovered token missing token_id")
	}

	// Convert wire chain entries to the wallet's row type.
	chainEntries := make([]wallet.RecoveredChainRow, 0, len(t.TxnInfos))
	for i := range t.TxnInfos {
		txn := &t.TxnInfos[i]
		if txn.ID == "" {
			return fmt.Errorf("recovered txn for token %s missing tx id", t.TokenID)
		}
		chainEntries = append(chainEntries, wallet.RecoveredChainRow{
			TokenID:               t.TokenID,
			TransactionID:         txn.ID,
			Role:                  txn.ChainEntry.Role,
			Position:              txn.ChainEntry.Position,
			PreviousTransactionID: txn.ChainEntry.PreviousTransactionID,
			Info:                  json.RawMessage(txn.Info),
			Signature:             json.RawMessage(txn.Signature),
		})
	}

	state := &models.Token{
		TokenID:        t.TokenID,
		TokenValue:     t.CurrentState.TokenValue,
		TokenStatus:    t.CurrentState.TokenStatus,
		DID:            t.CurrentState.DID,
		TransactionID:  t.CurrentState.TransactionID,
		TokenStateHash: t.CurrentState.TokenStateHash,
		LatestPosition: t.CurrentState.LatestPosition,
		LatestRole:     t.CurrentState.LatestRole,
	}
	if t.CurrentState.ParentTokenID != "" {
		state.ParentTokenID = pgtype.Text{String: t.CurrentState.ParentTokenID, Valid: true}
	}

	return c.w.PersistRecoveredTokenChainPage(ctx, did, t.TokenType, state, chainEntries)
}

// appendUnique appends items from `add` to `dst`, dropping duplicates already
// present. Order is preserved (first occurrence wins). Tiny utility used by
// the orchestration to dedupe divergent-token lists across pages.
func appendUnique(dst, add []string) []string {
	seen := make(map[string]struct{}, len(dst)+len(add))
	for _, s := range dst {
		seen[s] = struct{}{}
	}
	for _, s := range add {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		dst = append(dst, s)
	}
	return dst
}

// selectActiveFullnodePeer fetches the canonical fullnodes list and returns a
// random Active peer for the node's current network. We pick at random rather
// than always the first entry so load is spread across the published fullnodes.
func (c *Core) selectActiveFullnodePeer(ctx context.Context) (string, error) {
	registry, err := fetchFullnodeRegistry(ctx)
	if err != nil {
		return "", err
	}
	var candidates []fullnodeEntry
	switch {
	case c.testnet:
		candidates = registry.Testnet
	case c.localnet:
		// Localnet doesn't have a published fullnode list. Operators are
		// expected to run their own fullnode and use a different recovery
		// path (or extend the registry). Surface a clear error.
		return "", fmt.Errorf("localnet recovery via published fullnodes is not supported; configure a local fullnode")
	default:
		candidates = registry.Mainnet
	}

	active := make([]fullnodeEntry, 0, len(candidates))
	for _, e := range candidates {
		if e.Status == "Active" && e.PeerID != "" {
			active = append(active, e)
		}
	}
	if len(active) == 0 {
		return "", fmt.Errorf("no active fullnode listed for network")
	}
	// crypto/rand for index selection so behavior is unbiased even if the
	// process has not seeded math/rand. Falls back to math/rand if the
	// crypto source fails (extremely unlikely on supported platforms).
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(active))))
	if err != nil {
		c.log.Debug("selectActiveFullnodePeer: crypto/rand.Int failed, falling back to math/rand", "err", err)
		return active[mathrand.Intn(len(active))].PeerID, nil
	}
	return active[idx.Int64()].PeerID, nil
}

// fetchFullnodeRegistry pulls the canonical fullnodes JSON. A short timeout is
// used because a hung fetch should not stall the whole recovery call.
func fetchFullnodeRegistry(ctx context.Context) (*fullnodeRegistry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, fullnodesListFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", fullnodesListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	client := &http.Client{Timeout: fullnodesListFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch fullnodes list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch fullnodes list: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read fullnodes list body: %w", err)
	}
	var reg fullnodeRegistry
	if err := json.Unmarshal(body, &reg); err != nil {
		return nil, fmt.Errorf("parse fullnodes list: %w", err)
	}
	return &reg, nil
}
