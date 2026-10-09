package fullnode

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

func newInflightEntry(id string, deps ...string) *inflightTxn {
	return &inflightTxn{id: id, deps: deps, ready: make(chan struct{})}
}

// eventWithDeps builds an event with one RBT token per previous transaction ID,
// the shape registerInflight reads dependencies from.
func eventWithDeps(txnID string, prevTxIDs ...string) *models.EventTransaction {
	info := &models.TransactionInfo{
		Initiator: "did-initiator",
		Tokens:    &models.TransactionTokens{},
	}
	for i, prev := range prevTxIDs {
		info.Tokens.RBT = append(info.Tokens.RBT, &models.TokenInfo{
			TokenID:               txnID + "-token-" + string(rune('a'+i)),
			PreviousTransactionID: prev,
		})
	}
	raw, err := json.Marshal(info)
	if err != nil {
		panic(err)
	}
	return &models.EventTransaction{
		TransactionID: txnID,
		Status:        true,
		Transaction:   &models.Transactions{ID: txnID, Info: raw},
	}
}

func TestRegistryRegisterAndHas(t *testing.T) {
	r := newInflightRegistry()

	if r.has("txn-1") {
		t.Error("has() true for an unregistered ID")
	}
	if !r.register(newInflightEntry("txn-1")) {
		t.Fatal("register() did not register a new ID")
	}
	if !r.has("txn-1") {
		t.Error("has() false immediately after register()")
	}
	if got := r.len(); got != 1 {
		t.Errorf("len() = %d, want 1", got)
	}
}

// A duplicate must be refused: otherwise two workers would both defer
// unregister and the first to finish would erase the other's entry.
func TestRegistryRegisterRejectsDuplicate(t *testing.T) {
	r := newInflightRegistry()
	first := newInflightEntry("txn-1")

	if !r.register(first) {
		t.Fatal("first register() did not register the entry")
	}
	if r.register(newInflightEntry("txn-1")) {
		t.Error("second register() of the same ID succeeded, want it refused")
	}
	if got := r.len(); got != 1 {
		t.Errorf("len() = %d after a rejected duplicate, want 1", got)
	}
	if r.byID["txn-1"] != first {
		t.Error("the duplicate replaced the original entry")
	}
}

func TestRegistryRejectsNilAndEmptyID(t *testing.T) {
	r := newInflightRegistry()

	if r.register(nil) {
		t.Error("register(nil) registered an entry")
	}
	if r.register(newInflightEntry("")) {
		t.Error("register() with an empty ID registered an entry")
	}
	if r.has("") {
		t.Error("has(\"\") returned true")
	}
	if got := r.len(); got != 0 {
		t.Errorf("len() = %d, want 0", got)
	}
}

// The worker defers unregister unconditionally, so it must tolerate absent IDs.
func TestRegistryUnregisterIsSafeAndIdempotent(t *testing.T) {
	r := newInflightRegistry()

	r.unregister("never-registered") // must not panic

	r.register(newInflightEntry("txn-1"))
	r.unregister("txn-1")
	r.unregister("txn-1")

	if r.has("txn-1") {
		t.Error("has() true after unregister()")
	}
	if got := r.len(); got != 0 {
		t.Errorf("len() = %d, want 0", got)
	}
	if !r.register(newInflightEntry("txn-1")) {
		t.Error("register() after unregister() failed; the ID should be free again")
	}
}

// Workers and the sync guard use the registry concurrently. Run with -race;
// an unguarded map here would be a fatal concurrent map read/write.
func TestRegistryIsSafeUnderConcurrency(t *testing.T) {
	const workers = 50

	r := newInflightRegistry()
	var wins int64
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(workers * 2)

	for i := 0; i < workers; i++ {
		id := "txn-" + string(rune('a'+i%26)) + string(rune('0'+i/26))

		go func() { // owner: register, then release
			defer done.Done()
			start.Wait()
			if r.register(newInflightEntry(id)) {
				atomic.AddInt64(&wins, 1)
				r.unregister(id)
			}
		}()
		go func() { // reader: concurrent observation
			defer done.Done()
			start.Wait()
			_ = r.has(id)
			_ = r.len()
		}()
	}

	start.Done()
	done.Wait()

	if got := atomic.LoadInt64(&wins); got != workers {
		t.Errorf("%d of %d distinct IDs registered, want all", got, workers)
	}
	if got := r.len(); got != 0 {
		t.Errorf("len() = %d after every owner released, want 0", got)
	}
}

// Contending on one ID must produce exactly one owner, since only the owner may
// unregister.
func TestRegistrySingleOwnerUnderContention(t *testing.T) {
	const goroutines = 100

	r := newInflightRegistry()
	var wins int64
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer done.Done()
			start.Wait()
			if r.register(newInflightEntry("txn-contended")) {
				atomic.AddInt64(&wins, 1)
			}
		}()
	}

	start.Done()
	done.Wait()

	if got := atomic.LoadInt64(&wins); got != 1 {
		t.Errorf("register() succeeded %d times for one ID, want exactly 1", got)
	}
}

