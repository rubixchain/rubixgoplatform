package core

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// In-flight tracking for the fullnode transaction pipeline.
//
// processedTxns is a *seen-recently* set: an ID stays in it for dedupTTL after
// admission whether or not the transaction is still being worked on. This
// registry answers a different question — which transactions has the fullnode
// received but not yet persisted or failed, right now.
//
// Three things read that answer: the sync guard, which keeps a chain sync from
// ingesting a sibling that is still being validated; the readiness gate, which
// holds a transaction until the producers it declares are persisted; and the
// cascade below, which wakes those held transactions the moment their producer
// commits instead of leaving them to re-check or time out.

// inflightTxn is one transaction taken off txnQueue and not yet resolved.
type inflightTxn struct {
	id    string
	deps  []string
	event *models.EventTransaction

	// ready is closed once every producer this transaction parked on has been
	// persisted. The readiness gate waits on it; the cascade release closes it.
	ready     chan struct{}
	readyOnce sync.Once

	// pending is how many producers this transaction is still parked on.
	//
	// It is guarded by inflightRegistry.mu rather than by the entry itself,
	// because it only ever changes together with waitingOn and the two must not
	// be able to disagree: the count reaching zero is precisely the condition
	// that closes ready.
	pending int
}

// markReady closes ready, at most once.
//
// A plain close would panic on the second call, and there genuinely are two
// callers: the cascade, when the last producer commits, and — in the
// double-check below — the waiter itself. Always called outside the registry
// lock.
func (t *inflightTxn) markReady() {
	t.readyOnce.Do(func() { close(t.ready) })
}

// maxWaitersPerProducer bounds how many transactions may park on a single
// producer.
//
// The legitimate fan-out is small: a transfer plus one split per quorum member,
// so a handful. A list far longer than that is not a real bundle, it is either
// malformed input or a leak, and past the cap parking degrades to the plain
// timeout rather than growing the list without limit.
const maxWaitersPerProducer = 64

// inflightRegistry indexes received-but-unresolved transactions by their own ID.
//
// One mutex guards every field. sync.Map is deliberately not used: the
// operations are compound — check-and-insert, and later read-then-append — and
// sync.Map cannot make those atomic. Atomicity is the point, since pubsub
// dispatches each message on its own goroutine (types/pubsub.go:164) and the
// worker pool adds more concurrency on top.
//
// The lock is never held across a database call, a network call or a channel
// receive; every method below returns before its caller does any of those.
type inflightRegistry struct {
	mu   sync.Mutex
	byID map[string]*inflightTxn

	// waitingOn maps a producer transaction ID to the consumers blocked on it.
	//
	// The producer is a bare ID because it may be a transaction this node has
	// never seen — that is the whole point of the reverse edge, that a consumer
	// can park before its producer arrives. The consumers are entries rather
	// than IDs because releasing them means closing a channel on each, and
	// resolving IDs back to entries afterwards would reintroduce the window the
	// single lock exists to close.
	waitingOn map[string][]*inflightTxn

	// maxWaiters caps the length of any one waiter list. A field rather than the
	// constant directly so a test can reach the cap without parking 64
	// transactions.
	maxWaiters int
}

func newInflightRegistry() *inflightRegistry {
	return &inflightRegistry{
		byID:       make(map[string]*inflightTxn),
		waitingOn:  make(map[string][]*inflightTxn),
		maxWaiters: maxWaitersPerProducer,
	}
}

// register adds t and reports whether this call created the entry.
//
// A false return means the ID was already registered. The caller must then
// leave the entry alone: it belongs to whoever registered it, and unregistering
// it would drop that owner's tracking.
func (r *inflightRegistry) register(t *inflightTxn) bool {
	if t == nil || t.id == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[t.id]; exists {
		return false
	}
	r.byID[t.id] = t
	return true
}

