package fullnode

import "sort"

// Bundle tracking: a union-find forest grouping each transaction with the
// previous transactions it declares, transitively. componentRoot keys the
// per-bundle sync memo (bundleScope); everything else here only feeds logs and
// metrics. The forest shares the registry mutex so bundle membership and
// in-flight state cannot disagree.

// findLocked returns the root of id's component (with path compression),
// creating a component for id if it has none. The caller must hold r.mu.
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

// unionLocked merges the components of a and b, attaching the smaller to the
// larger. The caller must hold r.mu.
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

// linkComponent puts id and everything in related into one component, under a
// single lock so no caller sees a half-applied merge. With nothing to relate,
// id never enters the forest.
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

// componentMembers returns every transaction ID in id's component, sorted so
// logs do not depend on arrival order, or nil if id is in no component. Used
// for logging only.
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

// componentRoot returns the root of id's component, or "" if it has none. It
// keys the per-bundle sync memo. The root can change when components merge, so
// nothing may rely on its stability; a change only costs a memo miss.
func (r *inflightRegistry) componentRoot(id string) string {
	if id == "" {
		return ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, tracked := r.parent[id]; !tracked {
		return ""
	}
	return r.findLocked(id)
}

// componentLen returns how many transaction IDs the forest holds, for metrics
// and tests. A previous transaction that never arrives is only removed by
// pruning, so steady growth here means pruning is broken.
func (r *inflightRegistry) componentLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parent)
}

// pruneComponentLocked drops id's whole component once no member is in byID,
// and returns the dropped membership sorted (nil if nothing drained). It drops
// the component whole because members left behind would never be pruned. The
// caller must hold r.mu.
func (r *inflightRegistry) pruneComponentLocked(id string) []string {
	if _, tracked := r.parent[id]; !tracked {
		return nil
	}

	root := r.findLocked(id)
	for _, member := range r.members[root] {
		if _, live := r.byID[member]; live {
			return nil
		}
	}

	drained := r.members[root]
	for _, member := range drained {
		delete(r.parent, member)
	}
	delete(r.members, root)

	sort.Strings(drained)
	return drained
}
