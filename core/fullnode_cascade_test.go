package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for the cascade: parking a consumer on its producer, releasing it when
// that producer commits, and the guards that stop either from going wrong.
//
// The distinction that matters throughout is between the two arrival orders. A
// producer that arrives first is found by the consumer's readiness probe and no
// parking happens at all. A producer that arrives second has to be able to find
// consumers that parked before it existed, which is the reverse edge.

// cascadeCore wires a Core whose dependency probe reports only the given IDs as
// persisted, with tiers long enough that a test failing to release shows up as a
// timeout rather than as a pass.
func cascadeCore(t *testing.T, resolved ...string) (*Core, *DynamicTxnProcessor, func()) {
	t.Helper()
	cfg := awaitTestConfig()
	cfg.inflightWait = 2 * time.Second
	cfg.unknownWait = 2 * time.Second
	return newAwaitCore(t, cfg, resolvedSet(resolved...))
}

func TestParkRecordsTheReverseEdge(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-T", "txn-S")

	if !r.park(consumer, "txn-S") {
		t.Fatal("park() returned false for a new edge")
	}
	if got := r.waitersOf("txn-S"); len(got) != 1 || got[0] != "txn-T" {
		t.Errorf("waitersOf(txn-S) = %v, want [txn-T]", got)
	}
	if consumer.pending != 1 {
		t.Errorf("pending = %d, want 1", consumer.pending)
	}
}

func TestParkRejectsDegenerateEdges(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-T")

	if r.park(nil, "txn-S") {
		t.Error("park(nil, ...) returned true")
	}
	if r.park(consumer, "") {
		t.Error("park() on an empty producer returned true")
	}
	if r.park(consumer, "txn-T") {
		t.Error("park() on itself returned true")
	}
	if r.park(newInflightEntry(""), "txn-S") {
		t.Error("park() of an entry with no ID returned true")
	}
	if got := r.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers, want 0", got)
	}
}

// A second park on the same producer is refused rather than counted, because the
// release that follows decrements once. Counting it twice would leave the waiter
// permanently one short of zero and it would never be woken.
func TestParkRefusesDuplicateEdge(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-T", "txn-S")

	if !r.park(consumer, "txn-S") {
		t.Fatal("first park() returned false")
	}
	if r.park(consumer, "txn-S") {
		t.Error("second park() on the same producer returned true")
	}
	if consumer.pending != 1 {
		t.Errorf("pending = %d after a refused duplicate, want 1", consumer.pending)
	}
}

// Past the cap the edge is refused and the transaction falls back to its timer,
// which is the behaviour that existed before the cascade. Growing the list
// without limit is what this prevents.
func TestParkEnforcesFanOutCap(t *testing.T) {
	r := newInflightRegistry()
	r.maxWaiters = 3

	for i := 0; i < r.maxWaiters; i++ {
		consumer := newInflightEntry("txn-consumer-"+string(rune('a'+i)), "txn-S")
		if !r.park(consumer, "txn-S") {
			t.Fatalf("park() %d returned false below the cap", i)
		}
	}

	overflow := newInflightEntry("txn-overflow", "txn-S")
	if r.park(overflow, "txn-S") {
		t.Error("park() past the fan-out cap returned true")
	}
	if overflow.pending != 0 {
		t.Errorf("pending = %d for a refused waiter, want 0", overflow.pending)
	}
	if got := len(r.waitersOf("txn-S")); got != r.maxWaiters {
		t.Errorf("waitersOf(txn-S) has %d entries, want the cap of %d", got, r.maxWaiters)
	}
}

// A cycle cannot arise from a real chain — a transaction's producers precede it
// — but a malformed pair claiming to produce each other would park both until
// their timers expired. Refusing the closing edge keeps at least one of them
// moving.
func TestParkRefusesEdgeThatWouldCycle(t *testing.T) {
	r := newInflightRegistry()
	a := newInflightEntry("txn-A", "txn-B")
	b := newInflightEntry("txn-B", "txn-A")

	if !r.park(a, "txn-B") {
		t.Fatal("park(A on B) returned false")
	}
	if r.park(b, "txn-A") {
		t.Error("park(B on A) returned true, closing a cycle")
	}
	if b.pending != 0 {
		t.Errorf("pending = %d for the refused edge, want 0", b.pending)
	}
}

