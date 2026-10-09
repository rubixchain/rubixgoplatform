package fullnode

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// The readiness gate holds a transaction until its previous transactions are
// persisted, saving the peer chain sync that validating it early would trigger.
// A previous transaction wakes its waiters when it commits (releaseWaiters); the
// timer is only a backstop, and an expired wait falls through to the normal
// validate-and-sync path, so the gate never blocks a transaction permanently.

// errProcessorShuttingDown reports that the wait was abandoned because the
// processor is stopping.
var errProcessorShuttingDown = errors.New("transaction processor is shutting down")

// dependencyResolved reports whether depID is persisted on this node. An empty
// ID (genesis) is always resolved. A lookup failure is returned as an error, not
// as "absent", so the caller can tell a database problem from a missing row.
func (p *DynamicTxnProcessor) dependencyResolved(depID string) (bool, error) {
	if depID == "" {
		return true, nil
	}
	if _, err := p.host.Wallet().GetTransactionByID(depID, true); err != nil {
		if errors.Is(err, wallet.ErrTransactionNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// partitionDependencies splits deps into those already persisted and those not.
// A failed lookup counts as resolved: proceeding takes the path the transaction
// would have taken anyway, whereas waiting would turn a database blip into a
// stalled pipeline.
func (p *DynamicTxnProcessor) partitionDependencies(deps []string) (resolved, unresolved []string) {
	for _, dep := range deps {
		ok, err := p.resolveDependency(dep)
		if err != nil {
			p.host.Log().Warn("awaitDependencies: could not check a dependency, treating it as resolved",
				"dependsOn", dep, "err", err)
			resolved = append(resolved, dep)
			continue
		}
		if ok {
			resolved = append(resolved, dep)
			continue
		}
		unresolved = append(unresolved, dep)
	}
	return resolved, unresolved
}

// dependencyWait picks how long to wait, taking the longest applicable tier. A
// previous transaction this node holds (queued or in flight) will be processed
// soon and gets the long tier; one that is absent may never arrive (the node may
// have joined after it was published) and gets the short one.
func (p *DynamicTxnProcessor) dependencyWait(unresolved []string) time.Duration {
	cfg := p.bundle
	var wait time.Duration
	for _, dep := range unresolved {
		tier := cfg.unknownWait
		if p.isPending(dep) {
			tier = cfg.inflightWait
		}
		if tier > wait {
			wait = tier
		}
	}
	return wait
}

// awaitDependencies blocks until every previous transaction t declares is
// persisted, the wait expires, or the processor shuts down. It returns an error
// on shutdown or when a previous transaction was found invalid (wrapping
// errProducerFailed). An expired wait returns nil, so the transaction falls
// through to normal validation, whose integrity check syncs what is missing.
func (p *DynamicTxnProcessor) awaitDependencies(t *inflightTxn) error {
	if t == nil || len(t.deps) == 0 {
		return nil
	}

	_, unresolved := p.partitionDependencies(t.deps)
	if len(unresolved) == 0 {
		return nil
	}

	// Gauge for the metrics log only. Each wait occupies its own worker and is
	// bounded by its timer, so the number waiting is bounded by the pool size.
	atomic.AddInt64(&p.parkedCount, 1)
	defer atomic.AddInt64(&p.parkedCount, -1)

	// Record the edges first, so a previous transaction that commits from here
	// on finds this transaction and wakes it directly.
	parkedOn := make([]string, 0, len(unresolved))
	for _, dep := range unresolved {
		if p.inflight.park(t, dep) {
			parkedOn = append(parkedOn, dep)
			continue
		}
		p.host.Log().Debug("awaitDependencies: declined to park on a producer, this dependency can only time out",
			"txnID", t.id, "dependsOn", dep)
	}
	if len(parkedOn) == 0 {
		// Every edge was refused (a cycle, or a previous transaction's fan-out
		// cap). Nothing can wake this transaction, so waiting could only expire.
		return nil
	}
	defer p.inflight.unpark(t, parkedOn)

	// Re-probe after parking to avoid a lost wake-up: a previous transaction
	// that committed between the first probe and parking released nobody.
	// Release follows the commit, so it either shows up here or finds the edge.
	started := time.Now()
	resolved, waitingFor := p.partitionDependencies(parkedOn)
	if len(resolved) > 0 {
		if remaining := p.inflight.unpark(t, resolved); remaining == 0 {
			p.host.Log().Debug("awaitDependencies: producers resolved while the edges were being recorded",
				"txnID", t.id)
			return nil
		}
	}
	if len(waitingFor) == 0 {
		return nil
	}

	wait := p.dependencyWait(waitingFor)
	p.host.Log().Debug("awaitDependencies: holding transaction until its producers resolve",
		"txnID", t.id, "unresolved", waitingFor, "wait", wait)

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-t.ready:
		// ready closes on both release and failure. If a previous transaction
		// was found invalid, skip validation: it could only reach the same
		// verdict, after a peer sync that cannot help.
		if failure := p.inflight.failureOf(t); failure != nil {
			p.host.Log().Info("awaitDependencies: a producer failed validation",
				"txnID", t.id, "waited", time.Since(started), "cause", failure)
			return fmt.Errorf("awaitDependencies: %w: %w", errProducerFailed, failure)
		}

		p.host.Log().Debug("awaitDependencies: released by its producers",
			"txnID", t.id, "waited", time.Since(started))
		return nil

	case <-timer.C:
		// Not a failure: proceed, and let the integrity check sync what is missing.
		p.host.Log().Info("awaitDependencies: wait expired, proceeding to validation",
			"txnID", t.id, "unresolved", waitingFor, "waited", time.Since(started))
		return nil

	case <-p.ctx.Done():
		return fmt.Errorf("awaitDependencies: %w", errProcessorShuttingDown)
	}
}
