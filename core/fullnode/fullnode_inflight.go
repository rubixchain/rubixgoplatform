package fullnode

import (
	"encoding/json"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// In-flight tracking for the fullnode transaction pipeline: which transactions a
// worker is processing right now (unlike processedTxns, which remembers every
// admitted ID for dedupTTL). Read by the sync guard, by the readiness gate that
// holds a transaction until its previous transactions are persisted, and by the
// wake-up that releases those waiting transactions when one is persisted.

// inflightTxn is one transaction a worker has taken off txnQueue and not yet
// resolved.
type inflightTxn struct {
	id    string
	deps  []string
	event *models.EventTransaction

	// ready is closed when every previous transaction this one parked on is
	// persisted, or when one of them fails validation.
	ready     chan struct{}
	readyOnce sync.Once

	// pending is how many previous transactions this one is still parked on.
	// Guarded by inflightRegistry.mu because it must change together with
	// waitingOn.
	pending int

	// registeredAt lets sweepStale find a leaked entry. A leak matters because
	// the sync guard trims chains at pending IDs, so a stale entry would truncate
	// every sync of that token.
	registeredAt time.Time

	// failure is set when a previous transaction was found invalid. Guarded by
	// inflightRegistry.mu and read through failureOf: the channel close alone does
	// not order the writer's store against the waiter's read.
	failure error
}

// markReady closes ready at most once; both releaseWaiters and failDownstream
// may close it. Call it outside the registry lock.
func (t *inflightTxn) markReady() {
	t.readyOnce.Do(func() { close(t.ready) })
}

// maxWaitersPerProducer bounds how many transactions may park on one previous
// transaction. Real fan-out is a handful; past the cap park refuses and the
// waiting transaction falls back to its timer.
const maxWaitersPerProducer = 64

// inflightTTL is how old an entry must be before sweepStale treats it as
// leaked. Deliberately far above any real lifetime (wait plus three validation
// attempts with peer syncs), since sweeping a live entry would disable the sync
// guard for it.
const inflightTTL = 15 * time.Minute

// inflightRegistry indexes in-flight transactions by ID. One mutex guards every
// field because the operations are compound (check-and-insert, read-then-append),
// which sync.Map cannot make atomic. The lock is never held across a database
// call, a network call or a channel operation.
type inflightRegistry struct {
	mu   sync.Mutex
	byID map[string]*inflightTxn

	// waitingOn maps a previous transaction's ID to the transactions parked on
	// it. The key is a bare ID because the previous transaction may not have
	// arrived yet; the values are entries so release can close their channels
	// under the same lock.
	waitingOn map[string][]*inflightTxn

	// maxWaiters caps any one waiter list; a field so tests can lower it.
	maxWaiters int

	// parent (union-find forest) and members (root -> membership) track bundles;
	// see fullnode_components.go. Guarded by mu.
	parent  map[string]string
	members map[string][]string
}

func newInflightRegistry() *inflightRegistry {
	return &inflightRegistry{
		byID:       make(map[string]*inflightTxn),
		waitingOn:  make(map[string][]*inflightTxn),
		maxWaiters: maxWaitersPerProducer,
		parent:     make(map[string]string),
		members:    make(map[string][]string),
	}
}

// register adds t and reports whether it did. On false the caller does not own
// the entry and must not unregister it. No size cap is needed: only workers
// register, so the registry holds at most one entry per worker.
func (r *inflightRegistry) register(t *inflightTxn) bool {
	if t == nil || t.id == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[t.id]; exists {
		return false
	}

	if t.registeredAt.IsZero() {
		t.registeredAt = time.Now()
	}
	r.byID[t.id] = t
	return true
}

// unregister removes id (a no-op if absent) and prunes its bundle if no member is
// still in flight. It returns the drained bundle's membership, or nil.
func (r *inflightRegistry) unregister(id string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return r.pruneComponentLocked(id)
}

// sweepStale removes entries older than ttl and returns their IDs. A backstop:
// unregister is deferred (and runs on panic too), so a hit means a stuck worker.
// It matters because a stale entry makes the sync guard truncate every sync of
// that token. Waiters on a swept entry lose their wake-up and rely on their timers.
func (r *inflightRegistry) sweepStale(ttl time.Duration) []string {
	if ttl <= 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	var stale []string
	for id, t := range r.byID {
		if t.registeredAt.IsZero() || t.registeredAt.After(cutoff) {
			continue
		}
		stale = append(stale, id)
	}

	for _, id := range stale {
		delete(r.byID, id)
		delete(r.waitingOn, id)
	}
	// Prune only after every removal, so a bundle whose members all went stale
	// is judged on the final state.
	for _, id := range stale {
		r.pruneComponentLocked(id)
	}

	sort.Strings(stale)
	return stale
}

// has reports whether id is currently in flight.
func (r *inflightRegistry) has(id string) bool {
	if id == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.byID[id]
	return exists
}

// len returns how many transactions are in flight.
func (r *inflightRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byID)
}