// The guard has to follow the whole chain, not just the immediate edge: A waits
// on B, B waits on C, and C waiting on A is just as stuck for being three hops
// around.
func TestParkRefusesTransitiveCycle(t *testing.T) {
	r := newInflightRegistry()
	a := newInflightEntry("txn-A", "txn-B")
	b := newInflightEntry("txn-B", "txn-C")
	c := newInflightEntry("txn-C", "txn-A")

	if !r.park(a, "txn-B") {
		t.Fatal("park(A on B) returned false")
	}
	if !r.park(b, "txn-C") {
		t.Fatal("park(B on C) returned false")
	}
	if r.park(c, "txn-A") {
		t.Error("park(C on A) returned true, closing a three-hop cycle")
	}
}

// A diamond is not a cycle. Two transactions waiting on the same producer, and a
// third waiting on both of them, is an ordinary bundle shape and the guard must
// not mistake the repeated visit for a loop.
func TestParkAllowsDiamond(t *testing.T) {
	r := newInflightRegistry()
	left := newInflightEntry("txn-left", "txn-S")
	right := newInflightEntry("txn-right", "txn-S")
	joiner := newInflightEntry("txn-join", "txn-left", "txn-right")

	if !r.park(left, "txn-S") || !r.park(right, "txn-S") {
		t.Fatal("parking both sides on the shared producer failed")
	}
	if !r.park(joiner, "txn-left") || !r.park(joiner, "txn-right") {
		t.Fatal("parking the joiner on both sides failed")
	}
	if joiner.pending != 2 {
		t.Errorf("pending = %d, want 2", joiner.pending)
	}
}

func TestReleaseFreesWaiters(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-T", "txn-S")
	r.park(consumer, "txn-S")

	freed := r.release("txn-S")
	if len(freed) != 1 || freed[0] != consumer {
		t.Fatalf("release() freed %d waiters, want the one parked", len(freed))
	}
	if got := r.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after release, want 0", got)
	}
}

// Only the last producer frees a consumer. A transfer spending two splits has to
// see both on disk; woken after the first, it would validate against a chain
// that is still incomplete and sync the rest from a peer anyway.
func TestReleaseWaitsForEveryProducer(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-T", "txn-S1", "txn-S2")
	r.park(consumer, "txn-S1")
	r.park(consumer, "txn-S2")

	if freed := r.release("txn-S1"); len(freed) != 0 {
		t.Errorf("release of the first of two producers freed %d waiters, want 0", len(freed))
	}
	if freed := r.release("txn-S2"); len(freed) != 1 {
		t.Errorf("release of the last producer freed %d waiters, want 1", len(freed))
	}
}

// release is driven by whatever persists, so it is called for transactions with
// no waiters constantly, and can be called twice for the same producer if a
// re-delivery is validated again.
func TestReleaseIsSafeWithoutWaiters(t *testing.T) {
	r := newInflightRegistry()

	if freed := r.release("txn-unknown"); freed != nil {
		t.Errorf("release of an unknown producer returned %v, want nil", freed)
	}
	if freed := r.release(""); freed != nil {
		t.Errorf("release of an empty ID returned %v, want nil", freed)
	}

	consumer := newInflightEntry("txn-T", "txn-S")
	r.park(consumer, "txn-S")
	r.release("txn-S")
	if freed := r.release("txn-S"); len(freed) != 0 {
		t.Errorf("second release freed %d waiters, want 0", len(freed))
	}
}

// Both a release and a timeout can reach the same entry, so the close has to
// survive being asked for twice. A plain close() would panic here.
func TestMarkReadyIsIdempotent(t *testing.T) {
	entry := newInflightEntry("txn-T")

	entry.markReady()
	entry.markReady()

	select {
	case <-entry.ready:
	default:
		t.Error("ready is not closed after markReady()")
	}
}

