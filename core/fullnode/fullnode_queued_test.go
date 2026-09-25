package fullnode

import (
	"testing"
	"time"
)

// The blind spot this set exists to close: a producer sitting in txnQueue must
// be as visible as one under a worker, or the guard ingests it from a peer
// unvalidated and the readiness gate gives up on it early.
func TestQueuedTransactionsAreVisibleToBothReaders(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	// A split admitted and queued, no worker on it yet.
	p.QueueFullnodeTransaction(testEvent("split-1"))

	if !p.isPending("split-1") {
		t.Fatal("a queued transaction is invisible to isPending")
	}
	if !p.pendingIDSet()["split-1"] {
		t.Error("a queued transaction is missing from pendingIDSet")
	}
	if p.inflight.has("split-1") {
		t.Error("the queued transaction leaked into the in-flight registry")
	}

	// The guard must now refuse to hand that split back from a peer response.
	guarded := p.GuardAgainstInflight("token-X1", chain("split-1", "transfer-1"))
	if len(guarded) != 0 {
		t.Errorf("guard applied %v, want nothing: the split is queued and unvalidated", chainIDs(guarded))
	}

	// And the gate must offer the long tier rather than giving up in 1s.
	if got, want := p.dependencyWait([]string{"split-1"}), p.bundle.inflightWait; got != want {
		t.Errorf("dependencyWait = %v, want %v (the producer is queued, not absent)", got, want)
	}
}

// Dequeue is what ends the queued state. If it did not, a leaked ID would
// truncate every sync of that token for as long as the node runs.
func TestDequeueClearsTheQueuedEntry(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	p.QueueFullnodeTransaction(testEvent("txn-1"))
	if !p.isPending("txn-1") {
		t.Fatal("not marked queued after enqueue")
	}

	// Stand in for dynamicWorker's receive.
	event := <-p.txnQueue
	p.queued.remove(event.TransactionID)

	if p.isPending("txn-1") {
		t.Error("still marked queued after a worker took it off the queue")
	}
	if n := p.queued.len(); n != 0 {
		t.Errorf("queued.len() = %d, want 0", n)
	}
}

// A transaction that never reached a worker must leave nothing behind, or the
// set outlives the queue it shadows.
func TestFailedEnqueueLeavesNothingQueued(t *testing.T) {
	p, cancel := newTestProcessor(1, 20*time.Millisecond)
	defer cancel()

	p.QueueFullnodeTransaction(testEvent("txn-1")) // fills the queue
	p.QueueFullnodeTransaction(testEvent("txn-2")) // times out waiting for room

	if p.isPending("txn-2") {
		t.Error("a transaction that timed out on enqueue is still marked queued")
	}
	if n := p.queued.len(); n != 1 {
		t.Errorf("queued.len() = %d, want 1 (only the one that made it onto the queue)", n)
	}
}

// Both readers must be unaffected on a node that never built the set, since the
// same processor type serves the non-fullnode sync path.
func TestPendingReadsTolerateAnAbsentQueuedSet(t *testing.T) {
	p := &DynamicTxnProcessor{host: newTestHost(), inflight: newInflightRegistry()}

	if p.isPending("txn-1") {
		t.Error("isPending reported true with no queued set")
	}
	if len(p.pendingIDSet()) != 0 {
		t.Error("pendingIDSet returned entries with no queued set")
	}
	txs := chain("txn-1")
	if got := p.GuardAgainstInflight("token-X1", txs); len(got) != len(txs) {
		t.Errorf("guard trimmed %v with nothing in flight", chainIDs(got))
	}
}
