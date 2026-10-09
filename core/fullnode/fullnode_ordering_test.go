package fullnode

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// End-to-end ordering tests. Unlike the unit tests, these drive the real entry
// points, so they catch a previous transaction still sitting in txnQueue while
// the transaction that spends it is validated.

// A previous transaction still waiting in txnQueue must be treated as pending
// locally, and must stop being treated so once it leaves the pipeline.
func TestQueuedProducerIsHeldThenReleasedEndToEnd(t *testing.T) {
	p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	// A zero timeout can win the enqueue select even when the buffer has room,
	// so allow time for the enqueue to succeed.
	p.enqueueTimeout = time.Second

	// No workers run here, so the split stays on txnQueue as if behind a backlog.
	p.QueueFullnodeTransaction(testEvent("split-1"))
	if !p.queued.has("split-1") {
		t.Fatal("the split was not queued")
	}
	if p.inflight.has("split-1") {
		t.Fatal("the split reached the registry; this test needs it queued only")
	}

	consumer := p.registerInflight(eventWithDeps("transfer-1", "split-1"))
	if consumer == nil {
		t.Fatal("registerInflight returned nil")
	}

	// 1. A queued previous transaction gets the long wait, not the short one
	//    used for transactions this node has never seen.
	if got, want := p.dependencyWait([]string{"split-1"}), p.bundle.inflightWait; got != want {
		t.Errorf("dependencyWait = %v, want %v for a queued producer", got, want)
	}

	// 2. The guard must not ingest the queued split from a peer's chain: it is
	//    pending locally and not yet validated.
	if guarded := p.GuardAgainstInflight("token-a", chain("split-1", "later-1")); len(guarded) != 0 {
		t.Errorf("guard applied %v, want nothing ingested while the split is queued", chainIDs(guarded))
	}

	// 3. A waiting transaction parked on a previous transaction that was only
	//    queued (never registered) is still woken; park() keys on the bare ID.
	if !p.inflight.park(consumer, "split-1") {
		t.Fatal("could not park on a queued producer")
	}
	p.releaseWaiters("split-1")
	select {
	case <-consumer.ready:
	default:
		t.Error("the consumer was not woken when its queued producer persisted")
	}

	// 4. Once it leaves the pipeline the guard must stop trimming, or it could
	//    never be fetched from a peer again.
	p.queued.remove("split-1")
	guarded := p.GuardAgainstInflight("token-a", chain("split-1", "later-1"))
	if !reflect.DeepEqual(chainIDs(guarded), []string{"split-1", "later-1"}) {
		t.Errorf("guard applied %v after the producer resolved, want the whole chain", chainIDs(guarded))
	}
}

// Deferring a verdict must clear the sync memo for the transaction's tokens,
// or the memo would suppress the very sync the retry exists to make.
func TestDeferredVerdictMakesTheRetryReFetch(t *testing.T) {
	p, recorder := memoCore(t, time.Minute)

	consumer := p.registerInflight(eventWithDeps("transfer-1", "split-1"))
	if consumer == nil {
		t.Fatal("registerInflight returned nil")
	}
	tokenID := entryTokenIDs(consumer)[0]

	// Attempt 1 syncs and the memo records it.
	if err := p.syncChainsOnce("transfer-1", "peer-1", []string{tokenID}, nil, nil); err != nil {
		t.Fatalf("first sync = %v, want nil", err)
	}
	if n := len(recorder.snapshot()); n != 1 {
		t.Fatalf("first sync reached the network %d times, want 1", n)
	}

	// Without a deferral the memo suppresses the repeat.
	if err := p.syncChainsOnce("transfer-1", "peer-1", []string{tokenID}, nil, nil); err != nil {
		t.Fatalf("suppressed sync = %v, want nil", err)
	}
	if n := len(recorder.snapshot()); n != 1 {
		t.Fatalf("the memo did not suppress the repeat: %d calls, want 1", n)
	}

	// Now the guard trims that token and the verdict is deferred.
	p.truncated.record(tokenID)
	verdict := classify(errValidationFailed, errors.New("chain mismatch after sync from peer-1"))
	if got := p.deferVerdictWhileDependencyPending(consumer, verdict); errors.Is(got, errValidationFailed) {
		t.Fatalf("the verdict was not deferred: %v", got)
	}

	// The retry must genuinely go back to the peer.
	if err := p.syncChainsOnce("transfer-1", "peer-1", []string{tokenID}, nil, nil); err != nil {
		t.Fatalf("retry sync = %v, want nil", err)
	}
	calls := recorder.snapshot()
	if len(calls) != 2 {
		t.Errorf("the retry reached the network %d times in total, want 2 — the deferral must clear the memo", len(calls))
	} else if !reflect.DeepEqual(calls[1].tokenIDs, []string{tokenID}) {
		t.Errorf("the retry fetched %v, want %v", calls[1].tokenIDs, []string{tokenID})
	}
}

// A previous transaction this node does not hold must not be held back: it
// gets the short wait, its chain syncs in full, and a real verdict is kept.
func TestGenuinelyMissingDependencyStillSyncsAndStillVerdicts(t *testing.T) {
	p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	consumer := p.registerInflight(eventWithDeps("transfer-1", "split-1"))
	if consumer == nil {
		t.Fatal("registerInflight returned nil")
	}
	if p.isPending("split-1") {
		t.Fatal("the producer is held locally; this test needs it genuinely absent")
	}

	// The short wait, because a transaction that is not here may never come.
	if got, want := p.dependencyWait([]string{"split-1"}), p.bundle.unknownWait; got != want {
		t.Errorf("dependencyWait = %v, want %v for an absent producer", got, want)
	}

	// Nothing is trimmed, so the chain that fills the gap is applied in full.
	remote := chain("split-1", "transfer-0")
	guarded := p.GuardAgainstInflight("token-a", remote)
	if !reflect.DeepEqual(chainIDs(guarded), chainIDs(remote)) {
		t.Errorf("guard applied %v, want the whole chain %v", chainIDs(guarded), chainIDs(remote))
	}

	// No pending previous transaction and no recent trim, so the verdict stands
	// and the transaction is stored as invalid.
	verdict := classify(errValidationFailed, errors.New("signature verification failed"))
	if got := p.deferVerdictWhileDependencyPending(consumer, verdict); !errors.Is(got, errValidationFailed) {
		t.Errorf("a genuine verdict was deferred: %v — invalid transactions would never dead-letter", got)
	}
}
