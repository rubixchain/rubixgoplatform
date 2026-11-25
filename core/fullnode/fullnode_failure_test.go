package fullnode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for verdict-vs-transient classification and failure propagation.
// Over-propagation is the dangerous direction: one transient failure mistaken
// for a verdict would dead-letter a bundle of valid transactions, so most tests
// assert that nothing else was touched.

// Audit rows and external log tooling match on this text, so classifying must
// keep the message byte for byte and keep the original cause reachable.
func TestValidationFailureMessageIsUnchanged(t *testing.T) {
	const substring = "failed to validate transaction"

	inner := errors.New("ValidateTransaction: signature verification failed")
	plain := fmt.Errorf("processSingleTransaction: failed to validate transaction: %w", inner)
	classified := classify(errValidationFailed, plain)

	if classified.Error() != plain.Error() {
		t.Errorf("classify() changed the message:\n got %q\nwant %q", classified.Error(), plain.Error())
	}
	if !strings.Contains(classified.Error(), substring) {
		t.Errorf("message %q no longer contains %q", classified.Error(), substring)
	}
	if !errors.Is(classified, errValidationFailed) {
		t.Error("errors.Is does not find the class")
	}
	if !errors.Is(classified, inner) {
		t.Error("errors.Is no longer finds the original cause; the chain was broken")
	}
}

// Validation contacts peers, so an unreachable peer surfaces as a validation
// failure. It must classify as transient, never as a verdict.
func TestClassifyValidationFailureSeparatesTransientFromVerdict(t *testing.T) {
	peerDown := classify(errDependencyTimeout, errors.New("SyncTransactionChainsFromPeer: request failed"))
	throughValidation := fmt.Errorf("ValidateTransaction: %w",
		fmt.Errorf("TokenChainIntigrityCheck: sync failed from peer-1: %w", peerDown))

	if got := classifyValidationFailure(throughValidation); got != errDependencyTimeout {
		t.Errorf("classifyValidationFailure() = %v for an unreachable peer, want %v", got, errDependencyTimeout)
	}

	verdict := fmt.Errorf("ValidateTransaction: %w", errors.New("signature verification failed"))
	if got := classifyValidationFailure(verdict); got != errValidationFailed {
		t.Errorf("classifyValidationFailure() = %v for a signature failure, want %v", got, errValidationFailed)
	}
}

func TestClassifyLeavesNilAlone(t *testing.T) {
	if err := classify(errValidationFailed, nil); err != nil {
		t.Errorf("classify(_, nil) = %v, want nil", err)
	}
}

// A failure propagates down a chain: every transitive waiter of the invalid
// transaction is failed.
func TestFailWaitersPropagatesAlongAChain(t *testing.T) {
	r := newInflightRegistry()
	middle := newInflightEntry("txn-B", "txn-A")
	last := newInflightEntry("txn-C", "txn-B")
	r.park(middle, "txn-A")
	r.park(last, "txn-B")

	cause := errors.New("txn-A is invalid")
	failed := r.failWaiters("txn-A", cause)

	if len(failed) != 2 {
		t.Fatalf("failWaiters() failed %d transactions, want the whole chain of 2", len(failed))
	}
	for _, w := range []*inflightTxn{middle, last} {
		if got := r.failureOf(w); !errors.Is(got, cause) {
			t.Errorf("failureOf(%s) = %v, want %v", w.id, got, cause)
		}
	}
	if got := r.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after the walk, want 0", got)
	}
}

// The walk only follows waiters forwards, so it never touches the failed
// transaction's own previous transaction or an unrelated sibling.
func TestFailWaitersLeavesProducersAndSiblingsAlone(t *testing.T) {
	r := newInflightRegistry()

	// txn-B waits on its own previous transaction; txn-S waits on an unrelated
	// one.
	failing := newInflightEntry("txn-B", "txn-A")
	sibling := newInflightEntry("txn-S", "txn-other")
	consumer := newInflightEntry("txn-C", "txn-B")
	r.park(failing, "txn-A")
	r.park(sibling, "txn-other")
	r.park(consumer, "txn-B")

	failed := r.failWaiters("txn-B", errors.New("txn-B is invalid"))

	if len(failed) != 1 || failed[0] != consumer {
		t.Fatalf("failWaiters() failed %d transactions, want only the consumer", len(failed))
	}
	if got := r.failureOf(failing); got != nil {
		t.Errorf("the failing transaction's own producer edge was disturbed: %v", got)
	}
	if got := r.failureOf(sibling); got != nil {
		t.Errorf("an unrelated sibling was failed: %v", got)
	}
	if got := r.waitersOf("txn-other"); len(got) != 1 {
		t.Errorf("the sibling's edge was removed; waitersOf(txn-other) = %v", got)
	}
}