// unregister removes id. It is a no-op if id is absent, so callers can defer it
// unconditionally.
func (r *inflightRegistry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
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

// park records that t is waiting for producerID, and reports whether the edge
// was recorded.
//
// A false return is not an error. It means no cascade release will arrive for
// this edge and the caller is on its own timeout — which is exactly the
// behaviour that existed before the cascade, so refusing to park is always safe.
// There are three reasons for it: the edge is degenerate, the fan-out cap is
// reached, or the edge would close a waiting cycle.
//
// The caller must pass distinct producers. Parking twice on the same producer is
// refused rather than counted twice, since the release that follows would only
// decrement once and the waiter would never reach zero.
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

// unpark removes t from the waiter list of each producer named and returns how
// many producers it is still parked on.
//
// The waiter calls this itself when its wait ends, whatever ended it. Without it
// a producer that never arrives would hold a waiter list, and therefore an entry
// in waitingOn, for the lifetime of the process.
//
// Calling it for an edge that release already removed is a no-op, so the waiter
// can defer it unconditionally over the same set it parked on.
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

// release removes every waiter parked on producerID and returns those left with
// no producer to wait for.
//
// Only the last of a transaction's producers frees it: a transfer that spends
// two splits has to see both persisted, and waking it after the first would send
// it to validate against a chain that is still incomplete.
//
// Callers must invoke this only after the producer's row is committed, and must
// signal the returned entries outside the lock.
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

// waitersOf returns the IDs parked on producerID.
//
// This is the reverse check performed when a transaction arrives: it answers
// "did anyone give up on me arriving and park in the meantime?". A copy, because
// the caller reads it after the lock is dropped.
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

// waitingLen returns how many producers currently have waiters.
//
// Purely an observability figure. It should track the number of parked
// transactions and fall back to zero when the pipeline is idle; a value that
// only climbs means a waiter is failing to unpark.
func (r *inflightRegistry) waitingLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.waitingOn)
}

// wouldCycleLocked reports whether parking consumerID on producerID would close a
// waiting cycle. The caller must hold r.mu.
//
// waitingOn maps a producer to its waiters, so walking it forwards from
// consumerID enumerates everything transitively blocked *by* consumerID. If
// producerID turns up in that set then producerID is already waiting, directly
// or through a chain, on consumerID — and adding the reverse edge would park
// both until their timers expire.
//
// A real chain cannot do this: a transaction's producers precede it. The guard
// is for malformed or hostile input, where the cost of refusing is one lost
// cascade and the cost of not refusing is two stalled workers.
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

// idSet returns a snapshot of the in-flight transaction IDs.
//
// A copy rather than a live view: callers use it while doing network and
// database work, and the lock must not be held across either.
func (r *inflightRegistry) idSet() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make(map[string]bool, len(r.byID))
	for id := range r.byID {
		ids[id] = true
	}
	return ids
}

// truncateAtInflight returns the longest prefix of txs that contains no
// transaction currently in flight.
//
// A peer returns a token's chain as it stands on that peer, which can include
// transactions this fullnode has received but not yet validated. Applying those
// persists a chain entry the fullnode never checked, and it advances the local
// tip past what the still-in-flight transaction expects — that transaction then
// fails its own integrity check with a chain mismatch, caused entirely by a sync
// performed on someone else's behalf.
//
// Cutting to a prefix is what makes this safe. Dropping entries from the middle
// instead would leave a hole, and applyTokenChainFromSyncForFullNode rejects a
// chain whose links do not join up, failing the whole sync rather than trimming
// it. A prefix of a valid chain is always itself a valid chain.
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

// guardAgainstInflight trims a peer's chain response so it cannot carry an
// entry belonging to a transaction this node is still processing.
//
// Returns txs unchanged when there is nothing to trim, including on a node with
// no transaction processor, so the non-fullnode sync path is unaffected.
func (c *Core) guardAgainstInflight(tokenID string, txs []types.TransactionWithRole) []types.TransactionWithRole {
	if c.txnProcessor == nil || c.txnProcessor.inflight == nil {
		return txs
	}

	guarded := truncateAtInflight(txs, c.txnProcessor.inflight.idSet())
	if len(guarded) == len(txs) {
		return txs
	}

	// Worth an Info line: this is the difference between the fullnode ingesting
	// an unvalidated sibling and not. A token that truncates on every sync
	// points at a leaked registry entry rather than genuine concurrency.
	var firstDropped string
	if len(guarded) < len(txs) {
		firstDropped = txs[len(guarded)].Tx.ID
	}
	c.log.Info("Chain sync truncated at an in-flight transaction",
		"tokenID", tokenID,
		"remoteCount", len(txs),
		"appliedCount", len(guarded),
		"stoppedAt", firstDropped)

	return guarded
}