func TestRegisterInflightExtractsDependencies(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	entry := p.registerInflight(eventWithDeps("txn-T", "txn-S", "txn-Q"))
	if entry == nil {
		t.Fatal("registerInflight() returned nil for a new transaction")
	}
	if len(entry.deps) != 2 || entry.deps[0] != "txn-S" || entry.deps[1] != "txn-Q" {
		t.Errorf("deps = %v, want [txn-S txn-Q]", entry.deps)
	}
	if !p.inflight.has("txn-T") {
		t.Error("transaction is not registered as in flight")
	}
	if entry.ready == nil {
		t.Error("ready channel was not allocated")
	}
}

// Tokens with no previous transaction ID (genesis) add no dependency.
func TestRegisterInflightGenesisHasNoDependencies(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	entry := p.registerInflight(eventWithDeps("txn-S", "", ""))
	if entry == nil {
		t.Fatal("registerInflight() returned nil")
	}
	if len(entry.deps) != 0 {
		t.Errorf("deps = %v, want none", entry.deps)
	}
	if got := atomic.LoadInt64(&p.depsObserved); got != 0 {
		t.Errorf("depsObserved = %d, want 0", got)
	}
}

// An unparseable payload is still being processed, so it must still be
// registered (with no dependencies).
func TestRegisterInflightRegistersUnparseablePayload(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	entry := p.registerInflight(&models.EventTransaction{
		TransactionID: "txn-bad",
		Transaction:   &models.Transactions{ID: "txn-bad", Info: json.RawMessage(`{not json`)},
	})
	if entry == nil {
		t.Fatal("registerInflight() returned nil for an unparseable payload")
	}
	if len(entry.deps) != 0 {
		t.Errorf("deps = %v, want none", entry.deps)
	}
	if !p.inflight.has("txn-bad") {
		t.Error("unparseable transaction is not registered as in flight")
	}
}

func TestRegisterInflightNilTransaction(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	entry := p.registerInflight(&models.EventTransaction{TransactionID: "txn-nil"})
	if entry == nil {
		t.Fatal("registerInflight() returned nil")
	}
	if !p.inflight.has("txn-nil") {
		t.Error("transaction with a nil payload is not registered as in flight")
	}
}

// nil for an already-registered ID stops the second caller from deferring an
// unregister that would erase the first caller's entry.
func TestRegisterInflightReturnsNilForDuplicate(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	if entry := p.registerInflight(eventWithDeps("txn-1")); entry == nil {
		t.Fatal("first registerInflight() returned nil")
	}
	if entry := p.registerInflight(eventWithDeps("txn-1")); entry != nil {
		t.Error("duplicate registerInflight() returned an entry, want nil")
	}
	if got := p.inflight.len(); got != 1 {
		t.Errorf("len() = %d, want 1", got)
	}
}

// The entry must be released on every path, including failure: a leaked entry
// makes the sync guard trim every chain sync of that token.
// A nil payload fails in processSingleTransaction before touching the wallet,
// and that error is not a validation verdict, so nothing is stored as invalid
// and the full retry path runs with no database.
func TestProcessTxnWithRetryReleasesInflightEntry(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	p.maxRetries = 2
	p.retryDelay = 0

	p.processTxnWithRetry(&models.EventTransaction{TransactionID: "txn-1"}, 0)

	if p.inflight.has("txn-1") {
		t.Error("in-flight entry survived processTxnWithRetry")
	}
	if got := p.inflight.len(); got != 0 {
		t.Errorf("len() = %d after processing, want 0", got)
	}
}

// A nil event returns before registration, so nothing should be tracked.
func TestProcessTxnWithRetryNilEventRegistersNothing(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	p.processTxnWithRetry(nil, 0)

	if got := p.inflight.len(); got != 0 {
		t.Errorf("len() = %d, want 0", got)
	}
}

// depsInFlight counts how often a transaction arrives while a previous
// transaction it names is still being processed.
func TestRegisterInflightCountsDependencyEdges(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	// The previous transaction is still in flight.
	if entry := p.registerInflight(eventWithDeps("txn-S")); entry == nil {
		t.Fatal("registering the producer returned nil")
	}
	// The current transaction names it, plus one that is not in flight.
	if entry := p.registerInflight(eventWithDeps("txn-T", "txn-S", "txn-absent")); entry == nil {
		t.Fatal("registering the consumer returned nil")
	}

	if got := atomic.LoadInt64(&p.depsObserved); got != 2 {
		t.Errorf("depsObserved = %d, want 2", got)
	}
	if got := atomic.LoadInt64(&p.depsInFlight); got != 1 {
		t.Errorf("depsInFlight = %d, want 1 (only txn-S was in flight)", got)
	}
}