// A diamond must fail each member once, not once per path into it.
func TestFailWaitersHandlesADiamond(t *testing.T) {
	r := newInflightRegistry()
	left := newInflightEntry("txn-L", "txn-A")
	right := newInflightEntry("txn-R", "txn-A")
	joiner := newInflightEntry("txn-J", "txn-L", "txn-R")
	r.park(left, "txn-A")
	r.park(right, "txn-A")
	r.park(joiner, "txn-L")
	r.park(joiner, "txn-R")

	failed := r.failWaiters("txn-A", errors.New("txn-A is invalid"))

	if len(failed) != 3 {
		t.Fatalf("failWaiters() failed %d transactions, want 3 with no repeats", len(failed))
	}
	seen := map[string]int{}
	for _, w := range failed {
		seen[w.id]++
	}
	if seen["txn-J"] != 1 {
		t.Errorf("the joining transaction was failed %d times, want once", seen["txn-J"])
	}
}

// The walk must terminate on a malformed cycle. park refuses cycles, so the
// edges are planted directly; the visited set stops the walk failing its own
// origin.
func TestFailWaitersTerminatesOnACycle(t *testing.T) {
	r := newInflightRegistry()
	origin := newInflightEntry("txn-A", "txn-B")
	other := newInflightEntry("txn-B", "txn-A")
	r.waitingOn["txn-B"] = []*inflightTxn{origin}
	r.waitingOn["txn-A"] = []*inflightTxn{other}
	origin.pending, other.pending = 1, 1

	done := make(chan []*inflightTxn, 1)
	go func() { done <- r.failWaiters("txn-A", errors.New("boom")) }()

	select {
	case failed := <-done:
		if len(failed) != 1 || failed[0] != other {
			t.Fatalf("failWaiters() failed %d transactions around the cycle, want only txn-B", len(failed))
		}
		if got := r.failureOf(origin); got != nil {
			t.Errorf("the walk turned back onto its own origin and failed it: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("failWaiters() did not terminate on a cycle")
	}
}

// An existing failure is never overwritten: a woken waiter may already have
// read it, and a second write would race that read.
func TestFailWaitersDoesNotOverwriteAnExistingFailure(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-C", "txn-A", "txn-B")
	r.park(consumer, "txn-A")
	r.park(consumer, "txn-B")

	first := errors.New("txn-A is invalid")
	r.failWaiters("txn-A", first)
	r.failWaiters("txn-B", errors.New("txn-B is invalid too"))

	if got := r.failureOf(consumer); !errors.Is(got, first) {
		t.Errorf("failureOf() = %v, want the first cause %v", got, first)
	}
}

func TestFailWaitersIgnoresDegenerateInput(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-C", "txn-A")
	r.park(consumer, "txn-A")

	if got := r.failWaiters("", errors.New("boom")); got != nil {
		t.Errorf("failWaiters(\"\", ...) = %v, want nil", got)
	}
	if got := r.failWaiters("txn-A", nil); got != nil {
		t.Errorf("failWaiters(_, nil) = %v, want nil", got)
	}
	if got := r.failWaiters("txn-unknown", errors.New("boom")); got != nil {
		t.Errorf("failWaiters() on a producer with no waiters = %v, want nil", got)
	}
	if got := r.failureOf(consumer); got != nil {
		t.Errorf("the parked consumer was failed by a degenerate call: %v", got)
	}
	if got := r.failureOf(nil); got != nil {
		t.Errorf("failureOf(nil) = %v, want nil", got)
	}
}

// A waiting transaction whose previous transaction is found invalid stops
// waiting at once and returns that failure, instead of waiting out its timer.
func TestAwaitDependenciesReturnsTheProducerFailure(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	cause := errors.New("processSingleTransaction: failed to validate transaction: bad signature")
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.failDownstream("txn-S", classify(errValidationFailed, cause))
	}()

	start := time.Now()
	err := p.awaitDependencies(consumer)
	if !errors.Is(err, errProducerFailed) {
		t.Fatalf("awaitDependencies() = %v, want it to report %v", err, errProducerFailed)
	}
	if !errors.Is(err, cause) {
		t.Errorf("awaitDependencies() = %v, want the producer's own cause to survive", err)
	}
	if elapsed := time.Since(start); elapsed >= p.bundle.unknownWait {
		t.Errorf("waited %v, i.e. the full timer; a failed producer should end the wait", elapsed)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after the walk, want 0", got)
	}
}

// A non-verdict failure (the test host cannot initialise the DID) must not
// propagate: the waiter stays parked with no failure and the previous
// transaction's admission is released for re-delivery. Treating it as a verdict
// would also call storeInvalidTransaction, and the test host's Wallet() panics.
func TestProcessTxnWithRetryDoesNotPropagateANonVerdict(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()
	p.maxRetries = 2
	p.retryDelay = 0

	consumer := newInflightEntry("txn-T", "txn-S")
	if !p.inflight.park(consumer, "txn-S") {
		t.Fatal("park() returned false")
	}

	producer := eventWithDeps("txn-S")
	if !p.admit(producer.TransactionID) {
		t.Fatal("admit() returned false for a fresh producer")
	}
	p.processTxnWithRetry(producer, 0)

	if got := p.inflight.failureOf(consumer); got != nil {
		t.Errorf("the consumer was failed by a non-verdict producer failure: %v", got)
	}
	if got := p.inflight.waitersOf("txn-S"); len(got) != 1 || got[0] != "txn-T" {
		t.Errorf("waitersOf(txn-S) = %v, want the consumer still parked", got)
	}
	if got := atomic.LoadInt64(&p.failuresPropagated); got != 0 {
		t.Errorf("failuresPropagated = %d, want 0", got)
	}
	if !p.admit(producer.TransactionID) {
		t.Error("the producer's admission was not released after a non-verdict failure")
	}
}

func TestFailDownstreamToleratesNoProcessor(t *testing.T) {
	(*DynamicTxnProcessor)(nil).failDownstream("txn-S", errors.New("boom"))
}

// A quorum split found invalid fails the transfer waiting on it, while the
// initiator's split, which is not downstream of the failure, is untouched.
func TestPropagationReachesOnlyDownstream(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	initiatorSplit := p.registerInflight(eventWithDeps("txn-S1"))
	transfer := newInflightEntry("txn-T", "txn-S2")
	if initiatorSplit == nil {
		t.Fatal("registerInflight() returned nil")
	}
	if !p.inflight.park(transfer, "txn-S2") {
		t.Fatal("park() returned false")
	}

	verdict := classify(errValidationFailed,
		errors.New("processSingleTransaction: failed to validate transaction: quorum split invalid"))
	p.failDownstream("txn-S2", verdict)

	if got := p.inflight.failureOf(transfer); !errors.Is(got, errValidationFailed) {
		t.Errorf("the transfer was not failed: %v", got)
	}
	if got := p.inflight.failureOf(initiatorSplit); got != nil {
		t.Errorf("the initiator's split was failed, and it is not downstream of anything: %v", got)
	}
	if !p.inflight.has("txn-S1") {
		t.Error("the initiator's split was removed from the pipeline")
	}
	if got := p.failuresPropagated; got != 1 {
		t.Errorf("failuresPropagated = %d, want 1", got)
	}
}

// Fifty previous transactions fail while their waiters park. The walk writes
// entries the waiters read back, so run with -race.
func TestPropagationIsSafeUnderConcurrency(t *testing.T) {
	const pairs = 50

	p, cancel := cascadeCore(t)
	defer cancel()
	p.bundle.unknownWait = 500 * time.Millisecond
	p.bundle.inflightWait = 500 * time.Millisecond

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(pairs * 2)

	for i := 0; i < pairs; i++ {
		producerID := fmt.Sprintf("txn-P%02d", i)
		consumerID := fmt.Sprintf("txn-C%02d", i)

		go func() {
			defer done.Done()
			start.Wait()
			err := p.awaitDependencies(newInflightEntry(consumerID, producerID))
			if err != nil && !errors.Is(err, errProducerFailed) {
				t.Errorf("awaitDependencies(%s) = %v, want nil or a producer failure", consumerID, err)
			}
		}()
		go func() {
			defer done.Done()
			start.Wait()
			p.failDownstream(producerID, classify(errValidationFailed, errors.New("invalid")))
		}()
	}

	start.Done()
	done.Wait()

	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after every pair finished, want 0", got)
	}
}

