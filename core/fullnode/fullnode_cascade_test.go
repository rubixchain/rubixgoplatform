package fullnode

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for parking a waiting transaction on its previous transaction and waking
// it when that one is persisted. A previous transaction that arrives first is
// found by the probe, so nothing parks; one that arrives second must find the
// transactions that parked before it existed (the reverse edge).

// cascadeCore reports only the given IDs as persisted. The 2s waits make a
// missed wake-up show up as a full timeout rather than a pass.
func cascadeCore(t *testing.T, resolved ...string) (*DynamicTxnProcessor, func()) {
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

// A duplicate park is refused, not counted: release decrements once, so a
// double count would leave the waiter never woken.
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

// Past the per-transaction fan-out cap (maxWaiters), park is refused and the
// waiter falls back to its timer instead of growing the list without limit.
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

// A real chain cannot form a cycle, but two malformed transactions naming each
// other would both wait out their timers. Refusing the closing edge keeps one
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

// The cycle check follows the whole chain: A on B, B on C, then C on A is a
// three-hop cycle.
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

// A diamond (two transactions on one previous transaction, a third on both) is
// a normal bundle shape, not a cycle.
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

// Only the last previous transaction frees the waiter: woken early, a transfer
// spending two splits would validate against an incomplete chain.
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

// release runs for every persisted transaction, usually with no waiters, and
// may run twice if a re-delivery is validated again.
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

// A release and a failure propagation can both close the same entry, so
// markReady must survive a second call; a plain close() would panic.
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

// Every waiter defers unpark over all its edges, so it routinely runs for edges
// release already removed.
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

// Unparking from the middle of a waiter list must remove only that waiter's
// edge.
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

// Previous transaction first (the common case): the probe finds it on disk, so
// nothing parks or waits.
func TestAwaitDependenciesProducerAlreadyPersistedDoesNotPark(t *testing.T) {
	p, cancel := cascadeCore(t, "txn-S")
	defer cancel()

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v for a producer already on disk", elapsed)
	}
	if got := p.inflight.waitingLen(); got != 0 {
		t.Errorf("waitingOn holds %d producers, want 0 — nothing should have parked", got)
	}
}

// Waiting transaction first: it parks and is woken by the previous
// transaction's persist, not its timer. Otherwise it would wait the full
// timeout and then sync from a peer.
func TestAwaitDependenciesWokenByProducerPersist(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	go func() {
		// Long enough that the wait is genuinely under way, short enough that a
		// missed release shows up as the 2s timeout instead of a pass.
		time.Sleep(30 * time.Millisecond)
		p.releaseWaiters("txn-S")
	}()

	start := time.Now()
	if err := p.awaitDependencies(consumer); err != nil {
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

// The edge survives the previous transaction registering in between:
// registration counts the waiter (revEdges), and only the persist wakes it.
func TestAwaitDependenciesReverseEdgeSurvivesProducerRegistration(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	consumer := newInflightEntry("txn-T", "txn-S")
	go func() {
		time.Sleep(20 * time.Millisecond)
		if entry := p.registerInflight(eventWithDeps("txn-S")); entry == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
		p.releaseWaiters("txn-S")
	}()

	if err := p.awaitDependencies(consumer); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if got := atomic.LoadInt64(&p.revEdges); got != 1 {
		t.Errorf("revEdges = %d, want 1 — the producer should have seen its waiter on arrival", got)
	}
}

// A transaction waiting on two previous transactions resumes on the second
// persist, not the first.
func TestAwaitDependenciesWaitsForEveryProducer(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	var firstReleased atomic.Int64
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.releaseWaiters("txn-S1")
		firstReleased.Store(time.Now().UnixNano())
		time.Sleep(30 * time.Millisecond)
		p.releaseWaiters("txn-S2")
	}()

	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S1", "txn-S2")); err != nil {
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

// A previous transaction that never arrives leaves the waiter to its timer, and
// the edge must not outlive the wait.
func TestAwaitDependenciesTimesOutWhenProducerNeverArrives(t *testing.T) {
	p, cancel := newAwaitCore(t, awaitTestConfig(), resolvedSet())
	defer cancel()

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-T", "txn-S", "txn-Q")); err != nil {
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

// The edge that would close a cycle is refused, and that transaction proceeds
// without waiting.
func TestAwaitDependenciesProceedsRatherThanClosingACycle(t *testing.T) {
	p, cancel := cascadeCore(t)
	defer cancel()

	a := newInflightEntry("txn-A", "txn-B")
	if !p.inflight.park(a, "txn-B") {
		t.Fatal("park(A on B) returned false")
	}

	start := time.Now()
	if err := p.awaitDependencies(newInflightEntry("txn-B", "txn-A")); err != nil {
		t.Fatalf("awaitDependencies() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("waited %v; an edge that would cycle must not be waited on", elapsed)
	}
}

// releaseWaiters must tolerate a nil processor.
func TestReleaseWaitersToleratesNoProcessor(t *testing.T) {
	(*DynamicTxnProcessor)(nil).releaseWaiters("txn-S")
}

// Fifty pairs race a park against its previous transaction's release. Run with
// -race: the registry mutex alone serialises park, release and unpark.
func TestCascadeIsSafeUnderConcurrency(t *testing.T) {
	const pairs = 50

	p, cancel := cascadeCore(t)
	defer cancel()
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
			if err := p.awaitDependencies(newInflightEntry(consumerID, producerID)); err != nil {
				t.Errorf("awaitDependencies(%s) = %v, want nil", consumerID, err)
			}
		}()
		go func() { // producer: persist and release, racing the park above
			defer done.Done()
			start.Wait()
			p.releaseWaiters(producerID)
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
