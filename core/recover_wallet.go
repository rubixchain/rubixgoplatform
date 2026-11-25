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
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/fullnode"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	// Canonical source of active fullnodes per network. Fetched on each
	// recovery call so the list stays fresh without a node restart.
	fullnodesListURL = "https://raw.githubusercontent.com/rubixchain/assets/refs/heads/main/fullnodes.json"

	fullnodesListFetchTimeout = 15 * time.Second
	recoveryRequestTimeout    = 90 * time.Second

	// Hard cap on the page loop so a misbehaving fullnode cannot keep the
	// call spinning forever. With byte-bounded pages averaging a few chain
	// entries each, 100k iterations covers ~500k entries per run — well
	// above realistic single-DID holdings.
	recoveryMaxPageIterations = 100000

	// Pacing between page requests. The Kubo p2p-forward tunnel exhibits a
	// race where rapid back-to-back requests on the reused connection can
	// see the previous response's tail flush dropped (manifests as
	// "unexpected EOF" on the client). A small delay between requests lets
	// the previous response fully drain before the next request triggers
	// any stream state change.
	recoveryInterPageDelay = 50 * time.Millisecond
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
	DID                   string   `json:"did"`
	FullnodePeerID        string   `json:"fullnode_peer_id"`
	TokensSeen            int      `json:"tokens_seen"`
	ChainEntriesPersisted int      `json:"chain_entries_persisted"`
	TokensFailed          int      `json:"tokens_failed"`
	PagesFetched          int      `json:"pages_fetched"`
	TokensPinned          int      `json:"tokens_pinned"`
	TokensPinFailed       int      `json:"tokens_pin_failed"`
	DivergentTokens       []string `json:"divergent_tokens,omitempty"`
}

// recoveredPinTarget carries the per-token data needed to (re)pin its IPFS
// state-hash content after the DB rebuild.
type recoveredPinTarget struct {
	stateHash string
	txID      string
	value     float64
}

