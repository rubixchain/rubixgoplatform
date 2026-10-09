package fullnode

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for the per-bundle sync memo. It drops repeat syncs, so most tests check
// the cases where it must NOT: a different peer, a failed sync, a token since
// advanced locally, an expired record, another bundle.

// syncRecorder stands in for the peer sync and records each call that reached it.
type syncRecorder struct {
	mu    sync.Mutex
	calls []syncCall
	err   error
}

type syncCall struct {
	peerDID  string
	tokenIDs []string
}

func (s *syncRecorder) record(peerDID string, tokenIDs []string, _ map[string]string, _ []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, syncCall{peerDID: peerDID, tokenIDs: append([]string(nil), tokenIDs...)})
	return nil
}

func (s *syncRecorder) snapshot() []syncCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]syncCall(nil), s.calls...)
}

func (s *syncRecorder) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// memoCore returns a processor whose peer sync goes to the returned recorder.
func memoCore(t *testing.T, ttl time.Duration) (*DynamicTxnProcessor, *syncRecorder) {
	t.Helper()
	p, cancel := newTestProcessor(10, 0)
	t.Cleanup(cancel)
	p.bundle.syncMemoTTL = ttl
	p.syncMemo = newSyncedTokenMemo(ttl)

	recorder := &syncRecorder{}
	p.syncChains = recorder.record
	return p, recorder
}

// Two transactions in one bundle needing the same token from the same peer:
// only the first may go to the network.
func TestSyncOnceSkipsARepeatWithinTheBundle(t *testing.T) {
	p, recorder := memoCore(t, time.Second)

	// txn-T names txn-S as its previous transaction, so they form one bundle.
	p.registerInflight(eventWithDeps("txn-S"))
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	for _, txnID := range []string{"txn-S", "txn-T"} {
		if err := p.syncChainsOnce(txnID, "peer-1", []string{"token-a"}, nil, nil); err != nil {
			t.Fatalf("syncChainsOnce(%s) = %v, want nil", txnID, err)
		}
	}

	if got := recorder.snapshot(); len(got) != 1 {
		t.Errorf("the peer was asked %d times, want 1: %v", len(got), got)
	}
	if got := atomic.LoadInt64(&p.syncsSkipped); got != 1 {
		t.Errorf("syncsSkipped = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&p.syncsIssued); got != 1 {
		t.Errorf("syncsIssued = %d, want 1", got)
	}
}

// Only already-synced tokens are dropped; the rest of the call is still fetched.
func TestSyncOnceFiltersOnlyTheSeenTokens(t *testing.T) {
	p, recorder := memoCore(t, time.Second)
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil); err != nil {
		t.Fatalf("first sync = %v, want nil", err)
	}
	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a", "token-b"}, nil, nil); err != nil {
		t.Fatalf("second sync = %v, want nil", err)
	}

	calls := recorder.snapshot()
	if len(calls) != 2 {
		t.Fatalf("the peer was asked %d times, want 2", len(calls))
	}
	if !reflect.DeepEqual(calls[1].tokenIDs, []string{"token-b"}) {
		t.Errorf("second request asked for %v, want only [token-b]", calls[1].tokenIDs)
	}
}

// Different peers can hold different chains for the same token (transfer tokens
// from the initiator, pledge tokens from each quorum), so the memo is per peer.
func TestSyncOnceDoesNotSuppressADifferentPeer(t *testing.T) {
	p, recorder := memoCore(t, time.Second)
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil)
	p.syncChainsOnce("txn-T", "peer-2", []string{"token-a"}, nil, nil)

	calls := recorder.snapshot()
	if len(calls) != 2 {
		t.Fatalf("the peers were asked %d times, want 2", len(calls))
	}
	if calls[1].peerDID != "peer-2" {
		t.Errorf("second request went to %s, want peer-2", calls[1].peerDID)
	}
}

// A failed sync must not be remembered, or one unreachable peer would suppress
// the retry for a whole TTL.
func TestSyncOnceDoesNotMarkAFailedSync(t *testing.T) {
	p, recorder := memoCore(t, time.Second)
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	wantErr := errors.New("peer unreachable")
	recorder.fail(wantErr)
	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil); !errors.Is(err, wantErr) {
		t.Fatalf("syncChainsOnce() = %v, want %v", err, wantErr)
	}

	recorder.fail(nil)
	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil); err != nil {
		t.Fatalf("retry after a failed sync = %v, want nil", err)
	}

	if got := recorder.snapshot(); len(got) != 1 {
		t.Errorf("the retry recorded %d successful syncs, want 1 — the failure must not have been remembered", len(got))
	}
}

