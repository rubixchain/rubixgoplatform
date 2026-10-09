package fullnode

import "sync"

// queuedSet holds the IDs of admitted transactions still waiting in txnQueue,
// which the in-flight registry (filled only by workers) cannot see. The sync
// guard and readiness gate union the two. Kept separate from the registry so its
// deferred unregister and stale sweep stay worker-only. An ID is added before the
// send to txnQueue and removed when a worker dequeues it or the send fails.
type queuedSet struct {
	mu  sync.RWMutex
	ids map[string]struct{}
}

func newQueuedSet() *queuedSet {
	return &queuedSet{ids: make(map[string]struct{})}
}

// add records that id is about to be sent to txnQueue. Call it BEFORE the send:
// adding afterwards could land after the worker's remove and leave a stale ID,
// which would truncate every sync of that token.
func (q *queuedSet) add(id string) {
	if q == nil || id == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids[id] = struct{}{}
}

// remove clears id, whether a worker dequeued it or the send failed.
func (q *queuedSet) remove(id string) {
	if q == nil || id == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.ids, id)
}

// has reports whether id is waiting in the queue.
func (q *queuedSet) has(id string) bool {
	if q == nil || id == "" {
		return false
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	_, queued := q.ids[id]
	return queued
}

// union adds every queued ID to dst, which is the caller's own snapshot.
func (q *queuedSet) union(dst map[string]bool) map[string]bool {
	if q == nil {
		return dst
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if dst == nil {
		dst = make(map[string]bool, len(q.ids))
	}
	for id := range q.ids {
		dst[id] = true
	}
	return dst
}

// len returns how many transactions are queued but not yet dequeued.
func (q *queuedSet) len() int {
	if q == nil {
		return 0
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.ids)
}

// isPending reports whether this node already holds id, queued or in flight.
func (p *DynamicTxnProcessor) isPending(id string) bool {
	if p == nil || id == "" {
		return false
	}
	if p.inflight != nil && p.inflight.has(id) {
		return true
	}
	return p.queued.has(id)
}

// pendingIDSet returns a copy of every ID this node holds, queued or in flight,
// so the guard can use it without holding either lock.
func (p *DynamicTxnProcessor) pendingIDSet() map[string]bool {
	if p == nil {
		return nil
	}
	var ids map[string]bool
	if p.inflight != nil {
		ids = p.inflight.idSet()
	}
	return p.queued.union(ids)
}