// RecoverWalletFromFullnodeAsync runs RecoverWalletFromFullnode in the
// background and delivers the result (or error) over the request's signature
// channel. Recovery now signs an ownership challenge, so the API handler runs
// it asynchronously through the same OutChan/InChan signature mechanism as
// register/transfer: the node emits "Signature needed" (or "Password needed"),
// the caller answers via /rubix/v1/signature, then the final recovery summary
// is returned on the same channel.
func (c *Core) RecoverWalletFromFullnodeAsync(reqID string, did string) {
	result, err := c.RecoverWalletFromFullnode(reqID, c.w.Ctx, did)
	// Result stays nil: the CLI signature loop treats a non-nil Result as another
	// signature request, which made a successful recovery report a failure.
	br := &models.BasicResponse{
		Status:  true,
		Message: "wallet recovery completed",
	}
	if result != nil {
		br.Message = fmt.Sprintf("wallet recovery completed: %d tokens, %d chain entries persisted, %d pinned, %d divergent",
			result.TokensSeen, result.ChainEntriesPersisted, result.TokensPinned, len(result.DivergentTokens))
	}
	if err != nil {
		br.Status = false
		br.Message = "recovery failed: " + err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("RecoverWalletFromFullnodeAsync: failed to get did channel", "did", did)
		return
	}
	dc.OutChan <- br
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
func (c *Core) RecoverWalletFromFullnode(reqID string, ctx context.Context, did string) (*RecoverWalletResult, error) {
	if did == "" {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: did is required")
	}
	if ctx == nil {
		ctx = c.w.Ctx
	}

	// Ownership proof uses a server-issued single-use nonce. Get a signer handle
	// now; the nonce is fetched after the peer connection opens and signed ONCE,
	// then reused across every page (the possibly-external signer is invoked
	// exactly once per recovery — not per page).
	signer, err := c.SetupDID(reqID, did)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: setup signer for %s: %w", did, err)
	}

	peerID, err := c.selectActiveFullnodePeer(ctx)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: pick fullnode peer: %w", err)
	}
	c.log.Info("RecoverWalletFromFullnode: dialing fullnode", "did", did, "fullnode_peer", peerID)

	peer, err := c.pm.OpenPeerConn(peerID, "", c.getCoreAppName(peerID))
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: connect to fullnode %s: %w", peerID, err)
	}
	defer peer.Close()

	// Obtain a single-use ownership nonce from the fullnode, then sign it ONCE.
	// The signature (over the nonce) is reused on every page; the fullnode
	// verifies it on the first page and keeps the nonce live for the rest of the
	// recovery (no time limit), evicting it on completion.
	nonce, err := c.fetchRecoveryChallenge(peer, did)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: obtain recovery nonce: %w", err)
	}
	authSigBytes, err := signer.Sign(fullnode.RecoveryNonceHash(nonce))
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: sign recovery nonce: %w", err)
	}
	authSignature := util.BytesToBase64(authSigBytes)

	knownTokens, err := c.w.ReadLocalKnownState(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("RecoverWalletFromFullnode: read local known state: %w", err)
	}

	result := &RecoverWalletResult{DID: did, FullnodePeerID: peerID}

	// Tokens whose IPFS state-hash content must be (re)pinned after the DB
	// rebuild so quorums can fetch/verify them on a future spend. Deduped by
	// token id across pages; later pages overwrite with the latest state.
	tokensToPin := make(map[string]recoveredPinTarget)

	// Cursor-driven loop. The server returns HasMore + NextTokenID /
	// NextPosition on every page; we echo those back on the next request.
	// A crash partway through is harmless: ReadLocalKnownState above
	// re-derives the per-token threshold from the local DB on restart, so
	// already-persisted chain entries naturally filter out server-side.
	var lastTokenID string
	var lastPosition int64
	for iter := 0; iter < recoveryMaxPageIterations; iter++ {
		req := &types.RecoverFromFullnodeRequest{
			DID:          did,
			KnownTokens:  knownTokens,
			LastTokenID:  lastTokenID,
			LastPosition: lastPosition,
		}

		var resp struct {
			Status  bool                            `json:"status"`
			Message string                          `json:"message"`
			Result  types.RecoverFromFullnodeResult `json:"result"`
		}
		if err := c.fetchRecoveryPageWithDiag(peer, req, &resp, iter+1, nonce, authSignature); err != nil {
			return result, fmt.Errorf("RecoverWalletFromFullnode: fetch after cursor (%s,%d): %w", lastTokenID, lastPosition, err)
		}
		if !resp.Status {
			return result, fmt.Errorf("RecoverWalletFromFullnode: fullnode error after cursor (%s,%d): %s", lastTokenID, lastPosition, resp.Message)
		}
		result.PagesFetched++

		// Collect divergent tokens from this page; deduped at the end.
		if len(resp.Result.DivergentTokens) > 0 {
			result.DivergentTokens = appendUnique(result.DivergentTokens, resp.Result.DivergentTokens)
		}

		pageEntries := 0
		for i := range resp.Result.Tokens {
			t := &resp.Result.Tokens[i]
			if len(t.TxnInfos) == 0 {
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
			pageEntries += len(t.TxnInfos)

			// Queue this token's current state-hash content for re-pinning
			// after the DB rebuild completes.
			if t.CurrentState.TokenStateHash != "" {
				tokensToPin[t.TokenID] = recoveredPinTarget{
					stateHash: t.CurrentState.TokenStateHash,
					txID:      t.CurrentState.TransactionID,
					value:     t.CurrentState.TokenValue,
				}
			}
		}

		c.log.Info("RecoverWalletFromFullnode: page progress",
			"did", did,
			"page", iter+1,
			"page_entries", pageEntries,
			"tokens_on_page", len(resp.Result.Tokens),
			"entries_persisted_total", result.ChainEntriesPersisted,
			"tokens_seen_total", result.TokensSeen,
			"has_more", resp.Result.HasMore,
			"next_cursor", fmt.Sprintf("(%s,%d)", resp.Result.NextTokenID, resp.Result.NextPosition))

		if !resp.Result.HasMore {
			// DB rebuild done — now restore the IPFS layer so recovered tokens
			// are actually spendable: pin each token's state-hash content
			// (fetched from the network) and record this node as an Owner
			// provider. Without this, quorums can't fetch/verify the token chain
			// during a transfer and the spend is rejected.
			result.TokensPinned, result.TokensPinFailed = c.pinRecoveredTokenContent(ctx, did, tokensToPin)
			c.log.Info("RecoverWalletFromFullnode: recovery complete",
				"did", did,
				"pages_fetched", result.PagesFetched,
				"entries_persisted_total", result.ChainEntriesPersisted,
				"tokens_seen_total", result.TokensSeen,
				"tokens_failed", result.TokensFailed,
				"tokens_pinned", result.TokensPinned,
				"tokens_pin_failed", result.TokensPinFailed,
				"divergent_tokens", len(result.DivergentTokens))
			return result, nil
		}
		lastTokenID = resp.Result.NextTokenID
		lastPosition = resp.Result.NextPosition

		// Pace between page requests to reduce p2p-forward stream
		// contention on the reused libp2p connection.
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(recoveryInterPageDelay):
		}
	}
	return result, fmt.Errorf("RecoverWalletFromFullnode: page iteration cap (%d) reached without completion (last cursor (%s,%d))", recoveryMaxPageIterations, lastTokenID, lastPosition)
}

