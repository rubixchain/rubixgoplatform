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

// Tests for the sync-once gate.
//
// The gate exists to drop a repeat, so almost every test here is about the cases
// where it must *not* drop one: a different peer, a failed sync, a token this
// node has since advanced, an expired record, another bundle. Suppressing a sync
// that would have succeeded is the only way this can do harm, and each of those
// is a way it could.

// memoCore wires a Core whose peer sync is driven by syncChains, so the gate can
// be exercised without a peer. The returned recorder holds one entry per sync
// that actually reached the network layer.
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

// The case the gate is for: two members of one bundle both needing the same
// token from the same peer. The second must not go to the network.
func TestSyncOnceSkipsARepeatWithinTheBundle(t *testing.T) {
	p, recorder := memoCore(t, time.Second)

	// One bundle: the transfer names the split, which is what merges them.
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

// Only the seen tokens are dropped. A call mixing one of each must still fetch
// the one it has not seen.
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

// Different peers hold genuinely different chains for the same token: transfer
// tokens come from the initiator, pledge tokens from each quorum member. Keying
// on the token alone would suppress a sync that would have succeeded.
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

// A failed sync must stay re-syncable. Marking on attempt rather than on success
// would let one unreachable peer suppress, for a whole TTL, the retry that would
// have worked.
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

// Two bundles are two scopes. What one of them fetched says nothing about what
// the other is looking at.
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

// Once this node persists a transaction touching a token, that token's tip has
// advanced and every record of what a peer held a moment ago is describing a
// chain one entry short.
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

// The TTL is what stops a bundle that never drains from holding an opinion about
// a chain indefinitely.
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

// seen only expires the keys something asks about again, so a bundle nobody
// revisits would hold its records forever without the sweep.
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

// Invalidation is a fact about the token, not about who observed it, so it must
// reach every bundle and every peer.
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

// A transaction with no component scopes to itself. It still collapses the
// repeat syncs across its own retry attempts, and it cannot reach past the one
// transaction.
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

// A Core with no transaction processor is not a fullnode, so there is nothing to
// scope a memo to and the call must fall through rather than panic.
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

// The memo is read on the validation path, which every worker is on at once, and
// written from persists happening concurrently with those reads. Run with -race.
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

// End to end through the closure the validator actually calls: a split and the
// transfer that spends it, sharing a bundle, needing the same token from the
// same peer. The second must be served from the memo.
func TestSyncOnceThroughTheBundleCascade(t *testing.T) {
	p, recorder := memoCore(t, time.Second)

	producer := p.registerInflight(eventWithDeps("txn-S"))
	consumer := p.registerInflight(eventWithDeps("txn-T", "txn-S"))
	if producer == nil || consumer == nil {
		t.Fatal("registerInflight() returned nil")
	}

	if err := p.syncChainsOnce("txn-S", "peer-1", []string{"token-a"}, nil, nil); err != nil {
		t.Fatalf("producer sync = %v, want nil", err)
	}
	p.releaseWaiters("txn-S")
	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil); err != nil {
		t.Fatalf("consumer sync = %v, want nil", err)
	}

	if got := recorder.snapshot(); len(got) != 1 {
		t.Errorf("the peer was asked %d times for one bundle's token, want 1", len(got))
	}

	// And once the node advances that token itself, the memo stops answering for
	// it.
	p.invalidateSyncedTokens([]string{"token-a"})
	if err := p.syncChainsOnce("txn-T", "peer-1", []string{"token-a"}, nil, nil); err != nil {
		t.Fatalf("sync after invalidation = %v, want nil", err)
	}
	if got := recorder.snapshot(); len(got) != 2 {
		t.Errorf("the peer was asked %d times after invalidation, want 2", len(got))
	}
	if got := p.syncMemo.len(); got != 1 {
		t.Errorf("syncMemo holds %d records, want 1", got)
	}
}
