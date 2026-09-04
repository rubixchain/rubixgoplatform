package fullnode

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// DynamicTxnProcessor handles adaptive concurrent transaction processing
type DynamicTxnProcessor struct {
	// host is the node this pipeline runs inside; see Host.
	host Host

	txnQueue      chan *models.EventTransaction
	processedTxns sync.Map
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	// Dynamic scaling configuration
	minWorkers     int
	maxWorkers     int
	currentWorkers int
	workersMutex   sync.RWMutex

	// System monitoring
	cpuThreshold    float64
	memoryThreshold float64
	queueThreshold  int
	scaleUpDelay    time.Duration
	scaleDownDelay  time.Duration
	lastScaleAction time.Time

	// Received-but-unresolved transactions. Distinct from processedTxns, which
	// is a seen-recently set rather than a live one.
	inflight *inflightRegistry

	// Dependency-aware ingest settings. Always active; see bundleConfig.
	bundle bundleConfig

	// resolveDependency reports whether a producer transaction is persisted.
	// A field rather than a direct call so the readiness gate can be tested
	// without a database; initDynamicTxnProcessor points it at the real probe.
	resolveDependency func(depID string) (bool, error)

	// syncMemo records which chain syncs a bundle has already performed, so its
	// later members do not repeat them.
	syncMemo *syncedTokenMemo

	// syncChains fetches token chains from a peer. A field for the same reason
	// resolveDependency is one: the memo's rule is to mark on success only, and
	// that rule cannot be exercised against a real peer.
	syncChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error

	// Metrics
	queueLength        int64
	averageProcessTime time.Duration
	processedTxnCount  int64 // ADD ATOMIC COUNTER

	// Bundling observation counters, read via sync/atomic. depsObserved counts
	// every declared PreviousTransactionID edge; depsInFlight counts the subset
	// whose producer was still being processed when the consumer arrived. Their
	// ratio is what sizes the readiness gate. parkedCount is how many
	// transactions are waiting on a producer right now, and is what maxParked
	// bounds.
	//
	// revEdges counts arrivals that found transactions already parked on them —
	// the out-of-order case. cascadeReleases counts waiters woken by a producer
	// committing rather than by their own timer; the gap between it and the
	// number that parked is how many holds ended in a timeout, which is the
	// number that says whether the wait tiers are set correctly.
	//
	// syncsIssued and syncsSkipped count tokens, not calls, so they are directly
	// comparable: of every token the integrity check wanted fetched, skipped is
	// the share the bundle had already fetched from that same peer. That ratio is
	// the entire measurable effect of the sync-once gate.
	//
	// failuresPropagated counts transactions abandoned because a producer of
	// theirs was found invalid. It should be rare, and it is the number to look
	// at first if good transactions start being dead-lettered: every one of them
	// is a verdict this node reached without validating the transaction itself.
	//
	// registryFullEvents and staleSwept are the two that should stay at zero on
	// a healthy node. The first counts transactions processed untracked because
	// the registry was full; the second counts entries removed because they
	// outlived any plausible amount of work, which means a worker died without
	// releasing one. bundlesDrained counts completed bundles, and is the
	// denominator the other bundling numbers are read against.
	depsObserved       int64
	depsInFlight       int64
	parkedCount        int64
	revEdges           int64
	cascadeReleases    int64
	syncsIssued        int64
	syncsSkipped       int64
	failuresPropagated int64
	bundlesDrained     int64
	registryFullEvents int64
	staleSwept         int64

	// Worker management
	workerChannels  map[int]chan struct{}
	workerChanMutex sync.RWMutex

	// Retry configuration
	maxRetries int
	retryDelay time.Duration

	// How long an admitted transaction waits for room in txnQueue before it is
	// dropped and its admission released. A field rather than a literal so the
	// queue-full path is testable without a ten-second test.
	enqueueTimeout time.Duration

	// recoverySessions holds the single-use nonces issued to nodes rebuilding
	// their wallet from this fullnode. Moved off Core with the endpoint that
	// uses it; only a fullnode serves it.
	recoverySessions *recoverySessionStore
}

