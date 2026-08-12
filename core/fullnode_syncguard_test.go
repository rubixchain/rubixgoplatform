package core

import (
	"reflect"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// chain builds a peer chain response from transaction IDs, in position order.
func chain(ids ...string) []types.TransactionWithRole {
	txs := make([]types.TransactionWithRole, 0, len(ids))
	for _, id := range ids {
		txs = append(txs, types.TransactionWithRole{Tx: models.Transactions{ID: id}})
	}
	return txs
}

func chainIDs(txs []types.TransactionWithRole) []string {
	ids := make([]string, 0, len(txs))
	for _, tx := range txs {
		ids = append(ids, tx.Tx.ID)
	}
	return ids
}

func inflightSet(ids ...string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func TestTruncateAtInflight(t *testing.T) {
	tests := []struct {
		name     string
		remote   []string
		inflight []string
		want     []string
	}{
		{
			name:     "nothing in flight leaves the chain untouched",
			remote:   []string{"A", "B", "C"},
			inflight: nil,
			want:     []string{"A", "B", "C"},
		},
		{
			name:     "in-flight entries elsewhere do not match this chain",
			remote:   []string{"A", "B", "C"},
			inflight: []string{"X", "Y"},
			want:     []string{"A", "B", "C"},
		},
		{
			// The common case: the newest entry is the sibling still being
			// validated, so the tail is dropped and the settled history applied.
			name:     "in-flight at the tail keeps the prefix",
			remote:   []string{"A", "B", "C"},
			inflight: []string{"C"},
			want:     []string{"A", "B"},
		},
		{
			// The case that makes truncation necessary rather than filtering:
			// removing B alone would leave [A, C] where C links back to B, which
			// the apply path rejects outright as a broken chain.
			name:     "in-flight in the middle drops it and everything after",
			remote:   []string{"A", "B", "C", "D"},
			inflight: []string{"B"},
			want:     []string{"A"},
		},
		{
			name:     "in-flight at the head applies nothing",
			remote:   []string{"A", "B", "C"},
			inflight: []string{"A"},
			want:     []string{},
		},
		{
			name:     "cuts at the earliest in-flight entry",
			remote:   []string{"A", "B", "C", "D"},
			inflight: []string{"D", "B"},
			want:     []string{"A"},
		},
		{
			name:     "empty chain",
			remote:   []string{},
			inflight: []string{"A"},
			want:     []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateAtInflight(chain(tc.remote...), inflightSet(tc.inflight...))
			if !reflect.DeepEqual(chainIDs(got), tc.want) {
				t.Errorf("truncateAtInflight() = %v, want %v", chainIDs(got), tc.want)
			}
		})
	}
}

// The result is only ever a prefix, so every retained entry still links to its
// predecessor. This is the property the apply path's canonical-order check
// depends on, and it must hold for any input.
func TestTruncateAtInflightAlwaysReturnsAPrefix(t *testing.T) {
	remote := chain("A", "B", "C", "D", "E")

	for _, set := range [][]string{
		{}, {"A"}, {"B"}, {"C"}, {"E"}, {"B", "D"}, {"A", "E"}, {"A", "B", "C", "D", "E"},
	} {
		got := truncateAtInflight(remote, inflightSet(set...))
		if len(got) > len(remote) {
			t.Fatalf("inflight=%v produced %d entries from %d", set, len(got), len(remote))
		}
		for i := range got {
			if got[i].Tx.ID != remote[i].Tx.ID {
				t.Fatalf("inflight=%v: entry %d is %q, want %q — result is not a prefix",
					set, i, got[i].Tx.ID, remote[i].Tx.ID)
			}
		}
	}
}

func TestTruncateAtInflightDoesNotMutateInput(t *testing.T) {
	remote := chain("A", "B", "C")
	before := chainIDs(remote)

	truncateAtInflight(remote, inflightSet("B"))

	if !reflect.DeepEqual(chainIDs(remote), before) {
		t.Errorf("input chain was mutated: %v, want %v", chainIDs(remote), before)
	}
}

func TestRegistryIDSetSnapshot(t *testing.T) {
	r := newInflightRegistry()
	r.register(newInflightEntry("txn-1"))
	r.register(newInflightEntry("txn-2"))

	set := r.idSet()
	if len(set) != 2 || !set["txn-1"] || !set["txn-2"] {
		t.Fatalf("idSet() = %v, want txn-1 and txn-2", set)
	}

	// A snapshot, not a live view: it is used while the caller does network and
	// database work, and must not change underneath it.
	r.unregister("txn-1")
	r.register(newInflightEntry("txn-3"))
	if !set["txn-1"] || set["txn-3"] {
		t.Error("idSet() result tracked later registry changes; it must be a copy")
	}
}

func TestGuardAgainstInflightTrimsInFlightTail(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	// "B" is a sibling this node is still processing.
	p.inflight.register(newInflightEntry("B"))

	got := c.guardAgainstInflight("token-X", chain("A", "B", "C"))
	if want := []string{"A"}; !reflect.DeepEqual(chainIDs(got), want) {
		t.Errorf("guardAgainstInflight() = %v, want %v", chainIDs(got), want)
	}
}

func TestGuardAgainstInflightPassesThroughWhenNothingInFlight(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	remote := chain("A", "B", "C")
	got := c.guardAgainstInflight("token-X", remote)
	if !reflect.DeepEqual(chainIDs(got), chainIDs(remote)) {
		t.Errorf("guardAgainstInflight() = %v, want the chain unchanged", chainIDs(got))
	}
}

// The transaction being validated registers itself before it validates, so its
// own entry is in the set. Trimming at it matches what the sync request already
// asks the peer to exclude, and prevents a transaction ingesting itself from a
// peer mid-validation.
func TestGuardAgainstInflightTrimsTheCurrentTransaction(t *testing.T) {
	p, cancel := newTestProcessor(10, 0)
	defer cancel()
	c := newTestCore(p)

	p.inflight.register(newInflightEntry("current"))

	got := c.guardAgainstInflight("token-X", chain("A", "current"))
	if want := []string{"A"}; !reflect.DeepEqual(chainIDs(got), want) {
		t.Errorf("guardAgainstInflight() = %v, want %v", chainIDs(got), want)
	}
}

// SyncTransactionChainsFromPeer is shared with the quorum path, where there is
// no transaction processor at all. The guard must be inert there rather than
// panicking.
func TestGuardAgainstInflightIsInertWithoutAProcessor(t *testing.T) {
	c := newTestCore(nil)
	c.txnProcessor = nil

	remote := chain("A", "B")
	got := c.guardAgainstInflight("token-X", remote)
	if !reflect.DeepEqual(chainIDs(got), chainIDs(remote)) {
		t.Errorf("guardAgainstInflight() = %v, want the chain unchanged", chainIDs(got))
	}
}
