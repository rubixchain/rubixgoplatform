package core

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

	// Metrics
	queueLength        int64
	averageProcessTime time.Duration
	processedTxnCount  int64 // ADD ATOMIC COUNTER

	// Worker management
	workerChannels  map[int]chan struct{}
	workerChanMutex sync.RWMutex

	// Retry configuration
	maxRetries int
	retryDelay time.Duration

	// Resource monitor
	resourceMonitor *ResourceMonitor
}

// Initialize dynamic transaction processor
func (c *Core) initDynamicTxnProcessor() {
	ctx, cancel := context.WithCancel(context.Background())

	numCPU := runtime.NumCPU()

	c.txnProcessor = &DynamicTxnProcessor{
		txnQueue:        make(chan *models.EventTransaction, 1000),
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
		resourceMonitor: &ResourceMonitor{}, // INITIALIZE YOUR MONITOR
	}

	// Start initial workers
	for i := 0; i < c.txnProcessor.currentWorkers; i++ {
		c.startWorker(i)
	}

	// Start system monitor
	go c.systemMonitor()

	// Periodically evict stale entries from the dedup map to bound memory
	go c.dedupMapCleaner()
}

// Monitor system resources and adjust worker count
func (c *Core) systemMonitor() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	// Variables to calculate CPU usage between intervals
	var lastCPUStats map[string]uint64

	for {
		select {
		case <-ticker.C:
			c.evaluateAndScale(lastCPUStats)
		case <-c.txnProcessor.ctx.Done():
			return
		}
	}
}

// Evaluate system conditions and decide on scaling
func (c *Core) evaluateAndScale(lastCPUStats map[string]uint64) {
	// Get memory stats using your ResourceMonitor
	resourceStats := c.txnProcessor.resourceMonitor.GetResourceStats()
	memoryUsagePercent := resourceStats["memory_usage_pct"].(float64)

	// Get CPU usage using /proc/stat calculation
	cpuUsagePercent, newCPUStats := c.getCPUUsageLinux(lastCPUStats)
	lastCPUStats = newCPUStats // Update for next iteration

	// Get queue metrics
	queueLen := int64(len(c.txnProcessor.txnQueue))

	c.txnProcessor.workersMutex.RLock()
	currentWorkers := c.txnProcessor.currentWorkers
	c.txnProcessor.workersMutex.RUnlock()

	// Determine scaling action using your thresholds
	scalingDecision := c.determineScalingAction(
		cpuUsagePercent, memoryUsagePercent, queueLen, currentWorkers)

	// Apply scaling decision
	switch scalingDecision {
	case "scale_up":
		c.scaleUp()
	case "scale_down":
		c.scaleDown()
	}
}

