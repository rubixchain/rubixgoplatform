package fullnode

import (
	"reflect"
	"testing"
	"time"
)

// Tests for the stale-entry sweep and the bundle drain.

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
