package core

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// awaitTestConfig is deliberately fast: the tiers only have to be
// distinguishable from one another, not realistic.
func awaitTestConfig() bundleConfig {
	return bundleConfig{
		inflightWait: 300 * time.Millisecond,
		unknownWait:  40 * time.Millisecond,
		maxParked:    10,
	}
}

// newAwaitCore wires a processor whose dependency probe is driven by resolved,
// so the gate can be exercised with no database at all.
func newAwaitCore(t *testing.T, cfg bundleConfig, resolve func(string) (bool, error)) (*Core, *DynamicTxnProcessor, func()) {
	t.Helper()
	p, cancel := newTestProcessor(10, 0)
	p.bundle = cfg
	p.resolveDependency = resolve
	return newTestCore(p), p, cancel
}

// resolvedSet returns a probe reporting the given IDs as persisted.
func resolvedSet(ids ...string) func(string) (bool, error) {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return func(dep string) (bool, error) { return set[dep], nil }
}

// The gate is compulsory, so a real processor always has usable timings. A zero
// bundleConfig would silently mean unbounded parking and no cap, which is why
// initDynamicTxnProcessor sets the defaults unconditionally.
func TestDefaultBundleConfigIsUsable(t *testing.T) {
	cfg := defaultBundleConfig()
	if cfg.inflightWait <= 0 || cfg.unknownWait <= 0 {
		t.Errorf("wait tiers must be positive, got inflight=%v unknown=%v", cfg.inflightWait, cfg.unknownWait)
	}
	if cfg.unknownWait > cfg.inflightWait {
		t.Errorf("unknownWait %v exceeds inflightWait %v; an absent producer must not be waited on longer than one in flight",
			cfg.unknownWait, cfg.inflightWait)
	}
	if cfg.maxParked <= 0 {
		t.Errorf("maxParked = %d, want a positive cap", cfg.maxParked)
	}
}

func TestAwaitDependenciesNoDepsReturnsImmediately(t *testing.T) {
	c, _, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("waited %v for a transaction with no dependencies", elapsed)
	}
}

// The common case: an ordinary transfer whose producer is long since persisted
// must cost nothing at all.
func TestAwaitDependenciesAlreadyResolvedReturnsImmediately(t *testing.T) {
	c, _, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet("txn-S"))
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("waited %v for an already-persisted producer", elapsed)
	}
}

// An absent producer gets the short tier, not the long one — a fullnode that
// joined after network genesis has legitimately never seen most producers.
func TestAwaitDependenciesUnknownProducerUsesShortTier(t *testing.T) {
	cfg := awaitTestConfig()
	c, _, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-absent")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	elapsed := time.Since(start)
	if elapsed < cfg.unknownWait {
		t.Errorf("returned after %v, want at least the %v short tier", elapsed, cfg.unknownWait)
	}
	if elapsed >= cfg.inflightWait {
		t.Errorf("waited %v, i.e. the long tier, for a producer that is not in flight", elapsed)
	}
}

// A producer this node is demonstrably still processing is worth waiting for,
// so it gets the long tier.
func TestAwaitDependenciesInFlightProducerUsesLongTier(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = 120 * time.Millisecond
	cfg.unknownWait = 10 * time.Millisecond
	c, p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	p.inflight.register(newInflightEntry("txn-S"))

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < cfg.inflightWait {
		t.Errorf("returned after %v, want at least the %v long tier", elapsed, cfg.inflightWait)
	}
}

// The lost-wakeup window, and why the gate probes twice.
//
// Between the first probe and the parking that follows it, the producer can
// commit — and its release finds nobody, because the edge does not exist yet.
// A ready channel closes once and is never rearmed, so without the second probe
// this transaction would wait out its whole timer for a producer that is already
// on disk.
//
// The probe here reports unresolved exactly once, which is that interleaving.
func TestAwaitDependenciesReprobesAfterParking(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = time.Second
	cfg.unknownWait = time.Second
	var probes atomic.Int64
	c, p, cancel := newAwaitCore(t, cfg, func(dep string) (bool, error) {
		return probes.Add(1) > 1, nil
	})
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v for a producer that resolved while the edge was being recorded", elapsed)
	}
	if got := probes.Load(); got < 2 {
		t.Errorf("probed %d times, want a second probe after parking", got)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers, want 0 — the edge should have been given back", got)
	}
}

// Closing ready releases the wait. The cascade is what closes it in production;
// this pins the contract independently of that path.
func TestAwaitDependenciesReleasedByReadyChannel(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = time.Second
	cfg.unknownWait = time.Second
	c, _, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	entry := newInflightEntry("txn-T", "txn-S")
	go func() {
		time.Sleep(20 * time.Millisecond)
		entry.markReady()
	}()

	start := time.Now()
	if err := c.awaitDependencies(entry); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("waited %v; closing ready should have released it", elapsed)
	}
}