// Determine scaling action based on metrics
func (c *Core) determineScalingAction(cpuPercent, memoryPercent float64, queueLen int64, currentWorkers int) string {
	now := time.Now()

	// Scale up conditions - using your memory-based approach
	queuePressure := queueLen > int64(c.txnProcessor.queueThreshold)
	resourcesAvailable := cpuPercent < c.txnProcessor.cpuThreshold &&
		memoryPercent < c.txnProcessor.memoryThreshold
	hasWorkload := queueLen > 10
	canScaleUp := currentWorkers < c.txnProcessor.maxWorkers
	scaleUpDelayMet := now.Sub(c.txnProcessor.lastScaleAction) > c.txnProcessor.scaleUpDelay

	shouldScaleUp := (queuePressure || (hasWorkload && resourcesAvailable)) &&
		canScaleUp &&
		scaleUpDelayMet

	// Scale down conditions
	highResourceUsage := cpuPercent > c.txnProcessor.cpuThreshold ||
		memoryPercent > c.txnProcessor.memoryThreshold
	lowWorkload := queueLen == 0 && currentWorkers > c.txnProcessor.minWorkers
	canScaleDown := currentWorkers > c.txnProcessor.minWorkers
	scaleDownDelayMet := now.Sub(c.txnProcessor.lastScaleAction) > c.txnProcessor.scaleDownDelay

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
func (c *Core) scaleUp() {
	c.txnProcessor.workersMutex.Lock()
	defer c.txnProcessor.workersMutex.Unlock()

	if c.txnProcessor.currentWorkers >= c.txnProcessor.maxWorkers {
		return // ALREADY AT MAXIMUM
	}

	// Calculate how many workers to add (25% increase or minimum 1)
	newWorkers := max(1, c.txnProcessor.currentWorkers/4)                                 // 25% INCREASE
	newWorkers = min(newWorkers, c.txnProcessor.maxWorkers-c.txnProcessor.currentWorkers) // DON'T EXCEED MAX

	// Start new workers
	for i := 0; i < newWorkers; i++ {
		workerID := c.txnProcessor.currentWorkers + i
		c.startWorker(workerID) // START NEW WORKER
	}

	c.txnProcessor.currentWorkers += newWorkers // UPDATE COUNT
	c.txnProcessor.lastScaleAction = time.Now() // UPDATE TIMESTAMP
}

// Scale down worker count
func (c *Core) scaleDown() {
	c.txnProcessor.workersMutex.Lock()
	defer c.txnProcessor.workersMutex.Unlock()

	if c.txnProcessor.currentWorkers <= c.txnProcessor.minWorkers {
		return // ALREADY AT MINIMUM
	}

	// Calculate how many workers to remove (25% decrease or minimum 1)
	removeWorkers := max(1, c.txnProcessor.currentWorkers/4)                                    // 25% DECREASE
	removeWorkers = min(removeWorkers, c.txnProcessor.currentWorkers-c.txnProcessor.minWorkers) // DON'T GO BELOW MIN

	// Stop workers gracefully
	c.txnProcessor.workerChanMutex.Lock()
	workersToStop := make([]int, 0, removeWorkers)

	// Select workers to stop (LIFO - last in, first out)
	for workerID := c.txnProcessor.currentWorkers - 1; len(workersToStop) < removeWorkers && workerID >= c.txnProcessor.minWorkers; workerID-- {
		if stopChan, exists := c.txnProcessor.workerChannels[workerID]; exists {
			close(stopChan) // SIGNAL WORKER TO STOP
			delete(c.txnProcessor.workerChannels, workerID)
			workersToStop = append(workersToStop, workerID)
		}
	}
	c.txnProcessor.workerChanMutex.Unlock()

	c.txnProcessor.currentWorkers -= len(workersToStop) // UPDATE COUNT
	c.txnProcessor.lastScaleAction = time.Now()         // UPDATE TIMESTAMP

}

// Start a new worker
func (c *Core) startWorker(workerID int) {
	stopChan := make(chan struct{}) // INDIVIDUAL STOP CHANNEL

	c.txnProcessor.workerChanMutex.Lock()
	c.txnProcessor.workerChannels[workerID] = stopChan // STORE STOP CHANNEL
	c.txnProcessor.workerChanMutex.Unlock()

	c.txnProcessor.wg.Add(1)
	go c.dynamicWorker(workerID, stopChan) // START WORKER WITH STOP CHANNEL
}

// Dynamic worker that can be individually stopped
func (c *Core) dynamicWorker(workerID int, stopChan chan struct{}) {
	defer c.txnProcessor.wg.Done()

	for {
		select {
		case txnEvent, ok := <-c.txnProcessor.txnQueue:
			if !ok || txnEvent == nil {
				return
			}
			startTime := time.Now()
			c.processTxnWithRetry(txnEvent, workerID)
			processingTime := time.Since(startTime)

			c.updateProcessingMetrics(processingTime)

		case <-stopChan:
			return

		case <-c.txnProcessor.ctx.Done():
			return
		}
	}
}

// Update processing metrics for better scaling decisions
func (c *Core) updateProcessingMetrics(processingTime time.Duration) {
	// Simple exponential moving average for processing time
	if c.txnProcessor.averageProcessTime == 0 {
		c.txnProcessor.averageProcessTime = processingTime // FIRST MEASUREMENT
	} else {
		// EMA with alpha = 0.1 (90% old value, 10% new value)
		alpha := 0.1
		c.txnProcessor.averageProcessTime = time.Duration(
			float64(c.txnProcessor.averageProcessTime)*(1-alpha) +
				float64(processingTime)*alpha, // EXPONENTIAL MOVING AVERAGE
		)
	}
}

// Fast stats function using atomic counter
func (c *Core) GetDynamicWorkerPoolStats() map[string]interface{} {
	c.txnProcessor.workersMutex.RLock()
	currentWorkers := c.txnProcessor.currentWorkers
	c.txnProcessor.workersMutex.RUnlock()

	// Get detailed resource stats from your ResourceMonitor
	resourceStats := c.txnProcessor.resourceMonitor.GetResourceStats()

	// Add CPU usage calculation
	cpuUsage, _ := c.getCPUUsageLinux(nil)

	// Use atomic counter instead of counting sync.Map
	processedCount := atomic.LoadInt64(&c.txnProcessor.processedTxnCount)

	// Combine all stats
	stats := map[string]interface{}{
		// Worker pool stats
		"currentWorkers": currentWorkers,
		"minWorkers":     c.txnProcessor.minWorkers,
		"maxWorkers":     c.txnProcessor.maxWorkers,
		"queueLength":    len(c.txnProcessor.txnQueue),
		"processedTxns":  processedCount, // FAST ATOMIC READ

		// Resource stats from your ResourceMonitor
		"memoryTotalMB":      resourceStats["memory_total_mb"],
		"memoryAvailableMB":  resourceStats["memory_available_mb"],
		"memoryUsedMB":       resourceStats["memory_used_mb"],
		"memoryUsagePercent": resourceStats["memory_usage_pct"],
		"cpuCount":           resourceStats["cpu_count"],
		"goroutines":         resourceStats["goroutines"],

		// Calculated CPU usage
		"cpuUsagePercent": cpuUsage,

		// Thresholds
		"cpuThreshold":    c.txnProcessor.cpuThreshold,
		"memoryThreshold": c.txnProcessor.memoryThreshold,
		"queueThreshold":  c.txnProcessor.queueThreshold,
	}

	return stats
}

const dedupTTL = 10 * time.Minute

// dedupMapCleaner periodically removes entries older than dedupTTL from the
// processedTxns sync.Map so memory doesn't grow unboundedly on long-running nodes.
func (c *Core) dedupMapCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			c.txnProcessor.processedTxns.Range(func(key, value interface{}) bool {
				if ts, ok := value.(time.Time); ok && now.Sub(ts) > dedupTTL {
					c.txnProcessor.processedTxns.Delete(key)
				}
				return true
			})
		case <-c.txnProcessor.ctx.Done():
			return
		}
	}
}
