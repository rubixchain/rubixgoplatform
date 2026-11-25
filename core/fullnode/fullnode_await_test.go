package fullnode

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// awaitTestConfig uses short waits to keep tests fast; the two wait tiers only
// need to be told apart.
func awaitTestConfig() bundleConfig {
	return bundleConfig{
		inflightWait: 300 * time.Millisecond,
		unknownWait:  40 * time.Millisecond,
	}
}

// newAwaitCore builds a processor whose dependency probe is resolve, so the
// readiness gate runs without a database.
func newAwaitCore(t *testing.T, cfg bundleConfig, resolve func(string) (bool, error)) (*DynamicTxnProcessor, func()) {
	t.Helper()
	p, cancel := newTestProcessor(10, 0)
	p.bundle = cfg
	p.resolveDependency = resolve
	return p, cancel
}

// resolvedSet returns a probe reporting the given IDs as persisted.
func resolvedSet(ids ...string) func(string) (bool, error) {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return func(dep string) (bool, error) { return set[dep], nil }
}

// The readiness gate is always on, so the defaults must be usable: a zero wait
// would expire at once, and an absent previous transaction must not be waited
// on longer than one this node is processing.
func TestDefaultBundleConfigIsUsable(t *testing.T) {
	cfg := defaultBundleConfig()
	if cfg.inflightWait <= 0 || cfg.unknownWait <= 0 {
		t.Errorf("wait tiers must be positive, got inflight=%v unknown=%v", cfg.inflightWait, cfg.unknownWait)
	}
	if cfg.unknownWait > cfg.inflightWait {
		t.Errorf("unknownWait %v exceeds inflightWait %v; an absent producer must not be waited on longer than one in flight",
			cfg.unknownWait, cfg.inflightWait)
	}
}

func TestAwaitDependenciesNoDepsReturnsImmediately(t *testing.T) {
	p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("waited %v for a transaction with no dependencies", elapsed)
	}
}

// An absent previous transaction gets the short wait: a fullnode that joined
// after network genesis has never seen most previous transactions.
func TestAwaitDependenciesUnknownProducerUsesShortTier(t *testing.T) {
	cfg := awaitTestConfig()
	p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-absent")); err != nil {
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

// A previous transaction this node is still processing will resolve, so it
// gets the long wait.
func TestAwaitDependenciesInFlightProducerUsesLongTier(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = 120 * time.Millisecond
	cfg.unknownWait = 10 * time.Millisecond
	p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	p.inflight.register(newInflightEntry("txn-S"))

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < cfg.inflightWait {
		t.Errorf("returned after %v, want at least the %v long tier", elapsed, cfg.inflightWait)
	}
}

// Guards against a lost wake-up: the previous transaction can be persisted
// between the first probe and parking, when there is no edge to release yet.
// The gate re-probes after parking. This probe reports unresolved exactly once
// to reproduce that interleaving.
func TestAwaitDependenciesReprobesAfterParking(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = time.Second
	cfg.unknownWait = time.Second
	var probes atomic.Int64
	p, cancel := newAwaitCore(t, cfg, func(dep string) (bool, error) {
		return probes.Add(1) > 1, nil
	})
	defer cancel()

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
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

// A failed database lookup is not an absent previous transaction. Waiting on it
// would turn a brief outage into a stalled pipeline, so the gate proceeds.
func TestAwaitDependenciesProceedsWhenTheProbeFails(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.unknownWait = time.Second
	cfg.inflightWait = time.Second
	p, cancel := newAwaitCore(t, cfg, func(string) (bool, error) {
		return false, errors.New("connection refused")
	})
	defer cancel()

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v after a probe failure, want no wait", elapsed)
	}
	if got := atomic.LoadInt64(&p.parkedCount); got != 0 {
		t.Errorf("parkedCount = %d, want 0 — a failed probe must not park", got)
	}
}

func TestAwaitDependenciesReturnsErrorOnShutdown(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.unknownWait = time.Second
	p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	go func() {
		time.Sleep(20 * time.Millisecond)
		p.cancel()
	}()

	err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S"))
	if !errors.Is(err, errProcessorShuttingDown) {
		t.Errorf("awaitDependencies() = %v, want %v", err, errProcessorShuttingDown)
	}
}

// parkedCount is reported on the metrics line, so a leak would show a stuck
// wait that does not exist.
func TestAwaitDependenciesReleasesParkedCount(t *testing.T) {
	cfg := awaitTestConfig()
	p, cancel := newAwaitCore(t, cfg, resolvedSet())
	defer cancel()

	for i := 0; i < 3; i++ {
		if err := p.awaitDependencies(newInflightEntry(fmt.Sprintf("txn-%d", i), "txn-absent")); err != nil {
			t.Fatalf("awaitDependencies() = %v, want nil", err)
		}
	}
	if got := atomic.LoadInt64(&p.parkedCount); got != 0 {
		t.Errorf("parkedCount = %d after every wait finished, want 0", got)
	}
}

// With a mix of dependencies, only the unresolved ones are waited on, and the
// longest applicable wait is used.
func TestAwaitDependenciesIgnoresResolvedMembersOfAMixedSet(t *testing.T) {
	cfg := awaitTestConfig()
	cfg.inflightWait = 120 * time.Millisecond
	cfg.unknownWait = 10 * time.Millisecond
	p, cancel := newAwaitCore(t, cfg, resolvedSet("txn-done"))
	defer cancel()

	p.inflight.register(newInflightEntry("txn-S"))

	start := time.Now()
	entry := newInflightEntry("txn-T", "txn-done", "txn-S")
	if err := p.awaitDependencies(entry); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < cfg.inflightWait {
		t.Errorf("returned after %v; the in-flight member should have set the long tier", elapsed)
	}
}

func TestDependencyResolvedEmptyIDIsResolved(t *testing.T) {
	p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	// An empty PreviousTransactionID is a genesis entry: nothing to wait for.
	resolved, err := p.dependencyResolved("")
	if err != nil || !resolved {
		t.Errorf("dependencyResolved(\"\") = (%v, %v), want (true, nil)", resolved, err)
	}
}
