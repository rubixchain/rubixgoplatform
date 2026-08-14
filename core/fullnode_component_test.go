package core

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// Tests for the union-find forest that turns the cascade's pairwise edges into
// bundles.
//
// Two properties carry the whole thing. Membership must not depend on arrival
// order, since a bundle identity that changed with delivery timing would not be
// an identity. And the forest must empty itself, since it is the one map here
// with no per-entry removal point: an ID that never arrives is only ever a name
// in it.

// bundleEvents returns a three-member bundle: two splits and the transfer that
// spends both. This is the shape a transfer with one quorum pledge produces.
func bundleEvents() []string {
	return []string{"txn-S", "txn-Q", "txn-T"}
}

// registerBundle registers the three members in the given order and returns the
// core so the caller can inspect the forest.
func registerBundle(t *testing.T, order []string) (*Core, *DynamicTxnProcessor) {
	t.Helper()
	p, cancel := newTestProcessor(10, 0)
	t.Cleanup(cancel)
	c := newTestCore(p)

	for _, id := range order {
		event := eventWithDeps(id)
		if id == "txn-T" {
			// Only the transfer declares edges; the splits are genesis legs.
			event = eventWithDeps("txn-T", "txn-S", "txn-Q")
		}
		if entry := c.registerInflight(event); entry == nil {
			t.Fatalf("registerInflight(%s) returned nil", id)
		}
	}
	return c, p
}

// permutations returns every ordering of ids.
func permutations(ids []string) [][]string {
	if len(ids) <= 1 {
		return [][]string{append([]string(nil), ids...)}
	}
	var out [][]string
	for i := range ids {
		rest := make([]string, 0, len(ids)-1)
		rest = append(rest, ids[:i]...)
		rest = append(rest, ids[i+1:]...)
		for _, tail := range permutations(rest) {
			out = append(out, append([]string{ids[i]}, tail...))
		}
	}
	return out
}

// The property that matters: the bundle is the same set however its members
// arrive. Only the transfer declares the edges, so in half of these orders the
// splits are already registered when the edges appear and in the other half they
// are not.
func TestComponentIsIndependentOfArrivalOrder(t *testing.T) {
	want := []string{"txn-Q", "txn-S", "txn-T"} // sorted

	for _, order := range permutations(bundleEvents()) {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			_, p := registerBundle(t, order)

			for _, member := range bundleEvents() {
				got := p.inflight.componentMembers(member)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("componentMembers(%s) = %v, want %v", member, got, want)
				}
			}
		})
	}
}

// The forest must empty itself. Nothing else in the process would notice if it
// did not: the map is not consulted on any hot path and its growth would show up
// only as memory.
func TestComponentPrunedWhenBundleDrains(t *testing.T) {
	_, p := registerBundle(t, bundleEvents())

	if got := p.inflight.componentLen(); got != 3 {
		t.Fatalf("componentLen() = %d after registering a 3-member bundle, want 3", got)
	}

	for _, id := range bundleEvents() {
		p.inflight.unregister(id)
	}

	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d after the bundle drained, want 0", got)
	}
	if got := p.inflight.componentMembers("txn-T"); got != nil {
		t.Errorf("componentMembers(txn-T) = %v after the drain, want nil", got)
	}
}

// A component is dropped whole and only once nothing in it is left. Dropping it
// while a member is still working would lose the identity that member's own
// bundle-scoped work depends on.
func TestComponentSurvivesWhileAnyMemberIsInFlight(t *testing.T) {
	_, p := registerBundle(t, bundleEvents())

	p.inflight.unregister("txn-S")
	p.inflight.unregister("txn-Q")

	if got := p.inflight.componentLen(); got != 3 {
		t.Errorf("componentLen() = %d while the transfer is still in flight, want 3", got)
	}
	if got := len(p.inflight.componentMembers("txn-T")); got != 3 {
		t.Errorf("componentMembers(txn-T) has %d members, want the full bundle of 3", got)
	}

	p.inflight.unregister("txn-T")

	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d once the last member left, want 0", got)
	}
}

// A producer that is named but never arrives is only ever a name in the forest.
// It has no entry in byID to be removed, so if the drain check waited for one it
// would never fire.
func TestComponentPrunedWhenNamedProducerNeverArrives(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	if entry := c.registerInflight(eventWithDeps("txn-T", "txn-never")); entry == nil {
		t.Fatal("registerInflight() returned nil")
	}
	if got := p.inflight.componentLen(); got != 2 {
		t.Fatalf("componentLen() = %d, want 2 — the absent producer is still a member", got)
	}

	p.inflight.unregister("txn-T")

	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d, want 0 — the absent producer must go with its bundle", got)
	}
}

// The overwhelming majority of transactions relate to nothing, and must not pay
// for the forest or leave anything in it.
func TestComponentNotCreatedForUnrelatedTransaction(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	if entry := c.registerInflight(eventWithDeps("txn-alone")); entry == nil {
		t.Fatal("registerInflight() returned nil")
	}

	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d for a transaction with no relations, want 0", got)
	}
	if got := p.inflight.componentMembers("txn-alone"); got != nil {
		t.Errorf("componentMembers(txn-alone) = %v, want nil", got)
	}
}