// What one bundle synced must not suppress a sync for another bundle.
func TestSyncOnceScopesToTheBundle(t *testing.T) {
	p, recorder := memoCore(t, time.Second)
	p.registerInflight(eventWithDeps("txn-T1", "txn-S1"))
	p.registerInflight(eventWithDeps("txn-T2", "txn-S2"))

	p.syncChainsOnce("txn-T1", "peer-1", []string{"token-a"}, nil, nil)
	p.syncChainsOnce("txn-T2", "peer-1", []string{"token-a"}, nil, nil)

	if got := recorder.snapshot(); len(got) != 2 {
		t.Errorf("the peer was asked %d times across two bundles, want 2", len(got))
	}
}

// Persisting a transaction advances the local tip of its tokens, so their memo
// records are stale and must be dropped.
func TestSyncOnceInvalidatesOnPersist(t *testing.T) {
	p, recorder := memoCore(t, time.Second)
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	p.syncChainsOnce("txn-T", "peer-1", []string{"token-a", "token-b"}, nil, nil)
	p.invalidateSyncedTokens([]string{"token-a"})
	p.syncChainsOnce("txn-T", "peer-1", []string{"token-a", "token-b"}, nil, nil)

	calls := recorder.snapshot()
	if len(calls) != 2 {
		t.Fatalf("the peer was asked %d times, want 2", len(calls))
	}
	if !reflect.DeepEqual(calls[1].tokenIDs, []string{"token-a"}) {
		t.Errorf("second request asked for %v, want only the invalidated [token-a]", calls[1].tokenIDs)
	}
}

// Records expire after the TTL, so a bundle that never drains cannot suppress
// syncs forever.
func TestSyncOnceRecordExpires(t *testing.T) {
	p, recorder := memoCore(t, 30*time.Millisecond)
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil)
	time.Sleep(50 * time.Millisecond)
	p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil)

	if got := recorder.snapshot(); len(got) != 2 {
		t.Errorf("the peer was asked %d times across the TTL boundary, want 2", len(got))
	}
}

// seen only expires keys it is asked about, so the sweep is what frees records
// of a bundle nobody revisits.
func TestSyncMemoSweepDropsExpiredRecords(t *testing.T) {
	m := newSyncedTokenMemo(20 * time.Millisecond)
	m.mark("bundle-1", "peer-1", []string{"token-a", "token-b"})

	if got := m.len(); got != 2 {
		t.Fatalf("len() = %d after marking two tokens, want 2", got)
	}

	m.sweep()
	if got := m.len(); got != 2 {
		t.Errorf("len() = %d, want 2 — the sweep dropped records that had not expired", got)
	}

	time.Sleep(40 * time.Millisecond)
	m.sweep()
	if got := m.len(); got != 0 {
		t.Errorf("len() = %d after the TTL passed, want 0", got)
	}
}

func TestSyncMemoIgnoresDegenerateInput(t *testing.T) {
	m := newSyncedTokenMemo(time.Second)

	m.mark("", "peer-1", []string{"token-a"})
	m.mark("bundle-1", "peer-1", nil)
	m.mark("bundle-1", "peer-1", []string{""})
	m.invalidate(nil)
	m.invalidate([]string{""})

	if got := m.len(); got != 0 {
		t.Errorf("len() = %d, want 0", got)
	}
	if m.seen("", syncKey{tokenID: "token-a", peerDID: "peer-1"}) {
		t.Error("seen() returned true for an empty bundle")
	}
}

// Invalidating a token must clear it for every bundle and peer, and only it.
func TestSyncMemoInvalidateSpansBundlesAndPeers(t *testing.T) {
	m := newSyncedTokenMemo(time.Second)
	m.mark("bundle-1", "peer-1", []string{"token-a", "token-b"})
	m.mark("bundle-2", "peer-2", []string{"token-a"})

	m.invalidate([]string{"token-a"})

	if m.seen("bundle-1", syncKey{tokenID: "token-a", peerDID: "peer-1"}) {
		t.Error("token-a still seen in bundle-1 after invalidation")
	}
	if m.seen("bundle-2", syncKey{tokenID: "token-a", peerDID: "peer-2"}) {
		t.Error("token-a still seen in bundle-2 after invalidation")
	}
	if !m.seen("bundle-1", syncKey{tokenID: "token-b", peerDID: "peer-1"}) {
		t.Error("token-b was dropped; only the named tokens should be")
	}
}