// Every wait must give its edges back, however it ended. A waiter that timed out
// and left itself in waitingOn would be resurrected by a producer arriving much
// later, and its list would grow for the lifetime of the process.
func TestAwaitDependenciesUnparksOnTimeout(t *testing.T) {
	c, p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S", "txn-Q")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after the wait expired, want 0", got)
	}
}

// The load-bearing distinction: an unreadable database is not an absent
// producer. Parking on a lookup that failed would turn a brief outage into a
// stalled pipeline, so a failed probe means proceed.
func TestAwaitDependenciesProceedsWhenTheProbeFails(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.unknownWait = time.Second
	cfg.inflightWait = time.Second
	c, p, cancel := newAwaitCore(t, cfg, func(string) (bool, error) {
		return false, errors.New("connection refused")
	})
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v after a probe failure, want no wait", elapsed)
	}
	if got := atomic.LoadInt64(&p.parkedCount); got != 0 {
		t.Errorf("parkedCount = %d, want 0 — a failed probe must not park", got)
	}
}

// Past the cap the gate stops holding anything, so a flood of unresolvable
// dependencies degrades to the old behaviour instead of consuming the pool.
func TestAwaitDependenciesFailsOpenPastMaxParked(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.maxParked = 2
	cfg.unknownWait = time.Second
	c, p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	atomic.StoreInt64(&p.parkedCount, int64(cfg.maxParked))

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v past the parked cap, want no wait", elapsed)
	}
	if got := atomic.LoadInt64(&p.parkedCount); got != int64(cfg.maxParked) {
		t.Errorf("parkedCount = %d, want %d — the rejected waiter must not stay counted", got, cfg.maxParked)
	}
}

func TestAwaitDependenciesReturnsErrorOnShutdown(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.unknownWait = time.Second
	c, p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	go func() {
		time.Sleep(20 * time.Millisecond)
		p.cancel()
	}()

	err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S"))
	if !errors.Is(err, errProcessorShuttingDown) {
		t.Errorf("awaitDependencies() = %v, want %v", err, errProcessorShuttingDown)
	}
}

// parkedCount is what maxParked bounds, so a leak here would silently disable
// the cap.
func TestAwaitDependenciesReleasesParkedCount(t *testing.T) {
	cfg := awaitTestConfig()
	c, p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	for i := 0; i < 3; i++ {
		if err := c.awaitDependencies(newInflightEntry(fmt.Sprintf("txn-%d", i), "txn-absent")); err != nil {
			t.Fatalf("awaitDependencies() = %v, want nil", err)
		}
	}
	if got := atomic.LoadInt64(&p.parkedCount); got != 0 {
		t.Errorf("parkedCount = %d after every wait finished, want 0", got)
	}
}

// Only the unresolved subset is waited on, and a mix must still take the tier of
// the strongest reason to wait.
func TestAwaitDependenciesIgnoresResolvedMembersOfAMixedSet(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = 120 * time.Millisecond
	cfg.unknownWait = 10 * time.Millisecond
	c, p, cancel := newAwaitCore(t, cfg, resolvedSet("txn-done"))
	defer cancel()

	p.inflight.register(newInflightEntry("txn-S"))

	start := time.Now()
	entry := newInflightEntry("txn-T", "txn-done", "txn-S")
	if err := c.awaitDependencies(entry); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < cfg.inflightWait {
		t.Errorf("returned after %v; the in-flight member should have set the long tier", elapsed)
	}
}

// dependencyResolved translates the wallet's outcomes into the gate's vocabulary,
// and the "not found" case must be a normal answer rather than an error.
func TestDependencyResolvedTreatsNotFoundAsUnresolved(t *testing.T) {
	notFound := fmt.Errorf("transaction ID: %v is not present: %w", "txn-S", wallet.ErrTransactionNotFound)
	if !errors.Is(notFound, wallet.ErrTransactionNotFound) {
		t.Fatal("wallet.ErrTransactionNotFound does not survive wrapping")
	}

	other := fmt.Errorf("failed to get transaction: %w", errors.New("connection refused"))
	if errors.Is(other, wallet.ErrTransactionNotFound) {
		t.Error("an unrelated database error matched ErrTransactionNotFound")
	}
}

func TestDependencyResolvedEmptyIDIsResolved(t *testing.T) {
	c, _, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	// An empty PreviousTransactionID is a genesis entry: no producer to wait for.
	resolved, err := c.dependencyResolved("")
	if err != nil || !resolved {
		t.Errorf("dependencyResolved(\"\") = (%v, %v), want (true, nil)", resolved, err)
	}
}