// registerInflight records txnEvent as in flight and counts the dependency edges
// it declares.
//
// It returns the entry only when this call created it, in which case the caller
// owns the entry and must unregister it. It returns nil when the ID was already
// registered — unregistering in that case would remove another worker's entry.
//
// Registration is unconditional on the payload parsing: a transaction whose info
// cannot be unmarshalled still occupies the pipeline and still has to be visible
// as in flight. It is registered with no dependencies and will fail validation
// shortly afterwards on its own merits.
func (c *Core) registerInflight(txnEvent *models.EventTransaction) *inflightTxn {
	entry := &inflightTxn{
		id:    txnEvent.TransactionID,
		event: txnEvent,
		ready: make(chan struct{}),
	}

	if txnEvent.Transaction != nil && len(txnEvent.Transaction.Info) > 0 {
		var info models.TransactionInfo
		if err := json.Unmarshal(txnEvent.Transaction.Info, &info); err != nil {
			c.log.Debug("registerInflight: transaction info did not unmarshal, registering with no dependencies",
				"txnID", txnEvent.TransactionID, "err", err)
		} else {
			entry.deps = transactionDependencies(&info)
		}
	}

	if !c.txnProcessor.inflight.register(entry) {
		// Admission is single-winner, so one transaction reaches one worker and
		// this should be unreachable. Log rather than assume.
		c.log.Warn("registerInflight: transaction is already in flight, leaving the existing entry alone",
			"txnID", txnEvent.TransactionID)
		return nil
	}

	// How often a transaction arrives while a producer it declares is still
	// being processed is the number that sizes the readiness gate.
	if len(entry.deps) > 0 {
		atomic.AddInt64(&c.txnProcessor.depsObserved, int64(len(entry.deps)))
		for _, dep := range entry.deps {
			if c.txnProcessor.inflight.has(dep) {
				atomic.AddInt64(&c.txnProcessor.depsInFlight, 1)
				c.log.Debug("registerInflight: declared dependency is still in flight",
					"txnID", entry.id, "dependsOn", dep)
			}
		}
	}

	// The reverse edge. This transaction may be the producer that others have
	// already parked on, which is the arrival order the cascade exists to make
	// harmless: they are woken when this transaction persists, without ever
	// having had to know it was coming.
	//
	// Nothing has to be done here to make that happen — the edges were recorded
	// when they parked — but this is the only point at which the out-of-order
	// case is visible, and its frequency is what justifies the machinery.
	if waiters := c.txnProcessor.inflight.waitersOf(entry.id); len(waiters) > 0 {
		atomic.AddInt64(&c.txnProcessor.revEdges, int64(len(waiters)))
		c.log.Debug("registerInflight: transactions are already waiting on this one",
			"txnID", entry.id, "waiters", waiters)
	}

	return entry
}

// releaseWaiters wakes every transaction parked on producerID.
//
// Must be called only once producerID's row is committed. A waiter woken any
// earlier would re-probe, still not find the row, and lose its cascade for
// nothing — its ready channel closes once and cannot be rearmed.
//
// The channel closes happen outside the registry lock. Nothing blocks on a
// close, but keeping every wake-up outside the lock is what makes the rule
// "never hold the registry mutex across a database call or a channel operation"
// simple enough to enforce by inspection.
func (c *Core) releaseWaiters(producerID string) {
	if c.txnProcessor == nil || c.txnProcessor.inflight == nil {
		return
	}

	freed := c.txnProcessor.inflight.release(producerID)
	if len(freed) == 0 {
		return
	}

	ids := make([]string, 0, len(freed))
	for _, waiter := range freed {
		waiter.markReady()
		ids = append(ids, waiter.id)
	}
	atomic.AddInt64(&c.txnProcessor.cascadeReleases, int64(len(freed)))
	c.log.Debug("Released transactions waiting on a now-persisted producer",
		"producerID", producerID, "released", ids)
}