// NewTxnProcessor initializes the dynamic transaction processor and starts its
// workers and background goroutines. The caller must call ShutdownTxnProcessor.
func NewTxnProcessor(host Host) *DynamicTxnProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	numCPU := runtime.NumCPU()
	bundleCfg := defaultBundleConfig()

	p := &DynamicTxnProcessor{
		host:            host,
		txnQueue:        make(chan *models.EventTransaction, 10000),
		ctx:             ctx,
		cancel:          cancel,
		minWorkers:      max(1, numCPU/4),
		maxWorkers:      numCPU * 2,
		currentWorkers:  max(1, numCPU/2),
		cpuThreshold:    70.0, // MEMORY-BASED THRESHOLD
		memoryThreshold: 75.0, // MEMORY-BASED THRESHOLD
		queueThreshold:  100,
		scaleUpDelay:    time.Second * 10,
		scaleDownDelay:  time.Second * 30,
		workerChannels:  make(map[int]chan struct{}),
		maxRetries:      3,
		retryDelay:      time.Second * 2,
		enqueueTimeout:  time.Second * 10,
		inflight:        newInflightRegistry(),
		bundle:          bundleCfg,
		syncMemo:        newSyncedTokenMemo(bundleCfg.syncMemoTTL),
	}
	p.resolveDependency = p.dependencyResolved
	p.syncChains = func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
		return p.host.SyncTransactionChainsFromPeer(peerDID, tokenIDs, prevTxIDs, excludeTxIDs, false, p.host.IsFullNode())
	}

	// Start initial workers
	for i := 0; i < p.currentWorkers; i++ {
		p.startWorker(i)
	}

	p.host.Log().Info("Transaction processor initialized",
		"initialWorkers", p.currentWorkers,
		"minWorkers", p.minWorkers,
		"maxWorkers", p.maxWorkers,
		"queueCapacity", cap(p.txnQueue))

	// Start system monitor
	go p.systemMonitor()

	// Periodically evict stale entries from the dedup map to bound memory
	go p.dedupMapCleaner()

	// Periodically verify that at least minWorkers are alive
	go p.workerHealthCheck()

	return p
}

// Monitor system resources and adjust worker count
func (p *DynamicTxnProcessor) systemMonitor() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	// Variables to calculate CPU usage between intervals
	var lastCPUStats map[string]uint64

	for {
		select {
		case <-ticker.C:
			p.evaluateAndScale(lastCPUStats)
		case <-p.ctx.Done():
			return
		}
	}
}

// Evaluate system conditions and decide on scaling
func (p *DynamicTxnProcessor) evaluateAndScale(lastCPUStats map[string]uint64) {
	// Get memory stats from the node's resource monitor
	memoryUsagePercent := p.host.MemoryUsagePercent()

	// Get CPU usage using /proc/stat calculation
	cpuUsagePercent, newCPUStats := p.host.CPUUsage(lastCPUStats)
	lastCPUStats = newCPUStats // Update for next iteration

	// Get queue metrics
	queueLen := int64(len(p.txnQueue))

	p.workersMutex.RLock()
	currentWorkers := p.currentWorkers
	p.workersMutex.RUnlock()

	// Determine scaling action using your thresholds
	scalingDecision := p.determineScalingAction(
		cpuUsagePercent, memoryUsagePercent, queueLen, currentWorkers)

	// Apply scaling decision
	switch scalingDecision {
	case "scale_up":
		p.scaleUp()
	case "scale_down":
		p.scaleDown()
	}
}

// Determine scaling action based on metrics
func (p *DynamicTxnProcessor) determineScalingAction(cpuPercent, memoryPercent float64, queueLen int64, currentWorkers int) string {
	now := time.Now()

	// Scale up conditions - using your memory-based approach
	queuePressure := queueLen > int64(p.queueThreshold)
	resourcesAvailable := cpuPercent < p.cpuThreshold &&
		memoryPercent < p.memoryThreshold
	hasWorkload := queueLen > 10
	canScaleUp := currentWorkers < p.maxWorkers
	scaleUpDelayMet := now.Sub(p.lastScaleAction) > p.scaleUpDelay

	shouldScaleUp := (queuePressure || (hasWorkload && resourcesAvailable)) &&
		canScaleUp &&
		scaleUpDelayMet

	// Scale down conditions
	highResourceUsage := cpuPercent > p.cpuThreshold ||
		memoryPercent > p.memoryThreshold
	lowWorkload := queueLen == 0 && currentWorkers > p.minWorkers
	canScaleDown := currentWorkers > p.minWorkers
	scaleDownDelayMet := now.Sub(p.lastScaleAction) > p.scaleDownDelay

	shouldScaleDown := (highResourceUsage || lowWorkload) &&
		canScaleDown &&
		scaleDownDelayMet

	if shouldScaleUp {
		return "scale_up"
	} else if shouldScaleDown {
		return "scale_down"
	}

	return "no_change"
}

