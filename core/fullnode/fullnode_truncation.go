package fullnode

import (
	"sync"
	"time"
)

// Truncation records for the fullnode transaction pipeline.
//
// GuardAgainstInflight trims a peer's chain to the longest prefix containing
// nothing this node holds. That is the right thing to apply, but it is a *prefix*
// — so a held transaction sitting early in the chain drops everything after it,
// including entries the current transaction genuinely needed. The sync then
// reports success having applied nothing useful, the integrity check's phase 3
// finds the chain still short, and the plain mismatch it reports reads as this
// node's own verdict.
//
// deferVerdictWhileDependencyPending already catches the common shape of this by
// asking whether a producer the transaction DECLARES is still pending. That
// misses the case above: the entry the guard stopped at need not be the
// transaction's own producer, only something further back in the same chain.
// Those are exactly the arrears a fullnode catching up accumulates, and the
// queued set widened the trigger from the handful of transactions under a worker
// to everything sitting in txnQueue.
//
// So the guard leaves a note. It runs as a method on the processor and already
// knows which token it truncated, which is enough to answer the question later
// without threading a new error contract back through
// SyncTransactionChainsFromPeer — shared with the quorum path, and swallowing
// apply errors by design until very recently.
//
// The note is deliberately weak evidence: it says a sync for this token came
// back trimmed a moment ago, not that this transaction was harmed by it. Acting
// on it only ever costs a retry, and the retry ladder is bounded at three, so a
// stale note is worth a few seconds, never a wrong outcome.

// truncationTTL is how long a note stays worth acting on. It has to outlive one
// validation pass — the guard writes it during phase 2's sync and the
// classification reads it after phase 3, with peer round-trips in between — and
// nothing beyond that, since a transaction retrying past it has already had its
// three attempts.
const truncationTTL = 30 * time.Second

// truncationLog records, per token, when a chain sync for it was last trimmed.
type truncationLog struct {
	mu sync.Mutex
	at map[string]time.Time
}

func newTruncationLog() *truncationLog {
	return &truncationLog{at: make(map[string]time.Time)}
}

// record notes that a sync for tokenID came back trimmed. A later truncation
// overwrites an earlier one: only the most recent matters.
func (l *truncationLog) record(tokenID string) {
	if l == nil || tokenID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.at[tokenID] = time.Now()
}

// recentlyTruncated returns the first of tokenIDs whose sync was trimmed within
// ttl, and whether one was found.
func (l *truncationLog) recentlyTruncated(tokenIDs []string, ttl time.Duration) (string, bool) {
	if l == nil || len(tokenIDs) == 0 {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	for _, tokenID := range tokenIDs {
		if at, noted := l.at[tokenID]; noted && at.After(cutoff) {
			return tokenID, true
		}
	}
	return "", false
}

// forget drops the notes for these tokens. Called when a transaction persists:
// the chain has just advanced, so whatever a sync was short of before is no
// longer what this node is short of now.
func (l *truncationLog) forget(tokenIDs []string) {
	if l == nil || len(tokenIDs) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, tokenID := range tokenIDs {
		delete(l.at, tokenID)
	}
}

// sweep drops notes older than ttl and returns how many went. Without it a token
// synced once on a quiet node would keep its note for the life of the process.
func (l *truncationLog) sweep(ttl time.Duration) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	dropped := 0
	for tokenID, at := range l.at {
		if !at.After(cutoff) {
			delete(l.at, tokenID)
			dropped++
		}
	}
	return dropped
}

// len returns how many tokens currently carry a note.
func (l *truncationLog) len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.at)
}
