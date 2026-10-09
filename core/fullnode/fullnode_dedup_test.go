package fullnode

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// newTestProcessor builds a processor without the worker pool, scaling loop or
// resource monitor, which no unit test reaches. The caller must cancel the
// returned context.
func newTestProcessor(queueCap int, enqueueTimeout time.Duration) (*DynamicTxnProcessor, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &DynamicTxnProcessor{
		host:           newTestHost(),
		txnQueue:       make(chan *models.EventTransaction, queueCap),
		ctx:            ctx,
		cancel:         cancel,
		queueThreshold: 100,
		enqueueTimeout: enqueueTimeout,
		inflight:       newInflightRegistry(),
		queued:         newQueuedSet(),
		truncated:      newTruncationLog(),
		// Production defaults: the readiness gate is always on, and a zero
		// bundleConfig would make every wait expire at once.
		bundle:   defaultBundleConfig(),
		syncMemo: newSyncedTokenMemo(defaultBundleConfig().syncMemoTTL),
	}
	return p, cancel
}

func testEvent(txnID string) *models.EventTransaction {
	return &models.EventTransaction{
		TransactionID: txnID,
		Status:        true,
		Transaction:   &models.Transactions{ID: txnID},
	}
}

func TestAdmitFirstCallWins(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	if !p.admit("txn-1") {
		t.Fatal("first admit() returned false, want true")
	}
	if p.admit("txn-1") {
		t.Error("second admit() of the same ID returned true, want false")
	}
}

func TestAdmitDistinctIDsAllSucceed(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	for _, id := range []string{"txn-1", "txn-2", "txn-3"} {
		if !p.admit(id) {
			t.Errorf("admit(%q) returned false, want true", id)
		}
	}
}

// Admission must be one atomic check-and-set: pubsub runs each delivery on its
// own goroutine (types/pubsub.go), so a separate load and store would let
// duplicate deliveries both pass.
func TestAdmitIsAtomicUnderConcurrency(t *testing.T) {
	const goroutines = 100

	p, cancel := newTestProcessor(goroutines, time.Second)
	defer cancel()

	var wins int64
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer done.Done()
			start.Wait() // release all goroutines at once to maximise contention
			if p.admit("txn-contended") {
				atomic.AddInt64(&wins, 1)
			}
		}()
	}

	start.Done()
	done.Wait()

	if got := atomic.LoadInt64(&wins); got != 1 {
		t.Errorf("admit() succeeded %d times for one transaction ID, want exactly 1", got)
	}
}

// dedupMapCleaner expects a time.Time value. Anything else would fail its type
// assertion silently, so entries would never expire and memory would leak.
func TestAdmitStoresTimestampForTTLSweep(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	before := time.Now()
	p.admit("txn-1")
	after := time.Now()

	v, ok := p.processedTxns.Load("txn-1")
	if !ok {
		t.Fatal("admitted transaction is absent from processedTxns")
	}
	ts, ok := v.(time.Time)
	if !ok {
		t.Fatalf("processedTxns value is %T, want time.Time — dedupMapCleaner's assertion would fail", v)
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("stored timestamp %v is outside the admission window [%v, %v]", ts, before, after)
	}
}

func TestQueueFullnodeTransactionEnqueuesOnce(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	p.QueueFullnodeTransaction(testEvent("txn-1"))
	p.QueueFullnodeTransaction(testEvent("txn-1")) // duplicate delivery

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events, want 1", got)
	}
	if got := atomic.LoadInt64(&p.processedTxnCount); got != 1 {
		t.Errorf("processedTxnCount = %d, want 1", got)
	}
}

// Concurrent deliveries of the same transaction must produce exactly one queued
// event, not one per goroutine.
func TestQueueFullnodeTransactionConcurrentDuplicates(t *testing.T) {
	const goroutines = 100

	p, cancel := newTestProcessor(goroutines, time.Second)
	defer cancel()

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer done.Done()
			start.Wait()
			p.QueueFullnodeTransaction(testEvent("txn-contended"))
		}()
	}

	start.Done()
	done.Wait()

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events after %d concurrent deliveries, want 1", got, goroutines)
	}
}

// A queue-full drop must release admission, or a re-delivery would be rejected
// as a duplicate until dedupTTL (10 minutes) expires.
func TestQueueFullnodeTransactionReleasesAdmissionWhenQueueFull(t *testing.T) {
	p, cancel := newTestProcessor(1, 20*time.Millisecond)
	defer cancel()

	p.QueueFullnodeTransaction(testEvent("txn-1")) // fills the single slot
	if got := len(p.txnQueue); got != 1 {
		t.Fatalf("setup: queue holds %d events, want 1", got)
	}

	p.QueueFullnodeTransaction(testEvent("txn-2")) // no room, must time out

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events, want 1 — txn-2 should not have been queued", got)
	}
	if _, stillAdmitted := p.processedTxns.Load("txn-2"); stillAdmitted {
		t.Error("txn-2 is still admitted after a queue-full drop; a re-delivery would be rejected as a duplicate")
	}

	// Drain and confirm the dropped transaction is genuinely retryable.
	<-p.txnQueue
	p.QueueFullnodeTransaction(testEvent("txn-2"))
	if got := len(p.txnQueue); got != 1 {
		t.Errorf("re-delivered txn-2 was not queued; queue holds %d events, want 1", got)
	}
}

// Shutdown also releases admission. The queue is filled first so ctx.Done() is
// the only ready select case; with room left, select would pick randomly.
func TestQueueFullnodeTransactionReleasesAdmissionOnShutdown(t *testing.T) {
	p, cancel := newTestProcessor(1, time.Minute)
	defer cancel()

	p.QueueFullnodeTransaction(testEvent("txn-1")) // fills the single slot
	cancel()

	p.QueueFullnodeTransaction(testEvent("txn-2"))

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events, want 1 — txn-2 should not have been queued", got)
	}
	if _, stillAdmitted := p.processedTxns.Load("txn-2"); stillAdmitted {
		t.Error("txn-2 is still admitted after a shutdown drop")
	}
}

// A released ID must be admissible again, even after a second release, so a
// re-delivery can retry a transaction that failed without a verdict.
func TestReleaseAdmissionIsIdempotent(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	p.admit("txn-1")
	p.releaseAdmission("txn-1")
	p.releaseAdmission("txn-1") // must not panic

	if !p.admit("txn-1") {
		t.Error("admit() after a double release returned false, want true")
	}
}