// Scale up worker count
func (p *DynamicTxnProcessor) scaleUp() {
	p.workersMutex.Lock()
	defer p.workersMutex.Unlock()

	if p.currentWorkers >= p.maxWorkers {
		return // ALREADY AT MAXIMUM
	}

	// Calculate how many workers to add (25% increase or minimum 1)
	newWorkers := max(1, p.currentWorkers/4)                    // 25% INCREASE
	newWorkers = min(newWorkers, p.maxWorkers-p.currentWorkers) // DON'T EXCEED MAX

	// Start new workers
	for i := 0; i < newWorkers; i++ {
		workerID := p.currentWorkers + i
		p.startWorker(workerID) // START NEW WORKER
	}

	p.currentWorkers += newWorkers // UPDATE COUNT
	p.lastScaleAction = time.Now() // UPDATE TIMESTAMP
}

// Scale down worker count
func (p *DynamicTxnProcessor) scaleDown() {
	p.workersMutex.Lock()
	defer p.workersMutex.Unlock()

	if p.currentWorkers <= p.minWorkers {
		return
	}

	removeWorkers := max(1, p.currentWorkers/4)
	removeWorkers = min(removeWorkers, p.currentWorkers-p.minWorkers)

	// Never scale below minWorkers
	if p.currentWorkers-removeWorkers < p.minWorkers {
		removeWorkers = p.currentWorkers - p.minWorkers
	}
	if removeWorkers <= 0 {
		return
	}

	p.workerChanMutex.Lock()
	workersToStop := make([]int, 0, removeWorkers)

	for workerID := p.currentWorkers - 1; len(workersToStop) < removeWorkers && workerID >= p.minWorkers; workerID-- {
		if stopChan, exists := p.workerChannels[workerID]; exists {
			close(stopChan)
			delete(p.workerChannels, workerID)
			workersToStop = append(workersToStop, workerID)
		}
	}
	p.workerChanMutex.Unlock()

	p.currentWorkers -= len(workersToStop)
	p.lastScaleAction = time.Now()
}

// Start a new worker
func (p *DynamicTxnProcessor) startWorker(workerID int) {
	stopChan := make(chan struct{}) // INDIVIDUAL STOP CHANNEL

	p.workerChanMutex.Lock()
	p.workerChannels[workerID] = stopChan // STORE STOP CHANNEL
	p.workerChanMutex.Unlock()

	p.wg.Add(1)
	go p.dynamicWorker(workerID, stopChan) // START WORKER WITH STOP CHANNEL
}

// Dynamic worker that can be individually stopped
func (p *DynamicTxnProcessor) dynamicWorker(workerID int, stopChan chan struct{}) {
	defer p.wg.Done()
	defer func() {
		// Remove ourselves from the live worker map so the health
		// check can detect that we exited.
		p.workerChanMutex.Lock()
		delete(p.workerChannels, workerID)
		p.workerChanMutex.Unlock()

		if r := recover(); r != nil {
			p.host.Log().Error("Transaction worker panicked — will be restarted by health check",
				"workerID", workerID, "panic", r)
		}
	}()

	for {
		select {
		case txnEvent, ok := <-p.txnQueue:
			if !ok || txnEvent == nil {
				return
			}
			startTime := time.Now()
			p.processTxnWithRetry(txnEvent, workerID)
			processingTime := time.Since(startTime)

			p.updateProcessingMetrics(processingTime)

		case <-stopChan:
			return

		case <-p.ctx.Done():
			return
		}
	}
}

// Update processing metrics for better scaling decisions
func (p *DynamicTxnProcessor) updateProcessingMetrics(processingTime time.Duration) {
	// Simple exponential moving average for processing time
	if p.averageProcessTime == 0 {
		p.averageProcessTime = processingTime // FIRST MEASUREMENT
	} else {
		// EMA with alpha = 0.1 (90% old value, 10% new value)
		alpha := 0.1
		p.averageProcessTime = time.Duration(
			float64(p.averageProcessTime)*(1-alpha) +
				float64(processingTime)*alpha, // EXPONENTIAL MOVING AVERAGE
		)
	}
}

