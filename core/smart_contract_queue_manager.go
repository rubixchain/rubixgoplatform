package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// SmartContractQueueManager manages per-contract execution queues
// to prevent race conditions when multiple executions for the same contract
// occur in parallel on the same node.
type SmartContractQueueManager struct {
	queues map[string]*ContractQueue // map[smartContractToken]*ContractQueue
	mu     sync.RWMutex
	log    logger.Logger
	core   *Core // reference to Core for execution
}

// ContractQueue represents the hybrid queue for a single smart contract.
// It uses a buffered channel (fast queue) for typical load and a slice
// (slow queue) for overflow, ensuring no API requests are rejected.
type ContractQueue struct {
	contractToken string
	fastQueue     chan *ExecutionJob
	slowQueue     []*ExecutionJob
	slowQueueMu   sync.Mutex
	workerRunning bool
	log           logger.Logger
	core          *Core
	stopChan      chan struct{}

	// Metrics for monitoring
	enqueueCount  int64
	executeCount  int64
	fastHitCount  int64
	slowHitCount  int64
	metricsLock   sync.Mutex
}

// ExecutionJob represents a smart contract execution request
type ExecutionJob struct {
	ReqID       string
	ExecuteReq  *model.ExecuteSmartContractRequest
	EnqueueTime time.Time
}

const (
	// FastQueueSize is the buffered channel size for the fast queue
	FastQueueSize = 5
)

// NewSmartContractQueueManager creates a new queue manager instance
func NewSmartContractQueueManager(c *Core, log logger.Logger) *SmartContractQueueManager {
	return &SmartContractQueueManager{
		queues: make(map[string]*ContractQueue),
		log:    log.Named("SCQueueMgr"),
		core:   c,
	}
}

// EnqueueExecution enqueues a smart contract execution request.
// This method is called by the API layer instead of spawning a goroutine directly.
// It creates a queue for the contract if it doesn't exist and starts a worker.
func (mgr *SmartContractQueueManager) EnqueueExecution(
	reqID string,
	executeReq *model.ExecuteSmartContractRequest,
) error {
	mgr.mu.Lock()

	// Get or create queue for this contract
	queue, exists := mgr.queues[executeReq.SmartContractToken]
	if !exists {
		queue = mgr.createContractQueue(executeReq.SmartContractToken)
		mgr.queues[executeReq.SmartContractToken] = queue

		// Start worker goroutine for this contract
		go queue.worker()

		mgr.log.Info("Created new queue for contract",
			"contract", executeReq.SmartContractToken[:8]+"...")
	}
	mgr.mu.Unlock()

	// Enqueue the job
	job := &ExecutionJob{
		ReqID:       reqID,
		ExecuteReq:  executeReq,
		EnqueueTime: time.Now(),
	}

	return queue.enqueue(job)
}

// createContractQueue creates a new queue instance for a contract
func (mgr *SmartContractQueueManager) createContractQueue(
	contractToken string,
) *ContractQueue {
	return &ContractQueue{
		contractToken: contractToken,
		fastQueue:     make(chan *ExecutionJob, FastQueueSize),
		slowQueue:     make([]*ExecutionJob, 0),
		log:           mgr.log.Named(contractToken[:8] + "..."),
		core:          mgr.core,
		stopChan:      make(chan struct{}),
		workerRunning: true,
	}
}

// enqueue adds a job to the queue (fast or slow).
// It tries the fast queue first (non-blocking). If full, it uses the slow queue.
// This ensures no requests are rejected at the API level.
func (q *ContractQueue) enqueue(job *ExecutionJob) error {
	q.metricsLock.Lock()
	q.enqueueCount++
	q.metricsLock.Unlock()

	select {
	case q.fastQueue <- job:
		// Fast queue has space - immediate enqueue
		q.metricsLock.Lock()
		q.fastHitCount++
		fastLen := len(q.fastQueue)
		q.metricsLock.Unlock()

		q.log.Info("Job enqueued to FAST queue",
			"reqID", job.ReqID[:8]+"...",
			"fastQueueLen", fastLen,
			"contract", q.contractToken[:8]+"...")
		return nil

	default:
		// Fast queue full, use slow queue
		q.slowQueueMu.Lock()
		q.slowQueue = append(q.slowQueue, job)
		slowLen := len(q.slowQueue)
		q.slowQueueMu.Unlock()

		q.metricsLock.Lock()
		q.slowHitCount++
		q.metricsLock.Unlock()

		q.log.Info("Job enqueued to SLOW queue",
			"reqID", job.ReqID[:8]+"...",
			"slowQueueLen", slowLen,
			"contract", q.contractToken[:8]+"...")
		return nil
	}
}