// A chain mismatch reached while a previous transaction is still pending comes
// from GuardAgainstInflight trimming the sync, not from an invalid transaction.
// It must be re-tagged transient so the retries run.
func TestDeferVerdictWhileProducerInFlight(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	producer := &inflightTxn{id: "split-1", ready: make(chan struct{})}
	if !p.inflight.register(producer) {
		t.Fatal("register(producer) = false, want true")
	}
	consumer := &inflightTxn{id: "transfer-1", deps: []string{"split-1"}, ready: make(chan struct{})}

	verdict := classify(errValidationFailed,
		fmt.Errorf("processSingleTransaction: failed to validate transaction: %w",
			errors.New("TokenChainIntigrityCheck: token X (rbt) chain mismatch after sync from peer-1")))

	got := p.deferVerdictWhileDependencyPending(consumer, verdict)
	if errors.Is(got, errValidationFailed) {
		t.Error("the verdict survived; it would still break the retry ladder on attempt 1")
	}
	if !errors.Is(got, errDependencyTimeout) {
		t.Error("the deferred failure is not transient, so the retry ladder will not run")
	}
	if got.Error() != verdict.Error() {
		t.Errorf("the message changed:\n got %q\nwant %q", got.Error(), verdict.Error())
	}
	if n := atomic.LoadInt64(&p.verdictsDeferred); n != 1 {
		t.Errorf("verdictsDeferred = %d, want 1", n)
	}
}

