package fullnode

import (
	"sync"
	"sync/atomic"
	"time"
)

// The sync-once memo skips a chain sync of the same token from the same peer
// already done for this bundle within the TTL. It is an optimisation, not a
// correctness mechanism. A wrong skip makes phase 3 report "chain mismatch after
// sync", a verdict (no retry, dead-letter) unless deferVerdictWhileDependencyPending
// defers it; persisting a transaction invalidates its tokens to limit this.

// syncKey identifies one chain sync: a token, and the peer it came from. The
// peer is part of the key because different peers (initiator, quorum members)
// can hold different chains for the same token.
type syncKey struct {
	tokenID string
	peerDID string
}

// syncedTokenMemo records which (token, peer) pairs have been synced
// successfully, scoped per bundle and expired by TTL. It has its own mutex,
// separate from the registry's.
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

// mark records a successful sync of each token from peerDID. Call it only on
// success, so a failed sync stays re-syncable on retry.
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
// every peer. Called when this node advances their tips, after which every
// earlier record describes a chain that is now one entry short.
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

// sweep drops every expired record. seen only expires keys it is asked about,
// so records of a bundle that is never revisited need this periodic sweep.
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

// bundleScope returns the memo scope for a transaction: its bundle's component
// root, or its own ID if it has none. The root can change when bundles merge,
// which only causes a cache miss and one extra sync.
func (p *DynamicTxnProcessor) bundleScope(txnID string) string {
	if p == nil || p.inflight == nil {
		return txnID
	}
	if root := p.inflight.componentRoot(txnID); root != "" {
		return root
	}
	return txnID
}

// filterRecentlySynced drops the tokens already fetched from this peer for this
// bundle.
func (p *DynamicTxnProcessor) filterRecentlySynced(bundle, peerDID string, tokenIDs []string) []string {
	if p == nil || p.syncMemo == nil {
		return tokenIDs
	}

	kept := make([]string, 0, len(tokenIDs))
	var skipped []string
	for _, tokenID := range tokenIDs {
		if p.syncMemo.seen(bundle, syncKey{tokenID: tokenID, peerDID: peerDID}) {
			skipped = append(skipped, tokenID)
			continue
		}
		kept = append(kept, tokenID)
	}

	if len(skipped) > 0 {
		atomic.AddInt64(&p.syncsSkipped, int64(len(skipped)))
		p.host.Log().Debug("Skipping chain sync for tokens already fetched in this bundle",
			"bundle", bundle, "peerDID", peerDID, "skipped", skipped, "stillNeeded", len(kept))
	}
	return kept
}

// markSynced records that these tokens were fetched successfully from peerDID.
func (p *DynamicTxnProcessor) markSynced(bundle, peerDID string, tokenIDs []string) {
	if p == nil {
		return
	}
	p.syncMemo.mark(bundle, peerDID, tokenIDs)
}

// invalidateSyncedTokens forgets what the memo knows about tokens this node has
// just advanced.
func (p *DynamicTxnProcessor) invalidateSyncedTokens(tokenIDs []string) {
	if p == nil {
		return
	}
	p.syncMemo.invalidate(tokenIDs)
}

// syncChainsOnce fetches chains from a peer, skipping tokens this bundle has
// already fetched from it. It returns nil when everything was skipped; the
// caller's phase 3 re-reads every token from the database regardless.
func (p *DynamicTxnProcessor) syncChainsOnce(txnID, peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
	if p == nil || p.syncChains == nil {
		// No sync hook installed (processor not built by NewTxnProcessor): sync
		// directly, without the memo.
		return p.host.SyncTransactionChainsFromPeer(peerDID, tokenIDs, prevTxIDs, excludeTxIDs, false, p.host.IsFullNode())
	}

	bundle := p.bundleScope(txnID)
	tokens := p.filterRecentlySynced(bundle, peerDID, tokenIDs)
	if len(tokens) == 0 {
		return nil
	}

	// prevTxIDs is passed whole; entries for skipped tokens are never read
	// because the peer only returns the tokens asked for.
	atomic.AddInt64(&p.syncsIssued, int64(len(tokens)))
	if err := p.syncChains(peerDID, tokens, prevTxIDs, excludeTxIDs); err != nil {
		// Tag as transient here; untagged, validation would report a peer
		// failure as a verdict and dead-letter the transaction. The memo is
		// not marked, so the retry re-fetches.
		return classify(errDependencyTimeout, err)
	}

	p.markSynced(bundle, peerDID, tokens)
	return nil
}
