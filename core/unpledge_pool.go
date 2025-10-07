package core

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// DefaultUnpledgePoolConfig returns the default configuration
func DefaultUnpledgePoolConfig() config.UnpledgePoolConfig {
	return config.UnpledgePoolConfig{
		MaxWorkers:       runtime.NumCPU(),
		QueueSize:        1000,
		BatchSize:        50,
		TokenConcurrency: 5,
		ShutdownTimeout:  30 * time.Second,
		EnableMetrics:    true,
	}
}

// UnpledgeMetrics tracks performance metrics
type UnpledgeMetrics struct {
	TotalTransactions      int64
	SuccessfulTransactions int64
	FailedTransactions     int64
	TotalTokens            int64
	ProcessingTime         int64 // in milliseconds
	AverageLatency         int64 // in milliseconds
	mu                     sync.RWMutex
	startTime              time.Time
}

// UnpledgeWorkerPool manages concurrent unpledge operations
type UnpledgeWorkerPool struct {
	config    config.UnpledgePoolConfig
	workers   int
	taskQueue chan *UnpledgeTask
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	metrics   *UnpledgeMetrics

	// Resource management
	semaphore chan struct{} // Limits concurrent DB operations
	closed    int32         // Atomic flag for pool state
}

// UnpledgeTask represents a single unpledge transaction
type UnpledgeTask struct {
	Info   *wallet.UnpledgeSequenceInfo
	Core   *Core
	Result chan<- *UnpledgeResult
}

// UnpledgeResult contains the result of an unpledge operation
type UnpledgeResult struct {
	TransactionID  string
	Success        bool
	Error          error
	UnpledgeAmount float64
	PledgeInfo     []*wallet.PledgeInformation
	ProcessingTime time.Duration
	TokenCount     int
}

// NewUnpledgeWorkerPool creates a new worker pool with the given configuration
func NewUnpledgeWorkerPool(cfg config.UnpledgePoolConfig) (*UnpledgeWorkerPool, error) {
	// Validate configuration
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = runtime.NumCPU()
	}
	if cfg.MaxWorkers > 32 {
		cfg.MaxWorkers = 32 // Cap at reasonable limit
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.TokenConcurrency <= 0 {
		cfg.TokenConcurrency = 5
	}
	if cfg.TokenConcurrency > 20 {
		cfg.TokenConcurrency = 20 // Prevent DB connection exhaustion
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &UnpledgeWorkerPool{
		config:    cfg,
		workers:   cfg.MaxWorkers,
		taskQueue: make(chan *UnpledgeTask, cfg.QueueSize),
		ctx:       ctx,
		cancel:    cancel,
		semaphore: make(chan struct{}, cfg.TokenConcurrency),
		metrics: &UnpledgeMetrics{
			startTime: time.Now(),
		},
	}

	// Initialize semaphore
	for i := 0; i < cfg.TokenConcurrency; i++ {
		pool.semaphore <- struct{}{}
	}

	return pool, nil
}

// Start initializes and starts worker goroutines
func (p *UnpledgeWorkerPool) Start() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 0) {
		return errors.New("worker pool is closed")
	}

	// Start workers
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	// Start metrics collector if enabled
	if p.config.EnableMetrics {
		p.wg.Add(1)
		go p.metricsCollector()
	}

	return nil
}

// Stop gracefully shuts down the worker pool
func (p *UnpledgeWorkerPool) Stop() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return errors.New("worker pool already closed")
	}

	// Cancel context
	p.cancel()

	// Close task queue
	close(p.taskQueue)

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(p.config.ShutdownTimeout):
		return errors.New("shutdown timeout exceeded")
	}
}

// Submit adds a task to the processing queue
func (p *UnpledgeWorkerPool) Submit(task *UnpledgeTask) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New("worker pool is closed")
	}

	select {
	case p.taskQueue <- task:
		return nil
	case <-p.ctx.Done():
		return errors.New("worker pool is shutting down")
	default:
		return errors.New("task queue is full")
	}
}