// pinRecoveredTokenContent restores the IPFS layer for recovered tokens. The
// DB rebuild alone leaves token-state content un-pinned and this node absent
// from the provider records, so quorums can't fetch/verify a token's chain
// during a transfer and the spend is rejected ("token ... still not found
// locally after sync"). For each recovered token this pins the state-hash
// content (IPFS fetches it from the network) and records this node as an Owner
// provider — the step that makes a recovered token actually spendable.
//
// Best-effort by design: a single unavailable/timed-out CID must not abort the
// whole recovery, so failures are logged and counted (TokensPinFailed) rather
// than returned. Pin is idempotent, so re-running recovery safely re-pins.
func (c *Core) pinRecoveredTokenContent(ctx context.Context, did string, targets map[string]recoveredPinTarget) (pinned int, failed int) {
	for tokenID, t := range targets {
		if t.stateHash == "" {
			continue
		}
		select {
		case <-ctx.Done():
			c.log.Warn("pinRecoveredTokenContent: context cancelled; remaining tokens not pinned",
				"did", did, "pinned", pinned, "failed", failed, "remaining", len(targets)-pinned-failed)
			return pinned, failed
		default:
		}
		// Owner role + did so this node is advertised as a provider of the
		// token's state content (mirrors the normal receive/split pin path).
		if _, err := c.w.Pin(t.stateHash, constants.TokenProviderRole_Owner, did, t.txID, did, did, t.value); err != nil {
			c.log.Warn("pinRecoveredTokenContent: failed to pin token state content; token is in the DB but may be unspendable until re-pinned",
				"did", did, "tokenID", tokenID, "stateHash", t.stateHash, "err", err)
			failed++
			continue
		}
		pinned++
	}
	if failed > 0 {
		c.log.Warn("pinRecoveredTokenContent: some recovered tokens could not be pinned (re-run recovery to retry)",
			"did", did, "pinned", pinned, "failed", failed)
	} else {
		c.log.Info("pinRecoveredTokenContent: pinned all recovered token content", "did", did, "pinned", pinned)
	}
	return pinned, failed
}

// fetchRecoveryChallenge requests a one-time, single-use ownership nonce from
// the fullnode over the libp2p connection. The caller signs this nonce to prove
// it holds the DID's private key; the same signature is then reused on every
// recovery page.
func (c *Core) fetchRecoveryChallenge(peer *ipfsport.Peer, did string) (string, error) {
	httpReq, err := peer.JSONRequest("POST", setup.APIRecoverChallenge, &types.RecoverChallengeRequest{DID: did})
	if err != nil {
		return "", fmt.Errorf("build challenge request: %w", err)
	}
	httpResp, err := peer.Do(httpReq, recoveryRequestTimeout)
	if err != nil {
		return "", fmt.Errorf("challenge transport: %w", err)
	}
	bodyBytes, readErr := io.ReadAll(httpResp.Body)
	_ = httpResp.Body.Close()
	if readErr != nil {
		return "", fmt.Errorf("read challenge response: %w", readErr)
	}
	var resp struct {
		Status  bool                         `json:"status"`
		Message string                       `json:"message"`
		Result  types.RecoverChallengeResult `json:"result"`
	}
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return "", fmt.Errorf("decode challenge response: %w", err)
	}
	if !resp.Status {
		return "", fmt.Errorf("fullnode refused challenge: %s", resp.Message)
	}
	if resp.Result.Nonce == "" {
		return "", fmt.Errorf("fullnode returned empty nonce")
	}
	return resp.Result.Nonce, nil
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
	nonce string,
	signature string,
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
		// Ownership proof travels in headers (not the body) — see
		// fullnode.HeaderRecoveryNonce / fullnode.HeaderRecoverySignature.
		httpReq.Header.Set(fullnode.HeaderRecoveryNonce, nonce)
		httpReq.Header.Set(fullnode.HeaderRecoverySignature, signature)
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
