package fullnode

import (
	"testing"
	"time"
)

// Taking a transaction off the queue must clear its queued mark, or the sync
// guard would trim every sync of that token for as long as the node runs.
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

// A transaction whose enqueue timed out must not stay marked as queued.
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

// The pending checks and the sync guard must work on a processor whose queued
// set was never built (nil).
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