// worker processes tasks from the queue
func (p *UnpledgeWorkerPool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}
			p.processTask(task)

		case <-p.ctx.Done():
			return
		}
	}
}

// processTask handles a single unpledge task
func (p *UnpledgeWorkerPool) processTask(task *UnpledgeTask) {
	start := time.Now()
	result := &UnpledgeResult{
		TransactionID: task.Info.TransactionID,
		Success:       false,
	}

	defer func() {
		// Send result
		select {
		case task.Result <- result:
		case <-p.ctx.Done():
		}

		// Update metrics
		p.updateMetrics(result, time.Since(start))
	}()

	// Process unpledging with proper error handling
	pledgeInfo, tokenCount, err := p.unpledgeTransaction(task)
	if err != nil {
		result.Error = err
		return
	}

	result.PledgeInfo = pledgeInfo
	result.TokenCount = tokenCount

	// Store credits
	err = task.Core.w.StoreCredit(task.Info.TransactionID, task.Info.QuorumDID, pledgeInfo)
	if err != nil {
		result.Error = fmt.Errorf("credit storage failed: %w", err)
		return
	}

	// Remove unpledge sequence info
	err = task.Core.w.RemoveUnpledgeSequenceInfo(task.Info.TransactionID)
	if err != nil {
		// Rollback credit storage
		if rollbackErr := task.Core.w.RemoveCredit(task.Info.TransactionID); rollbackErr != nil {
			task.Core.log.Error("Failed to rollback credit storage", "err", rollbackErr)
		}
		result.Error = fmt.Errorf("failed to remove unpledge sequence: %w", err)
		return
	}

	// Calculate unpledge amount
	amount, err := task.Core.getTotalAmountFromTokenHashes(tokenStringToSlice(task.Info.PledgeTokens))
	if err != nil {
		task.Core.log.Error("Failed to calculate unpledge amount", "err", err)
		amount = 0
	}

	result.Success = true
	result.UnpledgeAmount = amount
	result.ProcessingTime = time.Since(start)

	// Update pledge status asynchronously
	go func() {
		task.Core.UpdatePledgeStatus(tokenStringToSlice(task.Info.PledgeTokens), task.Info.QuorumDID)
	}()
}

// unpledgeTransaction handles the actual unpledging logic
func (p *UnpledgeWorkerPool) unpledgeTransaction(task *UnpledgeTask) ([]*wallet.PledgeInformation, int, error) {
	pledgeTokensList := tokenStringToSlice(task.Info.PledgeTokens)
	if len(pledgeTokensList) == 0 {
		return nil, 0, fmt.Errorf("no pledged tokens found")
	}

	// For small token counts, use sequential processing
	if len(pledgeTokensList) <= 10 {
		pledgeInfo, err := unpledgeAllTokens(task.Core, task.Info.TransactionID,
			task.Info.PledgeTokens, task.Info.QuorumDID)
		return pledgeInfo, len(pledgeTokensList), err
	}

	// Parallel processing for larger token counts
	return p.unpledgeTokensParallel(task.Core, task.Info.TransactionID,
		pledgeTokensList, task.Info.QuorumDID)
}

