package core

import "sort"

// Component tracking for the fullnode transaction pipeline.
//
// The cascade relates transactions in pairs: this one waits for that one. A
// bundle is the transitive closure of those pairs — a split, the transfer that
// spends its output, and the quorum splits that arrived alongside them are one
// unit of work even though no single edge says so.
//
// Union-find is what turns the pairwise edges into that closure. Every
// transaction starts as its own component and each declared dependency merges
// two components into one, so membership is available the moment a transaction
// registers rather than only once its whole bundle has arrived.
//
// Nothing changes behaviour on the strength of it yet. The sync-once memo is
// scoped per bundle, and it needs this identity before it can be written.
//
// The forest lives under the registry's existing mutex rather than a second one.
// A component is a statement about which transactions are in flight and which
// are waiting on which, and both of those already live there; a separate lock
// would let the two answers disagree.

// findLocked returns the root of id's component, creating a component for id if
// it has none. The caller must hold r.mu.
//
// Path compression: every node on the way to the root is repointed straight at
// it, so a component that has been merged repeatedly still answers in near
// constant time on the next query.
func (r *inflightRegistry) findLocked(id string) string {
	if _, tracked := r.parent[id]; !tracked {
		r.parent[id] = id
		r.members[id] = []string{id}
		return id
	}

	root := id
	for r.parent[root] != root {
		root = r.parent[root]
	}
	for r.parent[id] != root {
		next := r.parent[id]
		r.parent[id] = root
		id = next
	}
	return root
}

// unionLocked merges the components of a and b. The caller must hold r.mu.
//
// The smaller membership is appended to the larger, which keeps the trees flat
// and makes each merge cost the size of the smaller side rather than the whole
// component.
func (r *inflightRegistry) unionLocked(a, b string) {
	rootA, rootB := r.findLocked(a), r.findLocked(b)
	if rootA == rootB {
		return
	}
	if len(r.members[rootA]) < len(r.members[rootB]) {
		rootA, rootB = rootB, rootA
	}

	r.parent[rootB] = rootA
	r.members[rootA] = append(r.members[rootA], r.members[rootB]...)
	delete(r.members, rootB)
}

// linkComponent puts id and everything in related into a single component.
//
// One lock acquisition for the whole set: a half-applied merge is a component
// that two concurrent callers would describe differently.
//
// Nothing is created when there is nothing to relate. A transaction declaring no
// dependencies and having no waiters — the overwhelming majority — never enters
// the forest at all, so the common case costs a comparison.
func (r *inflightRegistry) linkComponent(id string, related []string) {
	if id == "" || len(related) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, other := range related {
		if other == "" || other == id {
			continue
		}
		r.unionLocked(id, other)
	}
}

// componentMembers returns every transaction ID in id's component, sorted.
//
// Sorted because the order the merges happened to produce is an artefact of
// arrival order, and what reads this downstream compares and hashes the set. A
// bundle identity that depended on which member arrived first would not be an
// identity.
//
// Returns nil for a transaction with no relations, so a caller can treat "no
// component" and "a component of one" as the same thing — which they are.
func (r *inflightRegistry) componentMembers(id string) []string {
	if id == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, tracked := r.parent[id]; !tracked {
		return nil
	}
	members := append([]string(nil), r.members[r.findLocked(id)]...)
	sort.Strings(members)
	return members
}

// componentLen returns how many transaction IDs the forest holds.
//
// Observability, and the assertion that matters most in the tests. Unlike byID
// this map has no natural per-entry removal point — a producer that never
// arrives is only ever a name in here — so if pruning is wrong it grows without
// bound and nothing else in the process would notice.
func (r *inflightRegistry) componentLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parent)
}

// pruneComponentLocked drops id's whole component once no member of it is still
// in flight. The caller must hold r.mu.
//
// Being in byID is the only liveness test needed. A member that is parked is in
// flight by definition, and a member that has waiters has those waiters in
// flight; so if nothing in the component is in byID, nothing about it can still
// be true.
//
// The component is dropped whole rather than a member at a time. Removing one
// member of a live component would be wrong — the survivors are still one
// bundle — and removing one member of a dead component leaves the rest of it
// stranded with nothing left to trigger another sweep.
func (r *inflightRegistry) pruneComponentLocked(id string) {
	if _, tracked := r.parent[id]; !tracked {
		return
	}

	root := r.findLocked(id)
	for _, member := range r.members[root] {
		if _, live := r.byID[member]; live {
			return
		}
	}

	for _, member := range r.members[root] {
		delete(r.parent, member)
	}
	delete(r.members, root)
}