// workerHealthCheck periodically verifies that at least minWorkers are
// actively registered. On resource-constrained VMs aggressive scaleDown
// calls or worker panics can leave zero consumers; this goroutine
// detects the situation and restarts workers so that the txnQueue never
// stalls.
func (p *DynamicTxnProcessor) workerHealthCheck() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.workersMutex.Lock()
			p.workerChanMutex.RLock()
			alive := len(p.workerChannels)
			p.workerChanMutex.RUnlock()

			if alive < p.minWorkers {
				deficit := p.minWorkers - alive
				p.host.Log().Warn("Worker health check: fewer workers alive than minWorkers, restarting",
					"alive", alive, "minWorkers", p.minWorkers, "restarting", deficit)
				for i := 0; i < deficit; i++ {
					id := p.currentWorkers + i
					p.startWorker(id)
				}
				p.currentWorkers += deficit
			}
			p.workersMutex.Unlock()

		case <-p.ctx.Done():
			return
		}
	}
}

// admit reserves txnID for processing and reports whether this caller won the
// reservation. A false return means the transaction was already admitted and the
// caller must drop it.
//
// The reservation has to be atomic. pubsub dispatches every message on its own
// goroutine (types/pubsub.go:164), so TxnCallBack runs concurrently with itself,
// and a Load-then-Store pair leaves a window in which two deliveries of the same
// transaction both observe "not seen" and both enqueue.
//
// The stored value is the admission time, which dedupMapCleaner reads to expire
// entries after dedupTTL.
func (p *DynamicTxnProcessor) admit(txnID string) bool {
	_, alreadyAdmitted := p.processedTxns.LoadOrStore(txnID, time.Now())
	return !alreadyAdmitted
}

// releaseAdmission drops a reservation taken by admit, making txnID eligible for
// admission again on a later pubsub delivery.
//
// It must be called on every path that admits a transaction without handing it to
// a worker, and once a transaction has exhausted its retries. Skipping it leaves
// the transaction suppressed until dedupMapCleaner sweeps the entry dedupTTL
// later.
func (p *DynamicTxnProcessor) releaseAdmission(txnID string) {
	p.processedTxns.Delete(txnID)
}

const dedupTTL = 10 * time.Minute

// dedupMapCleaner periodically removes entries older than dedupTTL from the
// processedTxns sync.Map so memory doesn't grow unboundedly on long-running nodes.
func (p *DynamicTxnProcessor) dedupMapCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			p.processedTxns.Range(func(key, value interface{}) bool {
				if ts, ok := value.(time.Time); ok && now.Sub(ts) > dedupTTL {
					p.processedTxns.Delete(key)
				}
				return true
			})

			// Piggy-backed on the existing sweep rather than adding another
			// ticker. inflight should hover near the number of busy workers; a
			// value that climbs steadily means entries are leaking.
			//
			// waitingOn and components are the same kind of signal: both should
			// rise and fall with the workload, and either one only ever rising
			// means something is failing to unpark or to prune.
			p.syncMemo.sweep()

			// The registry has no natural expiry — an entry leaves when its
			// worker is done with it — so this is the only thing that would ever
			// notice one that never left. It should always find nothing.
			if stale := p.inflight.sweepStale(inflightTTL); len(stale) > 0 {
				atomic.AddInt64(&p.staleSwept, int64(len(stale)))
				p.host.Log().Error("Swept in-flight entries that outlived any plausible processing time",
					"count", len(stale), "txnIDs", stale, "ttl", inflightTTL)
			}

			p.host.Log().Info("Fullnode ingest metrics",
				"inflight", p.inflight.len(),
				"queueLength", len(p.txnQueue),
				"depsObserved", atomic.LoadInt64(&p.depsObserved),
				"depsInFlight", atomic.LoadInt64(&p.depsInFlight),
				"parked", atomic.LoadInt64(&p.parkedCount),
				"waitingOn", p.inflight.waitingLen(),
				"components", p.inflight.componentLen(),
				"revEdges", atomic.LoadInt64(&p.revEdges),
				"cascadeReleases", atomic.LoadInt64(&p.cascadeReleases),
				"syncsIssued", atomic.LoadInt64(&p.syncsIssued),
				"syncsSkipped", atomic.LoadInt64(&p.syncsSkipped),
				"syncMemo", p.syncMemo.len(),
				"failuresPropagated", atomic.LoadInt64(&p.failuresPropagated),
				"bundlesDrained", atomic.LoadInt64(&p.bundlesDrained),
				"registryFull", atomic.LoadInt64(&p.registryFullEvents),
				"staleSwept", atomic.LoadInt64(&p.staleSwept))
		case <-p.ctx.Done():
			return
		}
	}
}