// A transaction in no bundle uses its own ID as scope, so it still skips repeat
// syncs across its own retries; bundle members share one scope.
func TestBundleScopeFallsBackToTheTransactionID(t *testing.T) {
	p, _ := memoCore(t, time.Second)

	if got := p.bundleScope("txn-unrelated"); got != "txn-unrelated" {
		t.Errorf("bundleScope() = %q for a transaction with no component, want its own ID", got)
	}

	p.registerInflight(eventWithDeps("txn-T", "txn-S"))
	scope := p.bundleScope("txn-T")
	if scope == "" {
		t.Fatal("bundleScope() = \"\" for a transaction in a component")
	}
	if got := p.bundleScope("txn-S"); got != scope {
		t.Errorf("bundleScope(txn-S) = %q, want %q — bundle members share one scope", got, scope)
	}
	if got := p.inflight.componentRoot("txn-nowhere"); got != "" {
		t.Errorf("componentRoot() = %q for an unknown transaction, want \"\"", got)
	}
}

// The memo helpers must tolerate a nil processor rather than panic.
func TestSyncMemoHelpersToleratesNoProcessor(t *testing.T) {
	var p *DynamicTxnProcessor

	if got := p.bundleScope("txn-T"); got != "txn-T" {
		t.Errorf("bundleScope() = %q, want the transaction ID", got)
	}
	if got := p.filterRecentlySynced("bundle-1", "peer-1", []string{"token-a"}); len(got) != 1 {
		t.Errorf("filterRecentlySynced() = %v, want the tokens unchanged", got)
	}
	p.markSynced("bundle-1", "peer-1", []string{"token-a"})
	p.invalidateSyncedTokens([]string{"token-a"})
}

// Workers read and write the memo concurrently. Run with -race.
func TestSyncMemoIsSafeUnderConcurrency(t *testing.T) {
	const workers = 50

	m := newSyncedTokenMemo(time.Second)
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(workers * 3)

	for i := 0; i < workers; i++ {
		bundle := fmt.Sprintf("bundle-%02d", i%5)
		tokenID := fmt.Sprintf("token-%02d", i)

		go func() {
			defer done.Done()
			start.Wait()
			m.mark(bundle, "peer-1", []string{tokenID})
		}()
		go func() {
			defer done.Done()
			start.Wait()
			_ = m.seen(bundle, syncKey{tokenID: tokenID, peerDID: "peer-1"})
			_ = m.len()
		}()
		go func() {
			defer done.Done()
			start.Wait()
			m.invalidate([]string{tokenID})
			m.sweep()
		}()
	}

	start.Done()
	done.Wait()
}

// A peer chain that fails to apply must come back as a transient error, not a
// validation verdict, and must not be remembered, so the retry fetches again.
func TestSyncOnceTreatsAnApplyFailureAsTransient(t *testing.T) {
	p, recorder := memoCore(t, time.Second)
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	applyFailed := errors.New("SyncTransactionChainsFromPeer: chain apply failed for 1 of 1 token(s) from peer-1 [token-a: fork detected]")
	recorder.fail(applyFailed)

	err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil)
	if !errors.Is(err, applyFailed) {
		t.Fatalf("syncChainsOnce() = %v, want the apply failure to reach the caller", err)
	}
	if !errors.Is(err, errDependencyTimeout) {
		t.Error("the apply failure is not tagged transient, so the retry ladder will not run and the transaction is dead-lettered")
	}
	if errors.Is(err, errValidationFailed) {
		t.Error("a peer's unusable chain was classified as this node's verdict on the transaction")
	}

	// The retry must reach the network, not be suppressed by the memo.
	recorder.fail(nil)
	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil); err != nil {
		t.Fatalf("retry after an apply failure = %v, want nil", err)
	}
	calls := recorder.snapshot()
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].tokenIDs, []string{"token-a"}) {
		t.Errorf("retry reached the network %d time(s) with %v, want 1 fetch of token-a", len(calls), calls)
	}
}
