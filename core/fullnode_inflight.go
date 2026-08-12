package core

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// In-flight tracking for the fullnode transaction pipeline.
//
// processedTxns is a *seen-recently* set: an ID stays in it for dedupTTL after
// admission whether or not the transaction is still being worked on. This
// registry answers a different question — which transactions has the fullnode
// received but not yet persisted or failed, right now.
//
// At this commit nothing acts on the answer; the registry only feeds metrics.
// Later commits use it to keep a chain sync from ingesting a sibling that is
// still being validated, and to hold a transaction until the producers it
// declares have resolved.

// inflightTxn is one transaction taken off txnQueue and not yet resolved.
type inflightTxn struct {
	id    string
	deps  []string
	event *models.EventTransaction

	// ready is closed once every dependency has resolved. Nothing waits on it or
	// closes it yet — the readiness gate waits, the cascade release closes. It is
	// allocated here so a registry entry always has its final shape and later
	// commits do not have to reason about half-built entries.
	ready chan struct{}
}

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

	// waitingOn maps a producer transaction ID to the consumer IDs blocked on
	// it. Declared now so the registry's shape is settled; nothing populates it
	// until the cascade commit.
	waitingOn map[string][]string
}

func newInflightRegistry() *inflightRegistry {
	return &inflightRegistry{
		byID:      make(map[string]*inflightTxn),
		waitingOn: make(map[string][]string),
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

	// Metrics only at this commit. How often a transaction arrives while a
	// producer it declares is still being processed is the number that decides
	// how aggressive the readiness gate needs to be.
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

	return entry
}