// Without a pending previous transaction or a trimmed sync, a verdict must stand,
// or an invalid transaction would never be dead-lettered.
func TestDeferVerdictLeavesEverythingElseAlone(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	verdict := classify(errValidationFailed, errors.New("signature verification failed"))

	cases := []struct {
		name  string
		entry *inflightTxn
		err   error
	}{
		{"no producer declared", &inflightTxn{id: "txn-1"}, verdict},
		{"producer already resolved", &inflightTxn{id: "txn-1", deps: []string{"gone"}}, verdict},
		{"untracked transaction", nil, verdict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.deferVerdictWhileDependencyPending(tc.entry, tc.err); !errors.Is(got, errValidationFailed) {
				t.Errorf("the verdict was deferred, want it kept: %v", got)
			}
		})
	}

	if got := p.deferVerdictWhileDependencyPending(&inflightTxn{id: "txn-1"}, nil); got != nil {
		t.Errorf("a success was turned into %v, want nil", got)
	}
	if n := atomic.LoadInt64(&p.verdictsDeferred); n != 0 {
		t.Errorf("verdictsDeferred = %d, want 0", n)
	}
}

// Re-tagging cannot be done by wrapping: errors.Is walks both branches, so the
// old class would still match and the deferral would silently do nothing.
func TestStripClassRemovesOnlyTheOutermostClass(t *testing.T) {
	peerDown := classify(errDependencyTimeout, errors.New("peer unreachable"))
	outer := classify(errValidationFailed, fmt.Errorf("validation: %w", peerDown))

	stripped := stripClass(outer)
	if errors.Is(stripped, errValidationFailed) {
		t.Error("the outermost class survived stripClass")
	}
	if !errors.Is(stripped, errDependencyTimeout) {
		t.Error("stripClass removed a class further down the chain, which is still true")
	}

	plain := errors.New("never classified")
	if stripClass(plain) != plain {
		t.Error("stripClass altered an unclassified error")
	}
}

