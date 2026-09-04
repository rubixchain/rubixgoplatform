package fullnode

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Tests for the bounds and the bundle drain.
//
// Every bound here has the same contract: past it the pipeline stops tracking,
// never stops working. A cap that dropped transactions would be a worse failure
// than the growth it prevents, so each test checks both halves — that the bound
// engages, and that the work still happens.

// The drain is the only moment a bundle is complete, so what it reports has to
// be the whole membership — and sorted, since it is the sole record of the
// bundle and arrival order decides nothing about which transactions were in it.
func TestDrainReportsTheWholeMembershipSorted(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	p.registerInflight(eventWithDeps("txn-T", "txn-S", "txn-Q"))
	want := p.inflight.componentMembers("txn-T")

	drained := p.inflight.unregister("txn-T")

	if !reflect.DeepEqual(drained, want) {
		t.Errorf("drained = %v, want the component's membership %v", drained, want)
	}
	if !reflect.DeepEqual(drained, []string{"txn-Q", "txn-S", "txn-T"}) {
		t.Errorf("drained = %v, want it sorted", drained)
	}
}

// Only the last member out drains the bundle, so only one label is emitted per
// bundle however many members it had.
func TestUnregisterReportsTheDrainOnlyOnce(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	p.registerInflight(eventWithDeps("txn-S"))
	p.registerInflight(eventWithDeps("txn-T", "txn-S"))

	if got := p.inflight.unregister("txn-S"); got != nil {
		t.Errorf("unregister(txn-S) reported a drain of %v while txn-T was still in flight", got)
	}
	if got := p.inflight.unregister("txn-T"); len(got) != 2 {
		t.Errorf("unregister(txn-T) reported %v, want the drained bundle of 2", got)
	}
}

// A transaction that relates to nothing has no bundle, so there is nothing to
// name when it leaves.
func TestUnregisterReportsNoDrainWithoutABundle(t *testing.T) {
	r := newInflightRegistry()
	r.register(newInflightEntry("txn-alone"))

	if got := r.unregister("txn-alone"); got != nil {
		t.Errorf("unregister() reported a drain of %v for an unrelated transaction", got)
	}
	if got := r.unregister("txn-never-registered"); got != nil {
		t.Errorf("unregister() of an absent ID reported %v", got)
	}
}

// The bound engages, and past it registration reports why rather than pretending
// to have succeeded.
func TestRegisterRefusesPastTheRegistryCap(t *testing.T) {
	r := newInflightRegistry()
	r.maxEntries = 3

	for i := 0; i < r.maxEntries; i++ {
		if got := r.register(newInflightEntry(fmt.Sprintf("txn-%d", i))); got != registered {
			t.Fatalf("register() %d = %v below the cap, want registered", i, got)
		}
	}

	if got := r.register(newInflightEntry("txn-overflow")); got != registryFull {
		t.Errorf("register() past the cap = %v, want registryFull", got)
	}
	if got := r.len(); got != r.maxEntries {
		t.Errorf("len() = %d, want the cap of %d", got, r.maxEntries)
	}
	if r.has("txn-overflow") {
		t.Error("the refused transaction was registered anyway")
	}

	// And the bound lifts as soon as there is room again.
	r.unregister("txn-0")
	if got := r.register(newInflightEntry("txn-overflow")); got != registered {
		t.Errorf("register() = %v once an entry was released, want registered", got)
	}
}

// A duplicate and a full registry both mean "you do not own an entry", but they
// are different events and must stay distinguishable.
func TestRegistryFullIsDistinctFromDuplicate(t *testing.T) {
	r := newInflightRegistry()
	r.maxEntries = 1
	r.register(newInflightEntry("txn-1"))

	if got := r.register(newInflightEntry("txn-1")); got != alreadyInFlight {
		t.Errorf("re-registering the same ID = %v, want alreadyInFlight", got)
	}
	if got := r.register(newInflightEntry("txn-2")); got != registryFull {
		t.Errorf("registering past the cap = %v, want registryFull", got)
	}
}