// dequeue gets the next job (from fast or slow queue).
// It prioritizes the fast queue, then checks the slow queue.
func (q *ContractQueue) dequeue() (*ExecutionJob, bool) {
	select {
	case job := <-q.fastQueue:
		// Got job from fast queue
		q.log.Debug("Dequeued from FAST queue",
			"reqID", job.ReqID[:8]+"...")
		return job, true

	default:
		// Try slow queue
		q.slowQueueMu.Lock()
		defer q.slowQueueMu.Unlock()

		if len(q.slowQueue) > 0 {
			job := q.slowQueue[0]
			q.slowQueue = q.slowQueue[1:]

			q.log.Debug("Dequeued from SLOW queue",
				"reqID", job.ReqID[:8]+"...",
				"remainingSlowQueue", len(q.slowQueue))
			return job, true
		}

		return nil, false
	}
}

// worker is the single worker goroutine for this contract.
// It continuously dequeues jobs and executes them sequentially (FIFO).
// This ensures that all executions for the same contract are serialized,
// preventing race conditions when reading/writing the latest block.
func (q *ContractQueue) worker() {
	q.log.Info("Worker started for contract", "contract", q.contractToken[:8]+"...")

	for {
		select {
		case <-q.stopChan:
			q.log.Info("Worker stopped", "contract", q.contractToken[:8]+"...")
			return

		default:
			// Try to dequeue a job
			job, ok := q.dequeue()
			if !ok {
				// No jobs available, wait a bit before trying again
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Execute the job
			q.executeJob(job)
		}
	}
}

// executeJob executes a single smart contract execution job.
// It calls the original execution logic and logs detailed metrics.
func (q *ContractQueue) executeJob(job *ExecutionJob) {
	startTime := time.Now()
	queueWaitTime := startTime.Sub(job.EnqueueTime)

	q.metricsLock.Lock()
	q.executeCount++
	executeCount := q.executeCount
	fastQueueLen := len(q.fastQueue)
	q.metricsLock.Unlock()

	q.slowQueueMu.Lock()
	slowQueueLen := len(q.slowQueue)
	q.slowQueueMu.Unlock()

	q.log.Info("EXECUTION START",
		"reqID", job.ReqID[:8]+"...",
		"contract", q.contractToken[:8]+"...",
		"executeCount", executeCount,
		"queueWaitTime", queueWaitTime,
		"fastQueueLen", fastQueueLen,
		"slowQueueLen", slowQueueLen)

	// Call the original execution logic
	// This is the same function that was previously called in a goroutine
	q.core.ExecuteSmartContractToken(job.ReqID, job.ExecuteReq)

	executionTime := time.Since(startTime)
	totalTime := time.Since(job.EnqueueTime)

	q.log.Info("EXECUTION COMPLETE",
		"reqID", job.ReqID[:8]+"...",
		"contract", q.contractToken[:8]+"...",
		"executionTime", executionTime,
		"totalTime", totalTime)
}

// GetMetrics returns queue metrics for monitoring and observability
func (q *ContractQueue) GetMetrics() map[string]interface{} {
	q.metricsLock.Lock()
	q.slowQueueMu.Lock()
	defer q.metricsLock.Unlock()
	defer q.slowQueueMu.Unlock()

	return map[string]interface{}{
		"contract":       q.contractToken,
		"fastQueueLen":   len(q.fastQueue),
		"slowQueueLen":   len(q.slowQueue),
		"enqueueCount":   q.enqueueCount,
		"executeCount":   q.executeCount,
		"fastHitCount":   q.fastHitCount,
		"slowHitCount":   q.slowHitCount,
		"workerRunning":  q.workerRunning,
		"pendingCount":   (len(q.fastQueue) + len(q.slowQueue)),
		"fastHitRate":    float64(q.fastHitCount) / float64(q.enqueueCount),
	}
}

// Shutdown gracefully shuts down the queue worker
func (q *ContractQueue) Shutdown() {
	q.workerRunning = false
	close(q.stopChan)
}

// GetAllMetrics returns metrics for all contract queues
func (mgr *SmartContractQueueManager) GetAllMetrics() []map[string]interface{} {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	metrics := make([]map[string]interface{}, 0, len(mgr.queues))
	for _, queue := range mgr.queues {
		metrics = append(metrics, queue.GetMetrics())
	}
	return metrics
}

// Shutdown gracefully shuts down all contract queues
func (mgr *SmartContractQueueManager) Shutdown() {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	mgr.log.Info("Shutting down all contract queues", "count", len(mgr.queues))
	for token, queue := range mgr.queues {
		mgr.log.Info("Shutting down queue", "contract", token[:8]+"...")
		queue.Shutdown()
	}
}

// EnqueueSmartContractExecution is the public method called by the API layer.
// This is the main entry point that replaces the direct goroutine spawn.
func (c *Core) EnqueueSmartContractExecution(
	reqID string,
	executeReq *model.ExecuteSmartContractRequest,
) error {
	if c.scQueueMgr == nil {
		return fmt.Errorf("smart contract queue manager not initialized")
	}

	return c.scQueueMgr.EnqueueExecution(reqID, executeReq)
}

// GetSmartContractQueueMetrics returns metrics for monitoring and observability
func (c *Core) GetSmartContractQueueMetrics() []map[string]interface{} {
	if c.scQueueMgr == nil {
		return nil
	}
	return c.scQueueMgr.GetAllMetrics()
}
