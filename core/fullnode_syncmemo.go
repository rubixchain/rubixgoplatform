package core

import (
	"sync"
	"sync/atomic"
	"time"
)

// The sync-once gate.
//
// TokenChainIntegrityCheck reaches its sync phase only for tokens whose local
// tip already failed to match, and it issues one sync per peer holding those
// tokens (core/consensus/checks.go). Two transactions in the same bundle that
// both need the same token from the same peer therefore issue that sync twice,
// and the second call cannot learn anything the first did not already apply.
//
// This remembers the successful ones and drops the repeats. It is a cost
// optimisation and a second line of defence for concurrent siblings, nothing
// more: the unvalidated-tail bug is fixed by the sync guard, and bundle-internal
// dependencies are resolved before validation by the readiness gate, so this
// must not be asked to carry correctness.
//
// Why a skip is safe at all. The sync phase is reached only for a token that has
// already mismatched, so skipping cannot rescue a token — it can only avoid a
// useless retry, and it is useless precisely when the same token was fetched
// from the same peer moments ago. A skip therefore turns a slow double-failure
// into a fast single-failure, and never a success into a failure. The
// re-verification phase afterwards re-reads every token from the database, so a
// skip that turns out to have been wrong surfaces as an ordinary validation
// failure rather than as wrong data on disk.

// syncKey identifies one chain sync: a token, and the peer it came from.
//
// Never the token alone. Transfer tokens are synced from the transaction's
// initiator and pledge tokens from each quorum member, and those peers hold
// genuinely different chains — a token-only key would suppress a sync that would
// have succeeded.
type syncKey struct {
	tokenID string
	peerDID string
}

// syncedTokenMemo records which (token, peer) pairs have been synced
// successfully, scoped per bundle.
//
// The bundle is what makes the scope correct: members of one bundle are looking
// at the same moment in a token's history, so what one of them fetched is what
// the next one would have fetched. The TTL is what makes it bounded — a bundle
// that never drains would otherwise strand its entries — so scope decides
// correctness and the TTL decides memory.
//
// Its own mutex rather than the registry's. Nothing here has to agree with what
// is in flight, and this lock is taken on the validation path, which is exactly
// where the registry lock must not be.
type syncedTokenMemo struct {
	mu      sync.Mutex
	entries map[string]map[syncKey]time.Time
	ttl     time.Duration
}

func newSyncedTokenMemo(ttl time.Duration) *syncedTokenMemo {
	return &syncedTokenMemo{
		entries: make(map[string]map[syncKey]time.Time),
		ttl:     ttl,
	}
}

// seen reports whether key was synced successfully for this bundle within the
// TTL, dropping the record if it has expired.
func (m *syncedTokenMemo) seen(bundle string, key syncKey) bool {
	if m == nil || bundle == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	at, recorded := m.entries[bundle][key]
	if !recorded {
		return false
	}
	if time.Since(at) > m.ttl {
		delete(m.entries[bundle], key)
		if len(m.entries[bundle]) == 0 {
			delete(m.entries, bundle)
		}
		return false
	}
	return true
}

