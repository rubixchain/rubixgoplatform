package fullnode

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// Tests for the union-find forest that groups related transactions into
// bundles. The bundle's root keys the sync memo (bundleScope), so membership must
// not depend on arrival order, and the forest must empty itself once a bundle
// drains, including IDs that never arrived.

// bundleEvents names a three-member bundle: two splits and the transfer that
// spends both.
func bundleEvents() []string {
	return []string{"txn-S", "txn-Q", "txn-T"}
}

// registerBundle registers the bundle's members in the given order.
func registerBundle(t *testing.T, order []string) *DynamicTxnProcessor {
	t.Helper()
	p, cancel := newTestProcessor(10, 0)
	t.Cleanup(cancel)

	for _, id := range order {
		event := eventWithDeps(id)
		if id == "txn-T" {
			// Only the transfer declares previous transactions.
			event = eventWithDeps("txn-T", "txn-S", "txn-Q")
		}
		if entry := p.registerInflight(event); entry == nil {
			t.Fatalf("registerInflight(%s) returned nil", id)
		}
	}
	return p
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

// Membership must be the same in every arrival order, whether the splits
// register before or after the transfer that links them.
func TestComponentIsIndependentOfArrivalOrder(t *testing.T) {
	want := []string{"txn-Q", "txn-S", "txn-T"} // sorted

	for _, order := range permutations(bundleEvents()) {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			p := registerBundle(t, order)

			for _, member := range bundleEvents() {
				got := p.inflight.componentMembers(member)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("componentMembers(%s) = %v, want %v", member, got, want)
				}
			}
		})
	}
}

// The forest must empty itself once a bundle drains; otherwise it would only
// show up as growing memory.
func TestComponentPrunedWhenBundleDrains(t *testing.T) {
	p := registerBundle(t, bundleEvents())

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

// A bundle is kept until its last member leaves; dropping it early would change
// the sync-memo key of a member still working.
func TestComponentSurvivesWhileAnyMemberIsInFlight(t *testing.T) {
	p := registerBundle(t, bundleEvents())

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

// A named previous transaction that never arrives has no byID entry, so the
// drain must not wait for it.
func TestComponentPrunedWhenNamedProducerNeverArrives(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	if entry := p.registerInflight(eventWithDeps("txn-T", "txn-never")); entry == nil {
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

// Most transactions relate to nothing and must not enter the forest.
func TestComponentNotCreatedForUnrelatedTransaction(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	if entry := p.registerInflight(eventWithDeps("txn-alone")); entry == nil {
		t.Fatal("registerInflight() returned nil")
	}

	if got := p.inflight.componentLen(); got != 0 {
		t.Errorf("componentLen() = %d for a transaction with no relations, want 0", got)
	}
	if got := p.inflight.componentMembers("txn-alone"); got != nil {
		t.Errorf("componentMembers(txn-alone) = %v, want nil", got)
	}
}

// Bundles that share no member stay separate; merging them would widen the
// sync-memo scope.
func TestComponentsStaySeparate(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	p.registerInflight(eventWithDeps("txn-T1", "txn-S1"))
	p.registerInflight(eventWithDeps("txn-T2", "txn-S2"))

	first := p.inflight.componentMembers("txn-T1")
	second := p.inflight.componentMembers("txn-T2")

	if !reflect.DeepEqual(first, []string{"txn-S1", "txn-T1"}) {
		t.Errorf("componentMembers(txn-T1) = %v, want [txn-S1 txn-T1]", first)
	}
	if !reflect.DeepEqual(second, []string{"txn-S2", "txn-T2"}) {
		t.Errorf("componentMembers(txn-T2) = %v, want [txn-S2 txn-T2]", second)
	}
}

// Transactions sharing a previous transaction are one bundle, and one leaving
// must not change the other's membership.
func TestComponentsMergeThroughASharedProducer(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()

	p.registerInflight(eventWithDeps("txn-T1", "txn-S"))
	p.registerInflight(eventWithDeps("txn-T2", "txn-S"))

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

// componentMembers returns a copy, so a caller cannot corrupt the registry.
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

// Workers link and query the forest concurrently, and path compression writes
// during a query. Run with -race.
func TestComponentsAreSafeUnderConcurrency(t *testing.T) {
	const bundles = 50

	r := newInflightRegistry()
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(bundles * 2)

	for i := 0; i < bundles; i++ {
		transfer := fmt.Sprintf("txn-T%02d", i)
		split := fmt.Sprintf("txn-S%02d", i)

		go func() { // link the bundle
			defer done.Done()
			start.Wait()
			r.linkComponent(transfer, []string{split})
		}()
		go func() { // query it, compressing paths
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

// After register, park, wake-up and drain, byID, waitingOn and the forest must
// all be empty.
func TestComponentDrainsAfterAFullCascade(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	producer := p.registerInflight(eventWithDeps("txn-S"))
	consumer := p.registerInflight(eventWithDeps("txn-T", "txn-S"))
	if producer == nil || consumer == nil {
		t.Fatal("registerInflight() returned nil")
	}

	go func() {
		p.releaseWaiters("txn-S")
		p.inflight.unregister("txn-S")
	}()

	if err := p.awaitDependencies(consumer); err != nil {
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
