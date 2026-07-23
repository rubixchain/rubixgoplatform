package recovery

import (
	"context"
	"fmt"
	"strings"
)

// Recovery modes for an advanced recovery run.
const (
	// RecoverModeFull rebuilds every token that passes the selective filters.
	RecoverModeFull = "full"
	// RecoverModeDelta rebuilds only the tokens that are missing locally or whose
	// local state differs from the fullnode copy.
	RecoverModeDelta = "delta"
	// RecoverModeDryRun classifies the owned tokens against local state and
	// reports what a recovery would change, writing nothing.
	RecoverModeDryRun = "dryrun"
)

// RecoverOptions controls one advanced recovery run. The zero value is a full
// recovery of every token the DID owns.
type RecoverOptions struct {
	// Mode is full, delta, or dryrun. Empty is treated as full.
	Mode string
	// TokenTypes filters the owned tokens by type (rbt, ft, nft). Empty means all
	// types.
	TokenTypes []string
	// TokenIDs filters the owned tokens to this explicit set. Empty means no id
	// filter.
	TokenIDs []string
	// SelfTest verifies each recovered token was rebuilt coherently after the
	// pull.
	SelfTest bool
}

// RecoverWallet rebuilds the local wallet for `did` with the given options. It
// reuses the shared recovery steps (connect, pull tokens, pull transactions,
// finalize, pin) and adds selective filtering, delta and dry-run classification,
// resumable progress, and an optional self-test. A full run with no options
// rebuilds the whole wallet. Every write uses ON CONFLICT, so it is safe to
// re-run.
func RecoverWallet(ctx context.Context, d ClientDeps, did string, opts RecoverOptions) (*WalletSyncResult, error) {
	if did == "" {
		return nil, fmt.Errorf("RecoverWallet: did is required")
	}
	if ctx == nil {
		ctx = d.Wallet.Ctx
	}
	mode := opts.Mode
	if mode == "" {
		mode = RecoverModeFull
	}
	if mode != RecoverModeFull && mode != RecoverModeDelta && mode != RecoverModeDryRun {
		return nil, fmt.Errorf("RecoverWallet: unknown mode %q", mode)
	}

	peer, peerID, nonce, signature, err := connectAndAuthenticate(ctx, d, did)
	if err != nil {
		return nil, err
	}
	defer peer.Close()

	result := &WalletSyncResult{DID: did, FullnodePeerID: peerID, Mode: mode, DryRun: mode == RecoverModeDryRun}

	tokensByID, ordered, tokenPages, err := pullTokens(ctx, d, did, peer, nonce, signature)
	if err != nil {
		return result, err
	}
	result.PagesFetched += tokenPages

	candidates := filterCandidates(ordered, tokensByID, opts)
	result.TokensSelected = len(candidates)

	selected, err := classifyTokens(ctx, d, did, mode, candidates, tokensByID, result)
	if err != nil {
		return result, err
	}

	if mode == RecoverModeDryRun {
		d.Log.Info("RecoverWallet: dry-run complete",
			"did", did, "owned", len(ordered), "selected", result.TokensSelected,
			"in_sync", result.TokensInSync, "missing", result.TokensMissing, "divergent", result.TokensDivergent)
		return result, nil
	}

	if len(selected) == 0 {
		d.Log.Info("RecoverWallet: already in sync, nothing to recover",
			"did", did, "mode", mode, "owned", len(ordered))
		return result, nil
	}

	// Resume the transaction phase from a persisted cursor when a prior run for
	// this DID stopped mid-stream. Transaction blobs are shared across tokens and
	// committed per page, so resuming is safe regardless of the selected set.
	startCursor := RecoveryCursor{Phase: PhaseTx}
	if prior, ok, perr := d.Store.GetRecoveryProgress(ctx, did); perr != nil {
		d.Log.Warn("RecoverWallet: read prior progress failed; starting fresh", "did", did, "err", perr)
	} else if ok && prior.Status == recoveryStatusInProgress && prior.Phase == PhaseTx && prior.LastTxID != "" {
		startCursor = RecoveryCursor{Phase: PhaseTx, LastTxID: prior.LastTxID}
		d.Log.Info("RecoverWallet: resuming transaction phase", "did", did, "from_tx", prior.LastTxID)
	}

	saveProgress := func(c RecoveryCursor) {
		if err := d.Store.SaveRecoveryProgress(ctx, did, mode, c); err != nil {
			d.Log.Warn("RecoverWallet: save progress failed; continuing", "did", did, "err", err)
		}
	}
	saveProgress(startCursor)

	txPersisted, txPages, err := pullTransactions(ctx, d, did, peer, nonce, signature, startCursor, saveProgress)
	if err != nil {
		return result, err
	}
	result.PagesFetched += txPages
	result.TransactionsPersisted += txPersisted

	tokensToPin := finalizeTokens(ctx, d, did, tokensByID, ordered, selected, result)
	result.TokensPinned, result.TokensPinFailed = pinRecoveredTokenContent(ctx, d.Wallet, d.Log, did, tokensToPin)

	if opts.SelfTest {
		result.SelfTestOK, result.SelfTestFailed = selfTestTokens(d, did, ordered, selected, tokensByID)
	}

	if err := d.Store.CompleteRecoveryProgress(ctx, did); err != nil {
		d.Log.Warn("RecoverWallet: mark progress complete failed", "did", did, "err", err)
	}

	d.Log.Info("RecoverWallet: recovery complete",
		"did", did, "mode", mode,
		"selected", result.TokensSelected, "missing", result.TokensMissing, "divergent", result.TokensDivergent,
		"tokens_seen", result.TokensSeen, "tokens_failed", result.TokensFailed,
		"transactions_persisted", result.TransactionsPersisted,
		"tokens_pinned", result.TokensPinned, "tokens_pin_failed", result.TokensPinFailed,
		"self_test_ok", result.SelfTestOK, "self_test_failed", result.SelfTestFailed)
	return result, nil
}