// unpark is deferred by every waiter over the set it parked on, so it runs
// routinely for edges that release has already taken away.
func TestUnparkIsIdempotent(t *testing.T) {
	r := newInflightRegistry()
	consumer := newInflightEntry("txn-T", "txn-S1", "txn-S2")
	r.park(consumer, "txn-S1")
	r.park(consumer, "txn-S2")

	r.release("txn-S1")

	if got := r.unpark(consumer, []string{"txn-S1", "txn-S2"}); got != 0 {
		t.Errorf("unpark() left pending = %d, want 0", got)
	}
	if got := r.unpark(consumer, []string{"txn-S1", "txn-S2"}); got != 0 {
		t.Errorf("second unpark() left pending = %d, want 0", got)
	}
	if got := r.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers, want 0", got)
	}
}

// A producer with several waiters must lose only the one that unparks. Removing
// from the middle of the list is where an index slip would silently drop
// somebody else's edge.
func TestUnparkRemovesOnlyItsOwnEdge(t *testing.T) {
	r := newInflightRegistry()
	first := newInflightEntry("txn-1", "txn-S")
	second := newInflightEntry("txn-2", "txn-S")
	third := newInflightEntry("txn-3", "txn-S")
	for _, w := range []*inflightTxn{first, second, third} {
		r.park(w, "txn-S")
	}

	r.unpark(second, []string{"txn-S"})

	got := r.waitersOf("txn-S")
	if len(got) != 2 || got[0] != "txn-1" || got[1] != "txn-3" {
		t.Errorf("waitersOf(txn-S) = %v, want [txn-1 txn-3]", got)
	}
	if freed := r.release("txn-S"); len(freed) != 2 {
		t.Errorf("release freed %d waiters, want the 2 still parked", len(freed))
	}
	if second.pending != 0 {
		t.Errorf("the unparked waiter has pending = %d, want 0", second.pending)
	}
}

// Producer first: the ordinary case. The consumer's probe finds the row, so it
// never parks and never waits.
func TestAwaitDependenciesProducerAlreadyPersistedDoesNotPark(t *testing.T) {
	c, p, cancel := cascadeCore(t, "txn-S")
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v for a producer already on disk", elapsed)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers, want 0 — nothing should have parked", got)
	}
}

// Consumer first, the reverse edge: the transfer arrives before the split it
// spends, parks, and is woken by the split's persist rather than by its own
// timer. Without the cascade this costs the full wait and then a peer sync.
func TestAwaitDependenciesWokenByProducerPersist(t *testing.T) {
	c, p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	go func() {
		// Long enough that the wait is genuinely under way, short enough that a
		// missed release shows up as the 2s timeout instead of a pass.
		time.Sleep(30 * time.Millisecond)
		c.releaseWaiters("txn-S")
	}()

	start := time.Now()
	if err := c.awaitDependencies(consumer); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	elapsed := time.Since(start)
	if elapsed >= p.bundle.unknownWait {
		t.Errorf("waited %v, i.e. the full timer; the producer's persist should have woken it", elapsed)
	}
	if elapsed < 25*time.Millisecond {
		t.Errorf("returned after %v, before the producer persisted", elapsed)
	}
	if got := atomic.LoadInt64(&p.cascadeReleases); got != 1 {
		t.Errorf("cascadeReleases = %d, want 1", got)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after the release, want 0", got)
	}
}

// The reverse edge has to survive the producer being registered in between: the
// split arrives, is seen to have a waiter, and only wakes it once it persists.
func TestAwaitDependenciesReverseEdgeSurvivesProducerRegistration(t *testing.T) {
	c, p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	go func() {
		time.Sleep(20 * time.Millisecond)
		if entry := c.registerInflight(eventWithDeps("txn-S")); entry == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
		c.releaseWaiters("txn-S")
	}()

	if err := c.awaitDependencies(consumer); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if got := atomic.LoadInt64(&p.revEdges); got != 1 {
		t.Errorf("revEdges = %d, want 1 — the producer should have seen its waiter on arrival", got)
	}
}