// Two bundles that share no member stay two bundles. Merging them would hand the
// per-bundle work downstream a scope wider than the thing it is scoping.
func TestComponentsStaySeparate(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	c.registerInflight(eventWithDeps("txn-T1", "txn-S1"))
	c.registerInflight(eventWithDeps("txn-T2", "txn-S2"))

	first := p.inflight.componentMembers("txn-T1")
	second := p.inflight.componentMembers("txn-T2")

	if !reflect.DeepEqual(first, []string{"txn-S1", "txn-T1"}) {
		t.Errorf("componentMembers(txn-T1) = %v, want [txn-S1 txn-T1]", first)
	}
	if !reflect.DeepEqual(second, []string{"txn-S2", "txn-T2"}) {
		t.Errorf("componentMembers(txn-T2) = %v, want [txn-S2 txn-T2]", second)
	}
}

// Transactions that share a producer are one bundle, and draining one of them
// must not take the other's identity with it.
func TestComponentsMergeThroughASharedProducer(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	c.registerInflight(eventWithDeps("txn-T1", "txn-S"))
	c.registerInflight(eventWithDeps("txn-T2", "txn-S"))

	want := []string{"txn-S", "txn-T1", "txn-T2"}
	if got := p.inflight.componentMembers("txn-T1"); !reflect.DeepEqual(got, want) {
		t.Errorf("componentMembers(txn-T1) = %v, want %v", got, want)
	}

	p.inflight.unregister("txn-T1")
	if got := p.inflight.componentMembers("txn-T2"); !reflect.DeepEqual(got, want) {
		t.Errorf("componentMembers(txn-T2) = %v after its sibling drained, want %v", got, want)
	}
}

// A chain of merges must collapse to one component, and path compression must
// not lose a member on the way.
func TestComponentChainCollapses(t *testing.T) {
	r := newInflightRegistry()

	const length = 20
	for i := 1; i < length; i++ {
		r.linkComponent(fmt.Sprintf("txn-%02d", i), []string{fmt.Sprintf("txn-%02d", i-1)})
	}

	members := r.componentMembers("txn-00")
	if len(members) != length {
		t.Fatalf("componentMembers() has %d members, want %d", len(members), length)
	}
	for i, got := range members {
		if want := fmt.Sprintf("txn-%02d", i); got != want {
			t.Errorf("member %d = %s, want %s", i, got, want)
		}
	}
	if got := r.componentMembers(fmt.Sprintf("txn-%02d", length-1)); len(got) != length {
		t.Errorf("the far end of the chain sees %d members, want %d", len(got), length)
	}
}

// The caller gets a copy. Handing out the registry's own slice would let a
// caller reading a bundle corrupt it for everybody else.
func TestComponentMembersReturnsACopy(t *testing.T) {
	r := newInflightRegistry()
	r.linkComponent("txn-T", []string{"txn-S"})

	members := r.componentMembers("txn-T")
	members[0] = "tampered"

	if got := r.componentMembers("txn-T"); got[0] != "txn-S" {
		t.Errorf("componentMembers() = %v after the caller wrote to an earlier result", got)
	}
}

func TestLinkComponentIgnoresDegenerateInput(t *testing.T) {
	r := newInflightRegistry()

	r.linkComponent("", []string{"txn-S"})
	r.linkComponent("txn-T", nil)
	r.linkComponent("txn-T", []string{""})
	r.linkComponent("txn-T", []string{"txn-T"})

	if got := r.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d, want 0 — none of these relate two transactions", got)
	}
	if got := r.componentMembers(""); got != nil {
		t.Errorf("componentMembers(\"\") = %v, want nil", got)
	}
}

// The forest is merged by the pubsub callback goroutines and read while workers
// are draining. Path compression writes on what looks like a read, so a query
// racing a merge is a write racing a write. Run with -race.
func TestComponentsAreSafeUnderConcurrency(t *testing.T) {
	const bundles = 50

	r := newInflightRegistry()
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(bundles * 2)

	for i := 0; i < bundles; i++ {
		transfer := fmt.Sprintf("txn-T%02d", i)
		split := fmt.Sprintf("txn-S%02d", i)

		go func() { // arrival: merge the bundle
			defer done.Done()
			start.Wait()
			r.linkComponent(transfer, []string{split})
		}()
		go func() { // observer: query it, compressing paths as it goes
			defer done.Done()
			start.Wait()
			_ = r.componentMembers(transfer)
			_ = r.componentLen()
		}()
	}

	start.Done()
	done.Wait()

	if got := r.componentLen(); got != bundles*2 {
		t.Errorf("componentLen() = %d, want %d — every bundle contributes two members", got, bundles*2)
	}
	for i := 0; i < bundles; i++ {
		transfer := fmt.Sprintf("txn-T%02d", i)
		if got := len(r.componentMembers(transfer)); got != 2 {
			t.Errorf("componentMembers(%s) has %d members, want 2", transfer, got)
		}
	}
}

// The whole pipeline: a bundle registers, parks, cascades and drains, and leaves
// nothing behind in any of the four maps.
func TestComponentDrainsAfterAFullCascade(t *testing.T) {
	c, p, cancel := cascadeCore(t)
	defer cancel()

	producer := c.registerInflight(eventWithDeps("txn-S"))
	consumer := c.registerInflight(eventWithDeps("txn-T", "txn-S"))
	if producer == nil || consumer == nil {
		t.Fatal("registerInflight() returned nil")
	}

	go func() {
		c.releaseWaiters("txn-S")
		p.inflight.unregister("txn-S")
	}()

	if err := c.awaitDependencies(consumer); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	p.inflight.unregister("txn-T")

	if got := p.inflight.len(); got != 0 {
		t.Errorf("byID holds %d entries, want 0", got)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers, want 0", got)
	}
	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d, want 0", got)
	}
}
