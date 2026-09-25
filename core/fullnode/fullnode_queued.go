package fullnode

import "sync"

// Queue visibility for the fullnode transaction pipeline.
//
// The in-flight registry answers "which transactions is this node processing
// right now", and it is populated on dequeue — so it describes at most
// maxWorkers transactions while txnQueue can hold 10 000. Everything waiting in
// that queue is invisible to the two readers that most need to see it: the sync
// guard, which must not ingest a chain entry belonging to a transaction this
// node already holds, and the readiness gate, which picks how long to wait for a
// producer based on whether this node has it.
//
// The consequence is not a missed optimisation. A split sitting in the queue
// while the transfer that spends its output is validated means the guard does
// not trim, the transfer syncs the split from a peer and persists it unvalidated
// — which is precisely what the guard exists to prevent — and the split is then
// dead-lettered when its own turn comes and the tip has moved past it.
//
// Registering queued transactions in the registry itself would fix the blind
// spot and break four other things: maxInflightEntries (5 000) sits deliberately
// below the queue's capacity so the queue fills first, and inverting that would
// have the registry reject transactions under load and process them with no
// protections at all; the defer that releases an entry would no longer sit
// beside the call that takes it, which is the only reason leaks are impossible
// today; sweepStale's registeredAt would start measuring queue time and raise
// Error lines that mean "a worker died"; and the waiting edges, ready channels
// and components are all meaningless for a transaction with no goroutine.
//
// So the two states stay separate and are unioned where they are read. This set
// holds only IDs, is bounded by the queue it shadows, and has exactly one
// lifecycle: added before the send that puts a transaction on txnQueue, removed
// the moment a worker takes it off.
type queuedSet struct {
	mu  sync.RWMutex
	ids map[string]struct{}
}

func newQueuedSet() *queuedSet {
	return &queuedSet{ids: make(map[string]struct{})}
}

// add records that id has been admitted and is on its way to txnQueue.
//
// It is called BEFORE the channel send, not after. A send hands the event to a
// worker the instant it completes, so marking afterwards races the worker's own
// remove: the remove would find nothing, the add would land behind it, and the
// entry would never be cleared — and a stale entry here truncates every sync of
// that token, which is the one failure this set must not cause.
func (q *queuedSet) add(id string) {
	if q == nil || id == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids[id] = struct{}{}
}

// remove clears id, whether it was dequeued by a worker or never made it onto
// the queue at all.
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

// isPending reports whether this node already holds id — queued or in flight.
//
// This is the question every caller that used inflight.has() was actually
// asking. The registry alone answers a narrower one, "is a worker on it right
// now", and the gap between the two is the queue.
func (p *DynamicTxnProcessor) isPending(id string) bool {
	if p == nil || id == "" {
		return false
	}
	if p.inflight != nil && p.inflight.has(id) {
		return true
	}
	return p.queued.has(id)
}

// pendingIDSet snapshots every transaction this node holds, queued or in flight.
//
// A copy, for the reason idSet returns one: the guard uses it while doing
// network and database work, and neither lock may be held across that.
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
