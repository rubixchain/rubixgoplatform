package recovery

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

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	// Canonical source of active fullnodes per network. Fetched on each recovery
	// call so the list stays fresh without a node restart.
	fullnodesListURL = "https://raw.githubusercontent.com/rubixchain/assets/refs/heads/main/fullnodes.json"

	fullnodesListFetchTimeout = 15 * time.Second
	recoveryRequestTimeout    = 90 * time.Second

	// Hard cap on the page loop so a misbehaving fullnode cannot keep the call
	// spinning forever. 100k pages is far above any real single-DID holding.
	recoveryMaxPageIterations = 100000

	// Delay between page requests. On the Kubo p2p-forward tunnel, rapid
	// back-to-back requests on the reused connection can drop the previous
	// response's tail flush (seen as "unexpected EOF" on the client). A small
	// delay lets the previous response drain first.
	recoveryInterPageDelay = 50 * time.Millisecond
)

// fullnodeEntry mirrors one element of the JSON list at fullnodesListURL.
type fullnodeEntry struct {
	PeerID string `json:"peer_id"`
	Status string `json:"status"`
}

// fullnodeRegistry is the top-level shape returned by the list URL: a map of
// network name to list of fullnodes.
type fullnodeRegistry struct {
	Mainnet []fullnodeEntry `json:"mainnet"`
	Testnet []fullnodeEntry `json:"testnet"`
}

// WalletSyncResult is the summary of a wallet recovery run. The delta, dry-run,
// and self-test counters stay zero for a plain full recovery.
type WalletSyncResult struct {
	DID                   string `json:"did"`
	FullnodePeerID        string `json:"fullnode_peer_id"`
	Mode                  string `json:"mode,omitempty"`
	DryRun                bool   `json:"dry_run,omitempty"`
	TokensSeen            int    `json:"tokens_seen"`
	TransactionsPersisted int    `json:"transactions_persisted"`
	ChainEntriesPersisted int    `json:"chain_entries_persisted"`
	TokensFailed          int    `json:"tokens_failed"`
	PagesFetched          int    `json:"pages_fetched"`
	TokensPinned          int    `json:"tokens_pinned"`
	TokensPinFailed       int    `json:"tokens_pin_failed"`

	// Selective and delta accounting. TokensSelected is the owned token count
	// after the type/id filters; the in-sync/missing/divergent split is filled
	// only in delta and dry-run mode.
	TokensSelected  int `json:"tokens_selected,omitempty"`
	TokensInSync    int `json:"tokens_in_sync,omitempty"`
	TokensMissing   int `json:"tokens_missing,omitempty"`
	TokensDivergent int `json:"tokens_divergent,omitempty"`

	// Post-recovery self-test outcome, filled only when SelfTest is requested.
	SelfTestOK     int `json:"self_test_ok,omitempty"`
	SelfTestFailed int `json:"self_test_failed,omitempty"`
}

// pinTarget carries the per-token data needed to re-pin its IPFS state-hash
// content after the DB rebuild.
type pinTarget struct {
	stateHash string
	txID      string
	value     float64
}

// ClientDeps are the dependencies the recovering node injects into a wallet sync
// run. Core builds this (signer from SetupDID, peer manager, wallet, network
// flags) and calls RecoverWallet.
type ClientDeps struct {
	PM       *ipfsport.PeerManager
	Store    *Store
	Wallet   *wallet.Wallet // for Pin
	Signer   types.DIDCrypto
	AppName  func(peerID string) string
	Testnet  bool
	Localnet bool
	Log      logger.Logger
}

// connectAndAuthenticate selects an Active fullnode, opens the libp2p
// connection, and obtains a single-use ownership nonce signed once with the DID
// key. The signature is reused on every page. The caller closes the returned
// peer; on any error here the peer is closed before returning.
func connectAndAuthenticate(ctx context.Context, d ClientDeps, did string) (peer *ipfsport.Peer, peerID, nonce, signature string, err error) {
	peerID, err = selectActiveFullnodePeer(ctx, d.Testnet, d.Localnet, d.Log)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("connectAndAuthenticate: pick fullnode peer: %w", err)
	}
	d.Log.Info("connectAndAuthenticate: dialing fullnode", "did", did, "fullnode_peer", peerID)

	peer, err = d.PM.OpenPeerConn(peerID, "", d.AppName(peerID))
	if err != nil {
		return nil, "", "", "", fmt.Errorf("connectAndAuthenticate: connect to fullnode %s: %w", peerID, err)
	}

	nonce, err = fetchRecoveryChallenge(peer, did)
	if err != nil {
		peer.Close()
		return nil, "", "", "", fmt.Errorf("connectAndAuthenticate: obtain recovery nonce: %w", err)
	}
	authSigBytes, err := d.Signer.Sign(recoveryNonceHash(nonce))
	if err != nil {
		peer.Close()
		return nil, "", "", "", fmt.Errorf("connectAndAuthenticate: sign recovery nonce: %w", err)
	}
	return peer, peerID, nonce, util.BytesToBase64(authSigBytes), nil
}

