package fullnode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests for failure classification and downstream propagation.
//
// Almost all of these are about restraint. The walk reaches forwards and must
// reach nothing else; the verdict is this node's own and must not be inferred
// from a peer being unreachable. Over-propagation is the dangerous direction —
// one misclassified transient failure would dead-letter a whole bundle of good
// transactions — so the tests that matter most are the ones asserting that
// nothing happened.

// The audit trail was selected by strings.Contains on this text before typed
// errors existed, and log tooling outside this repository may still match on it.
// The switch must not have altered a single byte.
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

// Validation reaches out to peers to fill chain gaps, so an unreachable peer
// arrives at the caller as a validation failure. Treating that as a verdict is
// the single most damaging mistake this classification could make.
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

// The linear case: a split is found invalid and the transfer that spends it can
// never succeed.
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

// The safety property. The walk follows waitingOn forwards, so it can only ever
// reach transactions that declared a dependency on something downstream of the
// failure — never a producer of it, never a sibling that merely shares a bundle.
func TestFailWaitersLeavesProducersAndSiblingsAlone(t *testing.T) {
	r := newInflightRegistry()

	// The failing transaction has a producer of its own, and a sibling parked on
	// an entirely different producer.
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

// A malformed graph must come out as a bounded walk rather than a hang or a
// blown stack. park refuses to create a cycle, so the edges here are planted
// directly to exercise the walk's own defence.
//
// The origin closes the loop, and the visited set is what stops the walk turning
// back onto it: a transaction is never failed by its own failure.
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

// An entry that already carries a failure is left exactly as it is: its failure
// may already have been read by a waiter woken through the channel, and writing
// again would be a write racing that read.
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

// A held transaction whose producer is found invalid stops waiting at once and
// reports why, rather than sitting out its timer and then validating against a
// chain that will never exist.
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

// A producer that persists must not look like one that failed.
func TestAwaitDependenciesReleaseIsNotAFailure(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.releaseWaiters("txn-S")
	}()

	if err := p.awaitDependencies(consumer); err != nil {
		t.Errorf("awaitDependencies() = %v, want nil for a producer that persisted", err)
	}
}

// The whole point of Q6: a transient failure must not propagate. Nothing is
// recorded against the consumers, and they keep their own retries.
func TestFailDownstreamIsNotCalledForATransientFailure(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	if !p.inflight.park(consumer, "txn-S") {
		t.Fatal("park() returned false")
	}

	// The predicate the retry path gates propagation on, applied to a transient
	// failure exactly as processTxnWithRetry applies it.
	transient := classify(errDependencyTimeout, errors.New("peer unreachable"))
	if errors.Is(transient, errValidationFailed) {
		p.failDownstream("txn-S", transient)
		t.Error("a transient failure matched the verdict class and propagated")
	}

	if got := p.inflight.failureOf(consumer); got != nil {
		t.Errorf("the consumer was failed by a transient producer failure: %v", got)
	}
	if got := len(p.inflight.waitersOf("txn-S")); got != 1 {
		t.Errorf("waitersOf(txn-S) has %d entries, want the consumer still parked", got)
	}
}

func TestFailDownstreamToleratesNoProcessor(t *testing.T) {
	(*DynamicTxnProcessor)(nil).failDownstream("txn-S", errors.New("boom"))
}

// Fig. 3b in miniature, through the real entry points: the quorum split is found
// invalid, the transfer inherits that and never reaches validation, and the
// initiator's split — which produced nothing that failed — is untouched.
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

// Fifty producers failing while their consumers are parking on them. The walk
// mutates entries the waiters read back, so run with -race.
func TestPropagationIsSafeUnderConcurrency(t *testing.T) {
	const pairs = 50

	p, cancel := cascadeCore(t)
	defer cancel()
	p.bundle.maxParked = pairs * 2
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