// park records that t waits for the previous transaction producerID and reports
// whether the edge was recorded. Refusing is always safe: t just relies on its
// timer. It refuses a degenerate or duplicate edge (a duplicate would be counted
// twice but released once), a full waiter list, or an edge that closes a cycle.
func (r *inflightRegistry) park(t *inflightTxn, producerID string) bool {
	if t == nil || t.id == "" || producerID == "" || producerID == t.id {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	waiters := r.waitingOn[producerID]
	for _, w := range waiters {
		if w.id == t.id {
			return false
		}
	}
	if r.maxWaiters > 0 && len(waiters) >= r.maxWaiters {
		return false
	}
	if r.wouldCycleLocked(t.id, producerID) {
		return false
	}

	r.waitingOn[producerID] = append(waiters, t)
	t.pending++
	return true
}

// unpark removes t from the waiter list of each named previous transaction and
// returns how many it is still parked on. Edges already removed by release are
// skipped, so the waiting transaction can defer it unconditionally; without it a
// previous transaction that never arrives would keep its waitingOn entry forever.
func (r *inflightRegistry) unpark(t *inflightTxn, producerIDs []string) int {
	if t == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, producerID := range producerIDs {
		waiters, tracked := r.waitingOn[producerID]
		if !tracked {
			continue
		}
		for i, w := range waiters {
			if w.id != t.id {
				continue
			}
			waiters = append(waiters[:i], waiters[i+1:]...)
			if len(waiters) == 0 {
				delete(r.waitingOn, producerID)
			} else {
				r.waitingOn[producerID] = waiters
			}
			if t.pending > 0 {
				t.pending--
			}
			break
		}
	}
	return t.pending
}

// release removes every transaction parked on producerID and returns those with
// no previous transaction left to wait for. Call only after producerID's row is
// committed, and signal the returned entries outside the lock.
func (r *inflightRegistry) release(producerID string) []*inflightTxn {
	if producerID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	waiters, tracked := r.waitingOn[producerID]
	if !tracked {
		return nil
	}
	delete(r.waitingOn, producerID)

	var freed []*inflightTxn
	for _, w := range waiters {
		if w.pending > 0 {
			w.pending--
		}
		if w.pending == 0 {
			freed = append(freed, w)
		}
	}
	return freed
}

// failWaiters records cause against every transaction transitively parked on
// producerID and returns them for the caller to wake. The walk only follows
// waitingOn forwards, so it never touches a previous transaction or a mere bundle
// member. A visited set bounds it on cycles, and an existing failure is never
// overwritten (its waiter may already have read it).
func (r *inflightRegistry) failWaiters(producerID string, cause error) []*inflightTxn {
	if producerID == "" || cause == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	visited := map[string]bool{producerID: true}
	frontier := []string{producerID}
	var failed []*inflightTxn

	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]

		waiters, tracked := r.waitingOn[current]
		if !tracked {
			continue
		}
		delete(r.waitingOn, current)

		for _, w := range waiters {
			if w.pending > 0 {
				w.pending--
			}
			if visited[w.id] || w.failure != nil {
				continue
			}
			visited[w.id] = true
			w.failure = cause
			failed = append(failed, w)
			frontier = append(frontier, w.id)
		}
	}
	return failed
}