// pullTokens runs the tokens phase and returns every owned token buffered in
// memory with its chain structure. It stops at the tokens/transactions boundary
// without pulling any transaction blob, so callers that only need the token list
// (delta classification, dry-run) pay just this cheap phase.
func pullTokens(ctx context.Context, d ClientDeps, did string, peer *ipfsport.Peer, nonce, signature string) (map[string]*RecoveredToken, []string, int, error) {
	tokensByID := make(map[string]*RecoveredToken)
	ordered := make([]string, 0)
	pages := 0

	var cursor RecoveryCursor
	for iter := 0; iter < recoveryMaxPageIterations; iter++ {
		req := &RecoverFromFullnodeRequest{DID: did, Cursor: cursor}
		var resp struct {
			Status  bool                      `json:"status"`
			Message string                    `json:"message"`
			Result  RecoverFromFullnodeResult `json:"result"`
		}
		if err := fetchRecoveryPage(peer, req, &resp, nonce, signature, d.Log); err != nil {
			return nil, nil, pages, fmt.Errorf("pullTokens: fetch page: %w", err)
		}
		if !resp.Status {
			return nil, nil, pages, fmt.Errorf("pullTokens: fullnode error: %s", resp.Message)
		}
		if resp.Result.Phase != PhaseTokens {
			return nil, nil, pages, fmt.Errorf("pullTokens: unexpected response phase %q", resp.Result.Phase)
		}
		pages++
		for i := range resp.Result.Tokens {
			t := &resp.Result.Tokens[i]
			if buf, ok := tokensByID[t.TokenID]; ok {
				buf.Chain = append(buf.Chain, t.Chain...)
			} else {
				cp := *t
				tokensByID[t.TokenID] = &cp
				ordered = append(ordered, t.TokenID)
			}
		}
		d.Log.Info("pullTokens: page",
			"did", did, "page", pages, "tokens_on_page", len(resp.Result.Tokens),
			"tokens_buffered_total", len(ordered))

		// The tokens phase is done once the server advances the cursor to the tx
		// phase. Return without requesting the first tx page.
		if resp.Result.NextCursor.Phase == PhaseTx {
			return tokensByID, ordered, pages, nil
		}
		cursor = resp.Result.NextCursor

		select {
		case <-ctx.Done():
			return nil, nil, pages, ctx.Err()
		case <-time.After(recoveryInterPageDelay):
		}
	}
	return nil, nil, pages, fmt.Errorf("pullTokens: page iteration cap (%d) reached without completing token phase", recoveryMaxPageIterations)
}