// filterCandidates narrows the owned token list by the selective filters in
// opts. An empty TokenTypes or TokenIDs means no filter on that dimension; with
// neither set the full list is returned unchanged.
func filterCandidates(ordered []string, tokensByID map[string]*RecoveredToken, opts RecoverOptions) []string {
	typeSet := toLowerSet(opts.TokenTypes)
	idSet := toSet(opts.TokenIDs)
	if typeSet == nil && idSet == nil {
		return ordered
	}
	out := make([]string, 0, len(ordered))
	for _, tid := range ordered {
		t := tokensByID[tid]
		if typeSet != nil && !typeSet[strings.ToLower(t.TokenType)] {
			continue
		}
		if idSet != nil && !idSet[tid] {
			continue
		}
		out = append(out, tid)
	}
	return out
}

// classifyTokens decides which candidates to recover. Full mode selects all of
// them. Delta and dry-run read the local token state once and select only the
// tokens missing locally or whose local token_state_hash or token_status differs
// from the fullnode copy. The in-sync, missing, and divergent counters fold into
// result.
func classifyTokens(ctx context.Context, d ClientDeps, did, mode string, candidates []string, tokensByID map[string]*RecoveredToken, result *WalletSyncResult) (map[string]bool, error) {
	if mode == RecoverModeFull {
		selected := make(map[string]bool, len(candidates))
		for _, tid := range candidates {
			selected[tid] = true
		}
		return selected, nil
	}

	local, err := d.Store.GetLocalTokenState(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("classifyTokens: read local token state: %w", err)
	}
	selected := make(map[string]bool)
	for _, tid := range candidates {
		want := tokensByID[tid].CurrentState
		have, ok := local[tid]
		switch {
		case !ok:
			result.TokensMissing++
			selected[tid] = true
		case have.TokenStateHash != want.TokenStateHash || have.TokenStatus != want.TokenStatus:
			result.TokensDivergent++
			selected[tid] = true
		default:
			result.TokensInSync++
		}
	}
	return selected, nil
}

// selfTestTokens verifies each selected token was rebuilt coherently: the local
// tokens row carries the expected state hash and its chain has at least the
// expected number of rows. It reuses the wallet read path via
// Store.VerifyRecoveredToken so a botched rebuild surfaces at recovery time, not
// at the next spend. Failures are counted, not fatal.
func selfTestTokens(d ClientDeps, did string, ordered []string, selected map[string]bool, tokensByID map[string]*RecoveredToken) (ok int, failed int) {
	for _, tid := range ordered {
		if !selected[tid] {
			continue
		}
		t := tokensByID[tid]
		if err := d.Store.VerifyRecoveredToken(tid, t.CurrentState.TokenStateHash, len(t.Chain)); err != nil {
			d.Log.Warn("selfTestTokens: token failed verification", "did", did, "tokenID", tid, "err", err)
			failed++
			continue
		}
		ok++
	}
	if failed > 0 {
		d.Log.Warn("selfTestTokens: some recovered tokens failed verification", "did", did, "ok", ok, "failed", failed)
	}
	return ok, failed
}

// toSet returns a lookup set of the non-empty items, or nil when there are none.
func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, it := range items {
		if it != "" {
			set[it] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// toLowerSet returns a lower-cased lookup set of the non-empty items, or nil when
// there are none.
func toLowerSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, it := range items {
		if it != "" {
			set[strings.ToLower(it)] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}