// A consumer waiting on two producers resumes on the second persist, not the
// first.
func TestAwaitDependenciesWaitsForEveryProducer(t *testing.T) {
	c, p, cancel := cascadeCore(t)
	defer cancel()

	var firstReleased atomic.Int64
	go func() {
		time.Sleep(20 * time.Millisecond)
		c.releaseWaiters("txn-S1")
		firstReleased.Store(time.Now().UnixNano())
		time.Sleep(30 * time.Millisecond)
		c.releaseWaiters("txn-S2")
	}()

	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S1", "txn-S2")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	returned := time.Now().UnixNano()
	if got := firstReleased.Load(); got == 0 || returned-got < int64(20*time.Millisecond) {
		t.Error("returned on the first producer's release; both producers must persist first")
	}
	if got := atomic.LoadInt64(&p.cascadeReleases); got != 1 {
		t.Errorf("cascadeReleases = %d, want 1 — only the last producer frees the waiter", got)
	}
}

// A producer that never arrives leaves the waiter to its timer, which is exactly
// the pre-cascade behaviour, and the edge must not outlive the wait.
func TestAwaitDependenciesTimesOutWhenProducerNeverArrives(t *testing.T) {
	c, p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < p.bundle.unknownWait {
		t.Errorf("returned after %v, want at least the %v timer", elapsed, p.bundle.unknownWait)
	}
	if got := atomic.LoadInt64(&p.cascadeReleases); got != 0 {
		t.Errorf("cascadeReleases = %d, want 0", got)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after the timeout, want 0", got)
	}
}

// A cycle must not park either side indefinitely: the first edge is recorded,
// the closing one is refused, and the refused transaction proceeds without
// waiting at all.
func TestAwaitDependenciesProceedsRatherThanClosingACycle(t *testing.T) {
	c, p, cancel := cascadeCore(t)
	defer cancel()

	a := newInflightEntry("txn-A", "txn-B")
	if !p.inflight.park(a, "txn-B") {
		t.Fatal("park(A on B) returned false")
	}

	start := time.Now()
	if err := c.awaitDependencies(newInflightEntry("txn-B", "txn-A")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v; an edge that would cycle must not be waited on", elapsed)
	}
}

// releaseWaiters is called from processSingleTransaction on a Core that may have
// no processor at all in a non-fullnode configuration.
func TestReleaseWaitersToleratesNoProcessor(t *testing.T) {
	(&Core{}).releaseWaiters("txn-S")
}

// Fifty pairs racing, each consumer parking while its own producer releases. Run
// with -race: the registry is the only thing serialising park, release and
// unpark, and the whole design rests on that being true.
func TestCascadeIsSafeUnderConcurrency(t *testing.T) {
	const pairs = 50

	c, p, cancel := cascadeCore(t)
	defer cancel()
	p.bundle.maxParked = pairs * 2
	p.bundle.unknownWait = 500 * time.Millisecond
	p.bundle.inflightWait = 500 * time.Millisecond

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(pairs * 2)

	for i := 0; i < pairs; i++ {
		producerID := "txn-producer-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		consumerID := "txn-consumer-" + string(rune('a'+i%26)) + string(rune('0'+i/26))

		go func() { // consumer: park and wait
			defer done.Done()
			start.Wait()
			if err := c.awaitDependencies(newInflightEntry(consumerID, producerID)); err != nil {
				t.Errorf("awaitDependencies(%s) = %v, want nil", consumerID, err)
			}
		}()
		go func() { // producer: persist and release, racing the park above
			defer done.Done()
			start.Wait()
			c.releaseWaiters(producerID)
		}()
	}

	start.Done()
	done.Wait()

	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers after every pair finished, want 0", got)
	}
	if got := atomic.LoadInt64(&p.parkedCount); got != 0 {
		t.Errorf("parkedCount = %d, want 0", got)
	}
}