// unpledgeTokensParallel processes tokens concurrently with controlled parallelism
func (p *UnpledgeWorkerPool) unpledgeTokensParallel(c *Core, transactionID string,
	tokens []string, quorumDID string) ([]*wallet.PledgeInformation, int, error) {

	pledgeInfoList := make([]*wallet.PledgeInformation, len(tokens))
	errChan := make(chan error, len(tokens))
	var wg sync.WaitGroup

	for i, token := range tokens {
		wg.Add(1)
		go func(index int, pledgeToken string) {
			defer wg.Done()

			// Acquire semaphore to limit concurrent DB operations
			<-p.semaphore
			defer func() { p.semaphore <- struct{}{} }()

			// Check context cancellation
			select {
			case <-p.ctx.Done():
				errChan <- errors.New("context cancelled")
				return
			default:
			}

			// Get token type
			tokenType, err := getTokenType(c.w, pledgeToken, c.testNet)
			if err != nil {
				errChan <- fmt.Errorf("token %s: %w", pledgeToken, err)
				return
			}

			// Unpledge token
			pledgeBlockID, unpledgeBlockID, err := unpledgeToken(c, pledgeToken,
				tokenType, quorumDID, transactionID)
			if err != nil {
				errChan <- fmt.Errorf("unpledge token %s: %w", pledgeToken, err)
				return
			}

			// Store result
			pledgeInfoList[index] = &wallet.PledgeInformation{
				TokenID:         pledgeToken,
				TokenType:       tokenType,
				PledgeBlockID:   pledgeBlockID,
				UnpledgeBlockID: unpledgeBlockID,
				QuorumDID:       quorumDID,
				TransactionID:   transactionID,
			}
		}(i, token)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return nil, 0, err
		}
	}

	// Remove token state hashes
	err := c.w.RemoveTokenStateHashByTransactionID(transactionID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to remove token state hashes: %w", err)
	}

	return pledgeInfoList, len(tokens), nil
}

// updateMetrics updates performance metrics
func (p *UnpledgeWorkerPool) updateMetrics(result *UnpledgeResult, duration time.Duration) {
	if !p.config.EnableMetrics {
		return
	}

	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	atomic.AddInt64(&p.metrics.TotalTransactions, 1)
	if result.Success {
		atomic.AddInt64(&p.metrics.SuccessfulTransactions, 1)
		atomic.AddInt64(&p.metrics.TotalTokens, int64(result.TokenCount))
	} else {
		atomic.AddInt64(&p.metrics.FailedTransactions, 1)
	}

	// Update processing time
	atomic.AddInt64(&p.metrics.ProcessingTime, duration.Milliseconds())

	// Calculate average latency
	total := atomic.LoadInt64(&p.metrics.TotalTransactions)
	if total > 0 {
		avgLatency := atomic.LoadInt64(&p.metrics.ProcessingTime) / total
		atomic.StoreInt64(&p.metrics.AverageLatency, avgLatency)
	}
}

// metricsCollector periodically logs metrics
func (p *UnpledgeWorkerPool) metricsCollector() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.logMetrics()
		case <-p.ctx.Done():
			return
		}
	}
}

// logMetrics logs current metrics
func (p *UnpledgeWorkerPool) logMetrics() {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	total := atomic.LoadInt64(&p.metrics.TotalTransactions)
	if total == 0 {
		return
	}

	successful := atomic.LoadInt64(&p.metrics.SuccessfulTransactions)
	failed := atomic.LoadInt64(&p.metrics.FailedTransactions)
	tokens := atomic.LoadInt64(&p.metrics.TotalTokens)
	avgLatency := atomic.LoadInt64(&p.metrics.AverageLatency)
	uptime := time.Since(p.metrics.startTime)

	// Log metrics (using structured logging would be better)
	fmt.Printf("Unpledge Pool Metrics - Uptime: %v, Total: %d, Success: %d, Failed: %d, Tokens: %d, Avg Latency: %dms\n",
		uptime, total, successful, failed, tokens, avgLatency)
}

// GetMetrics returns current metrics snapshot
func (p *UnpledgeWorkerPool) GetMetrics() UnpledgeMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	return UnpledgeMetrics{
		TotalTransactions:      atomic.LoadInt64(&p.metrics.TotalTransactions),
		SuccessfulTransactions: atomic.LoadInt64(&p.metrics.SuccessfulTransactions),
		FailedTransactions:     atomic.LoadInt64(&p.metrics.FailedTransactions),
		TotalTokens:            atomic.LoadInt64(&p.metrics.TotalTokens),
		ProcessingTime:         atomic.LoadInt64(&p.metrics.ProcessingTime),
		AverageLatency:         atomic.LoadInt64(&p.metrics.AverageLatency),
		startTime:              p.metrics.startTime,
	}
}

// tokenStringToSlice converts comma-separated token string to slice
func tokenStringToSlice(tokens string) []string {
	if tokens == "" {
		return []string{}
	}
	return strings.Split(tokens, ",")
}