// failureOf reports the failure recorded against t, if any. Read under the lock
// because failure is written by another goroutine.
func (r *inflightRegistry) failureOf(t *inflightTxn) error {
	if t == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return t.failure
}

// waitersOf returns a copy of the IDs parked on producerID.
func (r *inflightRegistry) waitersOf(producerID string) []string {
	if producerID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	waiters := r.waitingOn[producerID]
	if len(waiters) == 0 {
		return nil
	}
	ids := make([]string, 0, len(waiters))
	for _, w := range waiters {
		ids = append(ids, w.id)
	}
	return ids
}

// waitingLen returns how many previous transactions currently have waiters, for
// metrics. It should return to zero when idle; steady growth means a missed unpark.
func (r *inflightRegistry) waitingLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.waitingOn)
}

// wouldCycleLocked reports whether parking consumerID on producerID would close a
// waiting cycle, i.e. producerID already waits (transitively) on consumerID.
// The caller must hold r.mu. Real chains cannot cycle; this guards malformed
// input that would otherwise stall both workers until their timers expire.
func (r *inflightRegistry) wouldCycleLocked(consumerID, producerID string) bool {
	visited := map[string]bool{consumerID: true}
	frontier := []string{consumerID}

	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		for _, w := range r.waitingOn[current] {
			if w.id == producerID {
				return true
			}
			if visited[w.id] {
				continue
			}
			visited[w.id] = true
			frontier = append(frontier, w.id)
		}
	}
	return false
}

// idSet returns a copy of the in-flight IDs, so callers can use it without
// holding the lock.
func (r *inflightRegistry) idSet() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make(map[string]bool, len(r.byID))
	for id := range r.byID {
		ids[id] = true
	}
	return ids
}

// truncateAtInflight returns the longest prefix of txs containing no ID in
// inflight. Applying a pending transaction from a peer would persist it
// unvalidated and move the tip past what its own validation expects. It cuts a
// prefix rather than dropping entries, because a chain with a hole is rejected.
func truncateAtInflight(txs []types.TransactionWithRole, inflight map[string]bool) []types.TransactionWithRole {
	if len(inflight) == 0 {
		return txs
	}
	for i, tx := range txs {
		if inflight[tx.Tx.ID] {
			return txs[:i]
		}
	}
	return txs
}

// GuardAgainstInflight trims a peer's chain at the first transaction this node
// already holds, in flight or still in txnQueue, and records the truncation.
// Returns txs unchanged when nothing needs trimming or there is no registry.
func (p *DynamicTxnProcessor) GuardAgainstInflight(tokenID string, txs []types.TransactionWithRole) []types.TransactionWithRole {
	if p == nil || p.inflight == nil {
		return txs
	}

	guarded := truncateAtInflight(txs, p.pendingIDSet())
	if len(guarded) == len(txs) {
		return txs
	}

	// A token that truncates on every sync points at a leaked entry.
	var firstDropped string
	if len(guarded) < len(txs) {
		firstDropped = txs[len(guarded)].Tx.ID
	}

	// Recorded because the cut can drop entries beyond the current transaction's
	// own previous transactions; later error classification reads this note.
	p.truncated.record(tokenID)

	p.host.Log().Info("Chain sync truncated at an in-flight transaction",
		"tokenID", tokenID,
		"remoteCount", len(txs),
		"appliedCount", len(guarded),
		"stoppedAt", firstDropped)

	return guarded
}