// The guard also trims for a queued previous transaction, so the deferral must
// count the queue as pending, or that trim would dead-letter the waiting
// transaction.
func TestDeferVerdictSeesAQueuedProducer(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	// The split is admitted and queued; no worker has reached it.
	p.QueueFullnodeTransaction(testEvent("split-1"))
	if p.inflight.has("split-1") {
		t.Fatal("the split reached the registry; this test needs it queued only")
	}

	consumer := &inflightTxn{id: "transfer-1", deps: []string{"split-1"}, ready: make(chan struct{})}
	verdict := classify(errValidationFailed, errors.New("chain mismatch after sync from peer-1"))

	if got := p.deferVerdictWhileDependencyPending(consumer, verdict); errors.Is(got, errValidationFailed) {
		t.Error("the verdict stood while its producer was still sitting in the queue")
	}
}

// The guard cuts to a prefix, so a pending transaction EARLIER in the chain
// drops the needed entry too. It is not a declared dependency, so only the
// guard's trim note defers the verdict. A catching-up fullnode hits this often.
func TestDeferVerdictOnATrimmedSyncWithNoDeclaredProducerPending(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	// txn-X sits earlier in token-a's chain and happens to be queued here.
	p.QueueFullnodeTransaction(testEvent("txn-X"))

	// The transfer needs split-1, which is NOT pending.
	consumer := p.registerInflight(eventWithDeps("transfer-1", "split-1"))
	if consumer == nil {
		t.Fatal("registerInflight returned nil")
	}
	if p.isPending("split-1") {
		t.Fatal("split-1 is pending; this test needs the declared-producer check to say no")
	}

	verdict := classify(errValidationFailed, errors.New("chain mismatch after sync from peer-1"))

	// Without the guard's note there is nothing to go on, so this is a verdict.
	if got := p.deferVerdictWhileDependencyPending(consumer, verdict); !errors.Is(got, errValidationFailed) {
		t.Fatalf("deferred with no evidence at all: %v", got)
	}

	// The guard truncating the transfer's own token is the evidence.
	// eventWithDeps names it "<txnID>-token-a".
	guarded := p.GuardAgainstInflight("transfer-1-token-a", chain("txn-X", "split-1"))
	if len(guarded) != 0 {
		t.Fatalf("guard applied %v, want nothing", chainIDs(guarded))
	}

	got := p.deferVerdictWhileDependencyPending(consumer, verdict)
	if errors.Is(got, errValidationFailed) {
		t.Error("the verdict stood although the sync that produced it came back trimmed")
	}
	if !errors.Is(got, errDependencyTimeout) {
		t.Error("the deferred failure is not transient, so the retry ladder will not run")
	}
}

// Trim notes must expire and be cleared on persist, or one trim would soften
// every later verdict on that token.
func TestTruncationNotesExpireAndAreClearedOnPersist(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	p.truncated.record("token-a")

	if _, trimmed := p.truncated.recentlyTruncated([]string{"token-a"}, truncationTTL); !trimmed {
		t.Fatal("a fresh note was not found")
	}
	if _, trimmed := p.truncated.recentlyTruncated([]string{"token-a"}, time.Nanosecond); trimmed {
		t.Error("a note older than the window was still acted on")
	}
	if _, trimmed := p.truncated.recentlyTruncated([]string{"token-b"}, truncationTTL); trimmed {
		t.Error("a note was found for a token that was never trimmed")
	}

	// A persist advances the chain, so the gap the note describes is gone.
	p.truncated.forget([]string{"token-a"})
	if _, trimmed := p.truncated.recentlyTruncated([]string{"token-a"}, truncationTTL); trimmed {
		t.Error("the note survived the persist that resolved it")
	}

	p.truncated.record("token-c")
	if dropped := p.truncated.sweep(time.Nanosecond); dropped != 1 {
		t.Errorf("sweep dropped %d notes, want 1", dropped)
	}
	if n := p.truncated.len(); n != 0 {
		t.Errorf("truncated.len() = %d after the sweep, want 0", n)
	}
}
