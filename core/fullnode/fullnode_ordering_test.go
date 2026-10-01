package fullnode

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// End-to-end ordering tests.
//
// The units are covered individually elsewhere, and that is the gap these fill:
// the guard tests hand truncateAtInflight an ID set directly, so they would pass
// unchanged against the production race this pipeline exists to close — a
// producer sitting in txnQueue, invisible to everything, while its consumer is
// validated. Each test below drives the real entry point instead and asserts on
// what the next stage actually observes.

// The deep-queue case, driven from QueueFullnodeTransaction rather than from an
// injected set: a producer that no worker has reached yet must be treated as
// held, and must stop being treated as held the moment it resolves.
func TestQueuedProducerIsHeldThenReleasedEndToEnd(t *testing.T) {
	p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	// A zero timeout can win the enqueue select even when the buffer has room.
	// Give admission time to complete so this test exercises a held producer.
	p.enqueueTimeout = time.Second

	// The split is admitted and queued. No workers run in this processor, so it
	// stays on txnQueue exactly as it would behind a backlog.
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

	// 1. The readiness gate must offer the long tier. Before the queued set this
	//    was the short one, so the consumer gave up on something seconds away.
	if got, want := p.dependencyWait([]string{"split-1"}), p.bundle.inflightWait; got != want {
		t.Errorf("dependencyWait = %v, want %v for a queued producer", got, want)
	}

	// 2. The guard must refuse to hand the queued split back from a peer. This is
	//    the "syncing a transaction that is pending locally but not yet
	//    validated" bug in its original form.
	if guarded := p.GuardAgainstInflight("token-a", chain("split-1", "later-1")); len(guarded) != 0 {
		t.Errorf("guard applied %v, want nothing ingested while the split is queued", chainIDs(guarded))
	}

	// 3. The cascade must still reach a consumer parked on a producer that was
	//    only ever queued — park() keys on the bare ID, so this works without the
	//    producer ever having been registered.
	if !p.inflight.park(consumer, "split-1") {
		t.Fatal("could not park on a queued producer")
	}
	p.releaseWaiters("split-1")
	select {
	case <-consumer.ready:
	default:
		t.Error("the consumer was not woken when its queued producer persisted")
	}

	// 4. And once the producer leaves the pipeline the guard must stop trimming,
	//    or the dependency could never be fetched from a peer again.
	p.queued.remove("split-1")
	guarded := p.GuardAgainstInflight("token-a", chain("split-1", "later-1"))
	if !reflect.DeepEqual(chainIDs(guarded), []string{"split-1", "later-1"}) {
		t.Errorf("guard applied %v after the producer resolved, want the whole chain", chainIDs(guarded))
	}
}

// A deferred verdict is only worth anything if the next attempt does more work
// than the last. The memo would otherwise suppress the very sync the retry
// exists to make.
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

	// Without a deferral the memo suppresses the repeat — the gate working.
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

// The regression this whole mechanism is prone to: refusing to fetch something
// that is genuinely absent, or softening a verdict that was real. A dependency
// this node does not hold must behave exactly as it did before any of it
// existed.
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

	// The short tier, because a producer that is simply not here may never come.
	if got, want := p.dependencyWait([]string{"split-1"}), p.bundle.unknownWait; got != want {
		t.Errorf("dependencyWait = %v, want %v for an absent producer", got, want)
	}

	// Nothing is trimmed, so the chain that fills the gap is applied in full.
	remote := chain("split-1", "transfer-0")
	guarded := p.GuardAgainstInflight("token-a", remote)
	if !reflect.DeepEqual(chainIDs(guarded), chainIDs(remote)) {
		t.Errorf("guard applied %v, want the whole chain %v", chainIDs(guarded), chainIDs(remote))
	}

	// And a real verdict is still a verdict: no pending producer, no recent trim,
	// so the retry ladder must break and the transaction must dead-letter.
	verdict := classify(errValidationFailed, errors.New("signature verification failed"))
	if got := p.deferVerdictWhileDependencyPending(consumer, verdict); !errors.Is(got, errValidationFailed) {
		t.Errorf("a genuine verdict was deferred: %v — invalid transactions would never dead-letter", got)
	}
}
