package fullnode

import (
	"sync"
	"time"
)

// Truncation notes. GuardAgainstInflight trims a peer's chain at the first
// locally pending transaction, which can drop entries the current transaction
// needed even when the cut is not at its own previous transaction. The note lets
// deferVerdictWhileDependencyPending treat the resulting phase-3 mismatch as
// transient; a stale note at worst defers a verdict, it never creates one.

// truncationTTL is how long a note stays worth acting on: long enough to span
// one validation pass (written during the sync, read after phase 3).
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

// forget drops the notes for these tokens. Called when a transaction persists,
// since the chain has advanced and the old shortfall no longer applies.
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

// sweep drops notes older than ttl and returns how many went, so notes for
// tokens never touched again do not accumulate.
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