// Fail-open is the whole contract: a transaction the registry cannot take is
// still processed, just untracked — which is what every transaction did before
// any of this existed.
func TestRegisterInflightFailsOpenWhenFull(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	p.inflight.maxEntries = 1
	p.maxRetries = 1
	p.retryDelay = 0

	if entry := p.registerInflight(eventWithDeps("txn-first", "txn-S")); entry == nil {
		t.Fatal("the first transaction was not registered")
	}
	if entry := p.registerInflight(eventWithDeps("txn-second", "txn-S")); entry != nil {
		t.Error("registerInflight() returned an entry past the cap; the caller would unregister someone else's")
	}
	if got := p.registryFullEvents; got != 1 {
		t.Errorf("registryFullEvents = %d, want 1", got)
	}

	// The untracked transaction still runs. A nil payload fails inside
	// processSingleTransaction before it reaches the wallet, so the whole path
	// runs with no database.
	p.processTxnWithRetry(&models.EventTransaction{TransactionID: "txn-untracked"}, 0)
	if p.inflight.has("txn-untracked") {
		t.Error("an untracked transaction left an entry behind")
	}
}

// The sweep is a backstop for a bug, so the case that matters most is that it
// leaves working transactions alone.
func TestSweepStaleLeavesRecentEntriesAlone(t *testing.T) {
	r := newInflightRegistry()
	r.register(newInflightEntry("txn-working"))

	if got := r.sweepStale(time.Hour); got != nil {
		t.Errorf("sweepStale() removed %v, want nothing", got)
	}
	if !r.has("txn-working") {
		t.Error("a transaction registered moments ago was swept")
	}
	if got := r.sweepStale(0); got != nil {
		t.Errorf("sweepStale(0) removed %v; a non-positive TTL must sweep nothing", got)
	}
}

// An entry that outlived any plausible amount of work is a leak, and leaving it
// would silently truncate every chain sync for its token.
func TestSweepStaleRemovesLeakedEntries(t *testing.T) {
	r := newInflightRegistry()

	leaked := newInflightEntry("txn-leaked")
	r.register(leaked)
	leaked.registeredAt = time.Now().Add(-2 * time.Hour)
	r.register(newInflightEntry("txn-working"))

	swept := r.sweepStale(time.Hour)

	if !reflect.DeepEqual(swept, []string{"txn-leaked"}) {
		t.Errorf("sweepStale() = %v, want [txn-leaked]", swept)
	}
	if r.has("txn-leaked") {
		t.Error("the leaked entry survived the sweep")
	}
	if !r.has("txn-working") {
		t.Error("the sweep took a healthy entry with it")
	}
	// The sync guard reads this set; a leak that survived here would keep
	// truncating.
	if r.idSet()["txn-leaked"] {
		t.Error("the swept ID is still in the guard set")
	}
}

// A swept producer takes its waiter list with it. The waiters keep their own
// timers, which is the same outcome as a producer that never arrived.
func TestSweepStaleClearsWaitersAndComponents(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	producer := p.registerInflight(eventWithDeps("txn-S"))
	consumer := p.registerInflight(eventWithDeps("txn-T", "txn-S"))
	if producer == nil || consumer == nil {
		t.Fatal("registerInflight() returned nil")
	}
	if !p.inflight.park(consumer, "txn-S") {
		t.Fatal("park() returned false")
	}

	producer.registeredAt = time.Now().Add(-2 * time.Hour)
	consumer.registeredAt = time.Now().Add(-2 * time.Hour)

	if got := len(p.inflight.sweepStale(time.Hour)); got != 2 {
		t.Fatalf("sweepStale() removed %d entries, want 2", got)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after the sweep, want 0", got)
	}
	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d after the sweep, want 0", got)
	}
	if got := p.inflight.len(); got != 0 {
		t.Errorf("len() = %d after the sweep, want 0", got)
	}
}

// The cap is checked under the same lock as the insert. A check-then-act bound
// is one a burst walks straight through, which is the traffic it exists to stop.
func TestRegistryCapHoldsUnderConcurrency(t *testing.T) {
	const goroutines = 200
	const cap = 20

	r := newInflightRegistry()
	r.maxEntries = cap

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		id := fmt.Sprintf("txn-%03d", i)
		go func() {
			defer done.Done()
			start.Wait()
			r.register(newInflightEntry(id))
		}()
	}
	start.Done()
	done.Wait()

	if got := r.len(); got != cap {
		t.Errorf("len() = %d after %d concurrent registrations, want exactly the cap of %d", got, goroutines, cap)
	}
}
