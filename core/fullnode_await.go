package core

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// The readiness gate.
//
// A transfer that spends the output of a split declares that split's ID as the
// PreviousTransactionID of the token it moves. If the fullnode validates the
// transfer before it has persisted the split, the integrity check finds a token
// it has never seen and pulls the whole chain from a peer to catch up.
//
// Holding the transfer until its producers are actually persisted removes that
// round trip. When the wait expires the transaction proceeds anyway, straight
// into the existing validate-and-sync path, so the gate can only ever save work
// — never block a transaction permanently.
//
// The hold is released by the producer itself: a transaction that commits wakes
// everything parked on it (see releaseWaiters). The timer is only the backstop
// for a producer that never arrives, so a transfer held behind a split now
// resumes within microseconds of that split's commit rather than on the next
// tick of a poll.

// errProcessorShuttingDown reports that the wait was abandoned because the
// processor is stopping, not because the dependencies resolved.
var errProcessorShuttingDown = errors.New("transaction processor is shutting down")

// dependencyResolved reports whether depID is durably persisted on this node.
//
// "Resolved" means present in the fullnode's own transaction table. An empty ID
// is a genesis entry, which has no producer to wait for and is resolved by
// definition.
//
// The error return exists to keep a database problem distinguishable from an
// absent producer. Both used to look the same from here, and conflating them
// would park every transaction during an outage.
func (c *Core) dependencyResolved(depID string) (bool, error) {
	if depID == "" {
		return true, nil
	}
	if _, err := c.w.GetTransactionByID(depID, true); err != nil {
		if errors.Is(err, wallet.ErrTransactionNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// partitionDependencies splits deps into those already persisted and those not.
//
// A dependency whose lookup failed is reported as resolved. That is deliberate:
// the point of waiting is to avoid a chain sync we know is unnecessary, and a
// lookup that did not answer tells us nothing. Proceeding sends the transaction
// down the path it would have taken anyway; parking it would convert a database
// blip into a stalled pipeline.
//
// Both halves are returned because the caller needs both: the resolved half to
// give back the edges it has just parked on, the unresolved half to pick how
// long to wait.
func (c *Core) partitionDependencies(deps []string) (resolved, unresolved []string) {
	for _, dep := range deps {
		ok, err := c.txnProcessor.resolveDependency(dep)
		if err != nil {
			c.log.Warn("awaitDependencies: could not check a dependency, treating it as resolved",
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

// dependencyWait picks how long to wait, taking the longest applicable tier.
//
// A producer that is currently in flight is worth waiting for, because it is
// going to resolve. A producer that is simply absent may never arrive — this
// node may have joined the network after it was published — and gets a much
// shorter grace period.
func (c *Core) dependencyWait(unresolved []string) time.Duration {
	cfg := c.txnProcessor.bundle
	var wait time.Duration
	for _, dep := range unresolved {
		tier := cfg.unknownWait
		if c.txnProcessor.inflight.has(dep) {
			tier = cfg.inflightWait
		}
		if tier > wait {
			wait = tier
		}
	}
	return wait
}

// awaitDependencies blocks until every producer t declares is persisted, the
// wait expires, or the processor shuts down.
//
// It returns an error only for shutdown. An expired wait is a normal outcome and
// returns nil: the transaction then runs the ordinary validation path, whose
// integrity check syncs whatever is missing, exactly as it did before this gate
// existed.
//
// Called once, before the retry loop rather than inside it. Waiting per attempt
// would multiply the hold by maxRetries for a transaction whose producer never
// shows up.
func (c *Core) awaitDependencies(t *inflightTxn) error {
	p := c.txnProcessor
	if t == nil || len(t.deps) == 0 {
		return nil
	}

	_, unresolved := c.partitionDependencies(t.deps)
	if len(unresolved) == 0 {
		return nil
	}

	// Bound how much of the worker pool can be waiting at once. Past the cap the
	// gate stops holding anything: degrading to the old behaviour is much better
	// than starving the pool.
	parked := atomic.AddInt64(&p.parkedCount, 1)
	if p.bundle.maxParked > 0 && parked > int64(p.bundle.maxParked) {
		atomic.AddInt64(&p.parkedCount, -1)
		c.log.Warn("awaitDependencies: too many transactions already waiting, proceeding without holding",
			"txnID", t.id, "parked", parked-1, "maxParked", p.bundle.maxParked)
		return nil
	}
	defer atomic.AddInt64(&p.parkedCount, -1)

	// Record the reverse edges before waiting on anything. From here on, a
	// producer that commits finds this transaction and wakes it directly, which
	// is what replaces re-checking the database on a ticker.
	parkedOn := make([]string, 0, len(unresolved))
	for _, dep := range unresolved {
		if p.inflight.park(t, dep) {
			parkedOn = append(parkedOn, dep)
			continue
		}
		c.log.Debug("awaitDependencies: declined to park on a producer, this dependency can only time out",
			"txnID", t.id, "dependsOn", dep)
	}
	if len(parkedOn) == 0 {
		// A cycle, or the fan-out cap on every dependency. Nothing can wake this
		// transaction, so waiting could only ever expire.
		return nil
	}
	defer p.inflight.unpark(t, parkedOn)

	// Close the gap between the first probe and the parking that followed it. A
	// producer that committed in between released nobody, because there was
	// nothing yet to release, and a ready channel closes once and is never
	// rearmed — so missing that wake-up would cost the full wait.
	//
	// Re-probing after parking is what makes it impossible to miss: release
	// strictly follows the commit, so a producer that committed either shows up
	// in this probe or finds the edge already recorded.
	started := time.Now()
	resolved, waitingFor := c.partitionDependencies(parkedOn)
	if len(resolved) > 0 {
		if remaining := p.inflight.unpark(t, resolved); remaining == 0 {
			c.log.Debug("awaitDependencies: producers resolved while the edges were being recorded",
				"txnID", t.id)
			return nil
		}
	}
	if len(waitingFor) == 0 {
		return nil
	}

	wait := c.dependencyWait(waitingFor)
	c.log.Debug("awaitDependencies: holding transaction until its producers resolve",
		"txnID", t.id, "unresolved", waitingFor, "wait", wait)

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-t.ready:
		c.log.Debug("awaitDependencies: released by its producers",
			"txnID", t.id, "waited", time.Since(started))
		return nil

	case <-timer.C:
		// Not a failure. The transaction proceeds and the integrity check syncs
		// what is missing, which is what would have happened immediately without
		// this gate.
		c.log.Info("awaitDependencies: wait expired, proceeding to validation",
			"txnID", t.id, "unresolved", waitingFor, "waited", time.Since(started))
		return nil

	case <-p.ctx.Done():
		return fmt.Errorf("awaitDependencies: %w", errProcessorShuttingDown)
	}
}