// registerInflight records txnEvent as in flight and links it into a bundle with
// its declared previous transactions. It returns the entry only if this call
// created it (the caller must then unregister it), else nil. A transaction whose
// info fails to parse is still registered, with no dependencies.
func (p *DynamicTxnProcessor) registerInflight(txnEvent *models.EventTransaction) *inflightTxn {
	entry := &inflightTxn{
		id:    txnEvent.TransactionID,
		event: txnEvent,
		ready: make(chan struct{}),
	}

	if txnEvent.Transaction != nil && len(txnEvent.Transaction.Info) > 0 {
		var info models.TransactionInfo
		if err := json.Unmarshal(txnEvent.Transaction.Info, &info); err != nil {
			p.host.Log().Debug("registerInflight: transaction info did not unmarshal, registering with no dependencies",
				"txnID", txnEvent.TransactionID, "err", err)
		} else {
			entry.deps = transactionDependencies(&info)
		}
	}

	if !p.inflight.register(entry) {
		// Should be unreachable: admission is single-winner.
		p.host.Log().Warn("registerInflight: transaction is already in flight, leaving the existing entry alone",
			"txnID", txnEvent.TransactionID)
		return nil
	}

	// Metrics: how often a declared previous transaction is still in flight.
	if len(entry.deps) > 0 {
		atomic.AddInt64(&p.depsObserved, int64(len(entry.deps)))
		for _, dep := range entry.deps {
			if p.inflight.has(dep) {
				atomic.AddInt64(&p.depsInFlight, 1)
				p.host.Log().Debug("registerInflight: declared dependency is still in flight",
					"txnID", entry.id, "dependsOn", dep)
			}
		}

		// Link every declared previous transaction, not just unresolved ones, so
		// bundle membership does not depend on persist timing.
		p.inflight.linkComponent(entry.id, entry.deps)
	}

	// Transactions that arrived first may already be parked on this one; they are
	// woken when it persists. Counted here because this is where out-of-order
	// arrival is visible.
	waiters := p.inflight.waitersOf(entry.id)
	if len(waiters) > 0 {
		atomic.AddInt64(&p.revEdges, int64(len(waiters)))

		// Redundant today (a transaction only parks on a previous transaction it
		// declared, already linked at its registration); kept in case parking
		// ever goes beyond declared dependencies.
		p.inflight.linkComponent(entry.id, waiters)

		p.host.Log().Debug("registerInflight: transactions are already waiting on this one",
			"txnID", entry.id, "waiters", waiters)
	}

	// Debug log only.
	if len(entry.deps) > 0 || len(waiters) > 0 {
		if members := p.inflight.componentMembers(entry.id); len(members) > 1 {
			p.host.Log().Debug("registerInflight: transaction belongs to a bundle",
				"txnID", entry.id, "bundleSize", len(members), "members", members)
		}
	}

	return entry
}

// unregisterInflight removes a transaction's registry entry and, if it was the
// last live member of its bundle, logs and counts the drained bundle.
func (p *DynamicTxnProcessor) unregisterInflight(id string) {
	if p == nil || p.inflight == nil {
		return
	}

	drained := p.inflight.unregister(id)
	if len(drained) == 0 {
		return
	}

	atomic.AddInt64(&p.bundlesDrained, 1)
	p.host.Log().Debug("Bundle drained", "size", len(drained), "members", drained)
}

// releaseWaiters wakes every transaction whose last pending previous transaction
// was producerID. Call only after producerID's row is committed: ready closes
// once, so a waiting transaction woken early would miss the row and not be woken
// again. Channels are closed outside the registry lock.
func (p *DynamicTxnProcessor) releaseWaiters(producerID string) {
	if p == nil || p.inflight == nil {
		return
	}

	freed := p.inflight.release(producerID)
	if len(freed) == 0 {
		return
	}

	ids := make([]string, 0, len(freed))
	for _, waiter := range freed {
		waiter.markReady()
		ids = append(ids, waiter.id)
	}
	atomic.AddInt64(&p.cascadeReleases, int64(len(freed)))
	p.host.Log().Debug("Released transactions waiting on a now-persisted producer",
		"producerID", producerID, "released", ids)
}

// failDownstream fails and wakes every transaction transitively waiting on
// producerID, which this node found invalid. Call it only for a validation
// verdict: a transient error says nothing about the waiting transactions, which
// must keep their own retries. Persisted transactions are never touched.
func (p *DynamicTxnProcessor) failDownstream(producerID string, cause error) {
	if p == nil || p.inflight == nil {
		return
	}

	failed := p.inflight.failWaiters(producerID, cause)
	if len(failed) == 0 {
		return
	}

	ids := make([]string, 0, len(failed))
	for _, waiter := range failed {
		waiter.markReady()
		ids = append(ids, waiter.id)
	}
	atomic.AddInt64(&p.failuresPropagated, int64(len(failed)))
	p.host.Log().Info("Failing transactions that depend on an invalid producer",
		"producerID", producerID, "failed", ids, "cause", cause)
}
