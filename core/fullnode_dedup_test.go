package core

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// newTestProcessor builds a DynamicTxnProcessor with only the fields the
// admission and enqueue paths touch. The worker pool, scaling loop and resource
// monitor are all left nil — nothing under test reaches them.
//
// The caller must cancel the returned context.
func newTestProcessor(queueCap int, enqueueTimeout time.Duration) (*DynamicTxnProcessor, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &DynamicTxnProcessor{
		txnQueue:       make(chan *models.EventTransaction, queueCap),
		ctx:            ctx,
		cancel:         cancel,
		queueThreshold: 100,
		enqueueTimeout: enqueueTimeout,
		inflight:       newInflightRegistry(),
		// Production defaults, since the readiness gate is always active and a
		// zero bundleConfig would mean no parked cap at all.
		bundle:   defaultBundleConfig(),
		syncMemo: newSyncedTokenMemo(defaultBundleConfig().syncMemoTTL),
	}
	return p, cancel
}

// newTestCore builds the minimum Core that queueFullnodeTransaction needs. It
// deliberately has no wallet: the point of splitting that method out of
// TxnCallBack is that admission can be exercised without a database.
func newTestCore(p *DynamicTxnProcessor) *Core {
	return &Core{
		fullNode:     true,
		txnProcessor: p,
		log:          logger.New(&logger.LoggerOptions{Output: []io.Writer{io.Discard}}),
	}
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

func TestReleaseAdmissionAllowsReadmission(t *testing.T) {
	p, cancel := newTestProcessor(10, time.Second)
	defer cancel()

	if !p.admit("txn-1") {
		t.Fatal("first admit() returned false, want true")
	}
	p.releaseAdmission("txn-1")
	if !p.admit("txn-1") {
		t.Error("admit() after releaseAdmission() returned false, want true")
	}
}

// The regression this commit exists for. Before the change, admission was a
// Load followed by a later Store, and pubsub hands every message to its own
// goroutine (types/pubsub.go:164), so concurrent deliveries of one transaction
// could both pass the check.
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

// dedupMapCleaner type-asserts the map value to time.Time
// (fullnode_txn_processor.go, dedupMapCleaner). If admit ever stored something
// else the assertion would fail silently and entries would never expire, turning
// the bounded dedup window into a permanent one and leaking memory.
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
	c := newTestCore(p)

	c.queueFullnodeTransaction(testEvent("txn-1"))
	c.queueFullnodeTransaction(testEvent("txn-1")) // duplicate delivery

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
	c := newTestCore(p)

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer done.Done()
			start.Wait()
			c.queueFullnodeTransaction(testEvent("txn-contended"))
		}()
	}

	start.Done()
	done.Wait()

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events after %d concurrent deliveries, want 1", got, goroutines)
	}
}

// A queue-full drop must not suppress the transaction for the whole dedup TTL.
// This is the branch most easily got wrong: forgetting the release here leaves
// the transaction unprocessable for ten minutes even if pubsub re-delivers it.
func TestQueueFullnodeTransactionReleasesAdmissionWhenQueueFull(t *testing.T) {
	p, cancel := newTestProcessor(1, 20*time.Millisecond)
	defer cancel()
	c := newTestCore(p)

	c.queueFullnodeTransaction(testEvent("txn-1")) // fills the single slot
	if got := len(p.txnQueue); got != 1 {
		t.Fatalf("setup: queue holds %d events, want 1", got)
	}

	c.queueFullnodeTransaction(testEvent("txn-2")) // no room, must time out

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events, want 1 — txn-2 should not have been queued", got)
	}
	if _, stillAdmitted := p.processedTxns.Load("txn-2"); stillAdmitted {
		t.Error("txn-2 is still admitted after a queue-full drop; a re-delivery would be rejected as a duplicate")
	}

	// Drain and confirm the dropped transaction is genuinely retryable.
	<-p.txnQueue
	c.queueFullnodeTransaction(testEvent("txn-2"))
	if got := len(p.txnQueue); got != 1 {
		t.Errorf("re-delivered txn-2 was not queued; queue holds %d events, want 1", got)
	}
}

// Shutdown takes the same release path. The queue is filled first so that
// ctx.Done() is the only ready case — select chooses uniformly among ready
// cases, so leaving room would make this test flaky.
func TestQueueFullnodeTransactionReleasesAdmissionOnShutdown(t *testing.T) {
	p, cancel := newTestProcessor(1, time.Minute)
	defer cancel()
	c := newTestCore(p)

	c.queueFullnodeTransaction(testEvent("txn-1")) // fills the single slot
	cancel()

	c.queueFullnodeTransaction(testEvent("txn-2"))

	if got := len(p.txnQueue); got != 1 {
		t.Errorf("queue holds %d events, want 1 — txn-2 should not have been queued", got)
	}
	if _, stillAdmitted := p.processedTxns.Load("txn-2"); stillAdmitted {
		t.Error("txn-2 is still admitted after a shutdown drop")
	}
}

// processTxnWithRetry releases the admission once retries are exhausted so a
// later re-delivery can try again. That path is not reachable without a wallet,
// but the release primitive it depends on is exercised here.
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