// mark records a successful sync of each token from peerDID.
//
// Only ever called after a sync returned without error. A failed sync has to
// stay re-syncable, or one unreachable peer suppresses for a whole TTL the retry
// that would have worked.
func (m *syncedTokenMemo) mark(bundle, peerDID string, tokenIDs []string) {
	if m == nil || bundle == "" || len(tokenIDs) == 0 {
		return
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	byKey, tracked := m.entries[bundle]
	if !tracked {
		byKey = make(map[syncKey]time.Time, len(tokenIDs))
		m.entries[bundle] = byKey
	}
	for _, tokenID := range tokenIDs {
		if tokenID == "" {
			continue
		}
		byKey[syncKey{tokenID: tokenID, peerDID: peerDID}] = now
	}
}

// invalidate forgets every record of these tokens, in every bundle and from
// every peer.
//
// Called when this node persists a transaction touching them. Their tips have
// legitimately advanced, so every record of what a peer held a moment ago now
// describes a chain one entry short. Across all bundles and peers because the
// advance is a fact about the token, not about who observed it.
func (m *syncedTokenMemo) invalidate(tokenIDs []string) {
	if m == nil || len(tokenIDs) == 0 {
		return
	}

	stale := make(map[string]struct{}, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if tokenID != "" {
			stale[tokenID] = struct{}{}
		}
	}
	if len(stale) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for bundle, byKey := range m.entries {
		for key := range byKey {
			if _, drop := stale[key.tokenID]; drop {
				delete(byKey, key)
			}
		}
		if len(byKey) == 0 {
			delete(m.entries, bundle)
		}
	}
}

// sweep drops every expired record.
//
// seen expires entries as it meets them, but only for keys something asks about
// again. A bundle that is never revisited would otherwise hold its records
// forever, so this runs on the existing dedup ticker.
func (m *syncedTokenMemo) sweep() {
	if m == nil {
		return
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	for bundle, byKey := range m.entries {
		for key, at := range byKey {
			if now.Sub(at) > m.ttl {
				delete(byKey, key)
			}
		}
		if len(byKey) == 0 {
			delete(m.entries, bundle)
		}
	}
}

// len returns how many records the memo holds across every bundle.
func (m *syncedTokenMemo) len() int {
	if m == nil {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var total int
	for _, byKey := range m.entries {
		total += len(byKey)
	}
	return total
}

// bundleScope returns the memo scope for a transaction.
//
// The component root, which is an artefact of how the merges happened to fall
// and can change when two components of similar size meet. That instability is
// tolerable precisely because this is only ever a cache key: a changed scope is
// a miss and one extra sync, never a wrong suppression.
//
// A transaction with no component falls back to its own ID. In practice that
// case does not arise on the sync path — a transaction only reaches the sync
// phase for a token that names a previous transaction, and naming one is what
// puts it in a component — but scoping to itself is the right answer if it ever
// does: it still collapses the repeat syncs across its own retry attempts, and
// it cannot reach beyond the one transaction.
func (c *Core) bundleScope(txnID string) string {
	if c.txnProcessor == nil || c.txnProcessor.inflight == nil {
		return txnID
	}
	if root := c.txnProcessor.inflight.componentRoot(txnID); root != "" {
		return root
	}
	return txnID
}

// filterRecentlySynced drops the tokens already fetched from this peer for this
// bundle.
func (c *Core) filterRecentlySynced(bundle, peerDID string, tokenIDs []string) []string {
	if c.txnProcessor == nil || c.txnProcessor.syncMemo == nil {
		return tokenIDs
	}

	kept := make([]string, 0, len(tokenIDs))
	var skipped []string
	for _, tokenID := range tokenIDs {
		if c.txnProcessor.syncMemo.seen(bundle, syncKey{tokenID: tokenID, peerDID: peerDID}) {
			skipped = append(skipped, tokenID)
			continue
		}
		kept = append(kept, tokenID)
	}

	if len(skipped) > 0 {
		atomic.AddInt64(&c.txnProcessor.syncsSkipped, int64(len(skipped)))
		c.log.Debug("Skipping chain sync for tokens already fetched in this bundle",
			"bundle", bundle, "peerDID", peerDID, "skipped", skipped, "stillNeeded", len(kept))
	}
	return kept
}

// markSynced records that these tokens were fetched successfully from peerDID.
func (c *Core) markSynced(bundle, peerDID string, tokenIDs []string) {
	if c.txnProcessor == nil {
		return
	}
	c.txnProcessor.syncMemo.mark(bundle, peerDID, tokenIDs)
}

// invalidateSyncedTokens forgets what the memo knows about tokens this node has
// just advanced.
func (c *Core) invalidateSyncedTokens(tokenIDs []string) {
	if c.txnProcessor == nil {
		return
	}
	c.txnProcessor.syncMemo.invalidate(tokenIDs)
}

// syncChainsOnce fetches chains from a peer, skipping tokens this bundle has
// already fetched from it.
//
// Returning nil when everything was filtered is correct rather than a silent
// no-op: the caller re-verifies every token against the database immediately
// afterwards, so a skip it should not have made fails there.
func (c *Core) syncChainsOnce(txnID, peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
	p := c.txnProcessor
	if p == nil || p.syncChains == nil {
		// Not a fullnode, so there are no bundles to scope a memo to.
		return c.SyncTransactionChainsFromPeer(peerDID, tokenIDs, prevTxIDs, excludeTxIDs, false, c.fullNode)
	}

	bundle := c.bundleScope(txnID)
	tokens := c.filterRecentlySynced(bundle, peerDID, tokenIDs)
	if len(tokens) == 0 {
		return nil
	}

	// prevTxIDs is passed through whole. It is read per token from the peer's
	// response, which only ever contains tokens that were asked for, so the
	// entries for filtered-out tokens are never looked at.
	atomic.AddInt64(&p.syncsIssued, int64(len(tokens)))
	if err := p.syncChains(peerDID, tokens, prevTxIDs, excludeTxIDs); err != nil {
		return err
	}

	c.markSynced(bundle, peerDID, tokens)
	return nil
}