// pullTransactions runs the transactions phase from startCursor, persisting each
// page into the local transactions table as it arrives (each page commits, so
// progress survives a crash). onProgress, when set, is called after each page
// with the cursor to resume from, so a caller can persist a resumable position.
// It returns the number of transactions persisted and pages fetched.
func pullTransactions(ctx context.Context, d ClientDeps, did string, peer *ipfsport.Peer, nonce, signature string, startCursor RecoveryCursor, onProgress func(RecoveryCursor)) (int, int, error) {
	persisted := 0
	pages := 0
	cursor := startCursor
	if cursor.Phase == "" {
		cursor.Phase = PhaseTx
	}
	for iter := 0; iter < recoveryMaxPageIterations; iter++ {
		req := &RecoverFromFullnodeRequest{DID: did, Cursor: cursor}
		var resp struct {
			Status  bool                      `json:"status"`
			Message string                    `json:"message"`
			Result  RecoverFromFullnodeResult `json:"result"`
		}
		if err := fetchRecoveryPage(peer, req, &resp, nonce, signature, d.Log); err != nil {
			return persisted, pages, fmt.Errorf("pullTransactions: fetch page: %w", err)
		}
		if !resp.Status {
			return persisted, pages, fmt.Errorf("pullTransactions: fullnode error: %s", resp.Message)
		}
		if resp.Result.Phase != PhaseTx {
			return persisted, pages, fmt.Errorf("pullTransactions: unexpected response phase %q", resp.Result.Phase)
		}
		pages++
		if len(resp.Result.Transactions) > 0 {
			if err := d.Store.PersistRecoveredTransactions(ctx, did, resp.Result.Transactions); err != nil {
				return persisted, pages, fmt.Errorf("pullTransactions: persist transactions: %w", err)
			}
			persisted += len(resp.Result.Transactions)
		}
		d.Log.Info("pullTransactions: page",
			"did", did, "page", pages, "txns_on_page", len(resp.Result.Transactions),
			"txns_persisted_total", persisted, "has_more", resp.Result.HasMore)

		if !resp.Result.HasMore {
			return persisted, pages, nil
		}
		cursor = resp.Result.NextCursor
		if onProgress != nil {
			onProgress(cursor)
		}

		select {
		case <-ctx.Done():
			return persisted, pages, ctx.Err()
		case <-time.After(recoveryInterPageDelay):
		}
	}
	return persisted, pages, fmt.Errorf("pullTransactions: page iteration cap (%d) reached without completion", recoveryMaxPageIterations)
}

// finalizeTokens writes the local state for each buffered token (chain, tokens
// row, derived accounting) and returns the token state-hash content to re-pin.
// When selected is non-nil only tokens whose id is in it are finalized; a nil
// set finalizes every buffered token. Per-token counters fold into result.
func finalizeTokens(ctx context.Context, d ClientDeps, did string, tokensByID map[string]*RecoveredToken, ordered []string, selected map[string]bool, result *WalletSyncResult) map[string]pinTarget {
	tokensToPin := make(map[string]pinTarget, len(ordered))
	for _, tid := range ordered {
		if selected != nil && !selected[tid] {
			continue
		}
		t := tokensByID[tid]
		if err := d.Store.PersistRecoveredToken(ctx, did, t); err != nil {
			d.Log.Warn("finalizeTokens: persist token failed; continuing",
				"did", did, "tokenID", t.TokenID, "tokenType", t.TokenType,
				"status", t.CurrentState.TokenStatus, "chainLen", len(t.Chain), "err", err)
			result.TokensFailed++
			continue
		}
		result.TokensSeen++
		result.ChainEntriesPersisted += len(t.Chain)
		if t.CurrentState.TokenStateHash != "" {
			tokensToPin[t.TokenID] = pinTarget{
				stateHash: t.CurrentState.TokenStateHash,
				txID:      t.CurrentState.TransactionID,
				value:     t.CurrentState.TokenValue,
			}
		}
	}
	return tokensToPin
}

// pinRecoveredTokenContent restores the IPFS layer for recovered tokens. The DB
// rebuild alone leaves the token-state content unpinned and this node out of the
// provider records, so a quorum can't fetch the chain during a transfer and the
// spend is rejected. For each token this pins the state-hash content (IPFS
// fetches it from the network) and records this node as an Owner provider, which
// makes the token spendable again. Failures are logged and counted, not
// returned, so one unavailable CID does not abort the recovery; a re-run re-pins.
func pinRecoveredTokenContent(ctx context.Context, w *wallet.Wallet, log logger.Logger, did string, targets map[string]pinTarget) (pinned int, failed int) {
	for tokenID, t := range targets {
		if t.stateHash == "" {
			continue
		}
		select {
		case <-ctx.Done():
			log.Warn("pinRecoveredTokenContent: context cancelled; remaining tokens not pinned",
				"did", did, "pinned", pinned, "failed", failed, "remaining", len(targets)-pinned-failed)
			return pinned, failed
		default:
		}
		// Owner role + did so this node is advertised as a provider of the
		// token's state content (mirrors the normal receive/split pin path).
		if _, err := w.Pin(t.stateHash, constants.TokenProviderRole_Owner, did, t.txID, did, did, t.value); err != nil {
			log.Warn("pinRecoveredTokenContent: failed to pin token state content; token may be unspendable until re-pinned",
				"did", did, "tokenID", tokenID, "stateHash", t.stateHash, "err", err)
			failed++
			continue
		}
		pinned++
	}
	if failed > 0 {
		log.Warn("pinRecoveredTokenContent: some recovered tokens could not be pinned (re-run recovery to retry)",
			"did", did, "pinned", pinned, "failed", failed)
	} else {
		log.Info("pinRecoveredTokenContent: pinned all recovered token content", "did", did, "pinned", pinned)
	}
	return pinned, failed
}

