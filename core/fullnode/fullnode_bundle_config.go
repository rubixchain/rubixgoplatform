package fullnode

import "time"

// bundleConfig holds the knobs for the dependency-aware ingest path.
//
// The behaviour is always on: there is no switch to turn it off, so the defaults
// have to be safe rather than merely reasonable. Holding a transaction costs a
// worker for the duration of the wait, and the pool is only max(1, NumCPU/2)
// workers to begin with, so they are chosen to make the common case cost nothing
// and the uncommon case cost a second.
//
// Every wait is bounded and every expiry falls through to the behaviour that
// existed before the gate, so the worst case is the old behaviour plus the wait.
type bundleConfig struct {
	// inflightWait is how long to wait for a producer this node is demonstrably
	// still processing. Worth waiting for: it is going to resolve.
	inflightWait time.Duration

	// unknownWait is how long to wait for a producer that is simply not here.
	// Deliberately much shorter — a fullnode that joined after network genesis
	// has legitimately never seen most producers, and treating those the same as
	// an in-flight one would add the full wait to every cold token.
	unknownWait time.Duration

	// maxParked caps how many transactions may be waiting at once. Past the cap
	// the gate fails open and transactions proceed unheld, so a flood of
	// unresolvable dependencies degrades to today's behaviour instead of
	// consuming the worker pool.
	maxParked int

	// syncMemoTTL is how long a successful chain sync is remembered, so a later
	// member of the same bundle does not repeat it.
	//
	// Short on purpose. The memo can only ever suppress a sync that would have
	// been redundant, and the integrity check re-reads every token from the
	// database afterwards regardless, so a wrong suppression fails loudly rather
	// than persisting bad data. A few seconds covers a bundle arriving together
	// without keeping opinions about a chain long enough for them to go stale.
	syncMemoTTL time.Duration
}

func defaultBundleConfig() bundleConfig {
	return bundleConfig{
		inflightWait: 5 * time.Second,
		unknownWait:  1 * time.Second,
		maxParked:    1000,
		syncMemoTTL:  5 * time.Second,
	}
}