// fetchRecoveryChallenge requests a one-time, single-use ownership nonce from the
// fullnode over the libp2p connection. The caller signs this nonce to prove it
// holds the DID's private key; the same signature is then reused on every page.
func fetchRecoveryChallenge(peer *ipfsport.Peer, did string) (string, error) {
	// The challenge needs no custom headers or connection reuse (unlike the page
	// requests), so it uses the shared Peer.SendJSONRequest.
	var resp struct {
		Status  bool                   `json:"status"`
		Message string                 `json:"message"`
		Result  RecoverChallengeResult `json:"result"`
	}
	if err := peer.SendJSONRequest("POST", setup.APIRecoverWalletChallenge, nil, &RecoverChallengeRequest{DID: did}, &resp, false, recoveryRequestTimeout); err != nil {
		return "", fmt.Errorf("fetchRecoveryChallenge: %w", err)
	}
	if !resp.Status {
		return "", fmt.Errorf("fetchRecoveryChallenge: fullnode refused challenge: %s", resp.Message)
	}
	if resp.Result.Nonce == "" {
		return "", fmt.Errorf("fetchRecoveryChallenge: fullnode returned empty nonce")
	}
	return resp.Result.Nonce, nil
}

// fetchRecoveryPage POSTs one page request to the fullnode and decodes the
// response into out. It mirrors Peer.SendJSONRequest (3 attempts, linear
// backoff) with two differences: it carries the ownership proof in the
// X-Rubix-Recovery-* headers, and it does not set httpReq.Close. Keeping the
// connection alive across pages avoids the p2p-forward truncation (see
// recoveryInterPageDelay).
func fetchRecoveryPage(peer *ipfsport.Peer, body *RecoverFromFullnodeRequest, out interface{}, nonce, signature string, log logger.Logger) error {
	const maxRetries = 3
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var httpReq *http.Request
		httpReq, err = peer.JSONRequest("POST", setup.APIRecoverWallet, body)
		if err != nil {
			log.Error("fetchRecoveryPage: build request failed", "attempt", attempt, "err", err)
			continue
		}
		httpReq.Header.Set(headerRecoveryNonce, nonce)
		httpReq.Header.Set(headerRecoverySignature, signature)

		var httpResp *http.Response
		httpResp, err = peer.Do(httpReq, recoveryRequestTimeout)
		if err != nil {
			log.Error("fetchRecoveryPage: transport error", "attempt", attempt, "err", err)
			time.Sleep(time.Second * time.Duration(attempt))
			continue
		}
		if httpResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			return fmt.Errorf("fetchRecoveryPage: status=%d body=%s", httpResp.StatusCode, string(bodyBytes))
		}
		err = jsonutil.DecodeJSONFromReader(httpResp.Body, out)
		httpResp.Body.Close()
		if err != nil {
			log.Error("fetchRecoveryPage: decode response failed", "attempt", attempt, "err", err)
			time.Sleep(time.Second * time.Duration(attempt))
			continue
		}
		return nil
	}
	return fmt.Errorf("fetchRecoveryPage: all %d attempts failed: %w", maxRetries, err)
}

// selectActiveFullnodePeer fetches the canonical fullnodes list and returns a
// random Active peer for the node's current network. We pick at random rather
// than always the first entry so load is spread across the published fullnodes.
func selectActiveFullnodePeer(ctx context.Context, testnet, localnet bool, log logger.Logger) (string, error) {
	registry, err := fetchFullnodeRegistry(ctx)
	if err != nil {
		return "", err
	}
	var candidates []fullnodeEntry
	switch {
	case testnet:
		candidates = registry.Testnet
	case localnet:
		// Localnet doesn't have a published fullnode list. Operators are expected
		// to run their own fullnode and use a different recovery path (or extend
		// the registry). Surface a clear error.
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
	// crypto/rand for index selection so behavior is unbiased even if the process
	// has not seeded math/rand. Falls back to math/rand if the crypto source fails
	// (extremely unlikely on supported platforms).
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(active))))
	if err != nil {
		log.Debug("selectActiveFullnodePeer: crypto/rand.Int failed, falling back to math/rand", "err", err)
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
