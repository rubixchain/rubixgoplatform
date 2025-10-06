package wallet

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// AsyncProviderDetailsManager handles background provider details updates
type AsyncProviderDetailsManager struct {
	wallet    *Wallet
	log       logger.Logger
	jobQueue  chan *ProviderDetailsJob
	workers   int
	wg        sync.WaitGroup
	mu        sync.RWMutex
	stats     ProviderDetailsStats
}

// ProviderDetailsJob represents a batch of provider details to process
type ProviderDetailsJob struct {
	ProviderMaps  []model.TokenProviderMap
	TransactionID string
	TokenCount    int
	SubmittedAt   time.Time
}

// ProviderDetailsStats tracks performance statistics
type ProviderDetailsStats struct {
	TotalJobs      int64
	CompletedJobs  int64
	FailedJobs     int64
	TotalTokens    int64
	ProcessedTokens int64
}

// NewAsyncProviderDetailsManager creates a new async provider details manager
func NewAsyncProviderDetailsManager(wallet *Wallet, workers int) *AsyncProviderDetailsManager {
	if workers <= 0 {
		workers = 2 // Default workers
	}

	apdm := &AsyncProviderDetailsManager{
		wallet:   wallet,
		log:      wallet.log.Named("AsyncProviderDetails"),
		jobQueue: make(chan *ProviderDetailsJob, 100),
		workers:  workers,
	}

	// Start worker pool
	for i := 0; i < workers; i++ {
		apdm.wg.Add(1)
		go apdm.worker(i)
	}

	// Start stats monitor
	apdm.wg.Add(1)
	go apdm.statsMonitor()

	return apdm
}

// SubmitProviderDetails submits provider details for background processing
func (apdm *AsyncProviderDetailsManager) SubmitProviderDetails(providerMaps []model.TokenProviderMap, transactionID string) error {
	job := &ProviderDetailsJob{
		ProviderMaps:  providerMaps,
		TransactionID: transactionID,
		TokenCount:    len(providerMaps),
		SubmittedAt:   time.Now(),
	}

	select {
	case apdm.jobQueue <- job:
		apdm.mu.Lock()
		apdm.stats.TotalJobs++
		apdm.stats.TotalTokens += int64(len(providerMaps))
		apdm.mu.Unlock()
		
		apdm.log.Info("Provider details job queued",
			"transaction_id", transactionID,
			"token_count", len(providerMaps))
		return nil
	case <-time.After(5 * time.Second):
		apdm.log.Error("Failed to queue provider details job - queue full")
		return fmt.Errorf("job queue full")
	}
}

// worker processes provider details jobs
func (apdm *AsyncProviderDetailsManager) worker(id int) {
	defer apdm.wg.Done()

	for job := range apdm.jobQueue {
		apdm.processJob(job)
	}
}

// processJob processes a single provider details job
func (apdm *AsyncProviderDetailsManager) processJob(job *ProviderDetailsJob) {
	startTime := time.Now()
	
	apdm.log.Debug("Processing provider details job",
		"transaction_id", job.TransactionID,
		"token_count", job.TokenCount)

	// Process in smaller batches to avoid database locks
	batchSize := 50
	if job.TokenCount > 500 {
		batchSize = 25
	}

	successCount := 0
	failCount := 0

	for i := 0; i < len(job.ProviderMaps); i += batchSize {
		end := i + batchSize
		if end > len(job.ProviderMaps) {
			end = len(job.ProviderMaps)
		}

		batch := job.ProviderMaps[i:end]
		
		// Try to add batch with retries
		maxRetries := 3
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			err = apdm.wallet.AddProviderDetailsBatch(batch)
			if err == nil {
				successCount += len(batch)
				break
			}
			
			apdm.log.Debug("Batch failed, retrying",
				"attempt", attempt+1,
				"batch_size", len(batch),
				"error", err)
			
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
		
		if err != nil {
			failCount += len(batch)
			apdm.log.Error("Failed to add provider details batch",
				"transaction_id", job.TransactionID,
				"batch_start", i,
				"batch_size", len(batch),
				"error", err)
		}

		// Small delay between batches to avoid overwhelming the database
		if end < len(job.ProviderMaps) {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Update stats
	apdm.mu.Lock()
	apdm.stats.CompletedJobs++
	apdm.stats.ProcessedTokens += int64(successCount)
	if failCount > 0 {
		apdm.stats.FailedJobs++
	}
	apdm.mu.Unlock()

	duration := time.Since(startTime)
	apdm.log.Info("Provider details job completed",
		"transaction_id", job.TransactionID,
		"duration", duration,
		"success_count", successCount,
		"fail_count", failCount,
		"queue_time", startTime.Sub(job.SubmittedAt))
}

// statsMonitor periodically logs statistics
func (apdm *AsyncProviderDetailsManager) statsMonitor() {
	defer apdm.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		apdm.mu.RLock()
		stats := apdm.stats
		queueLen := len(apdm.jobQueue)
		apdm.mu.RUnlock()

		if stats.TotalJobs > 0 || queueLen > 0 {
			apdm.log.Info("Async provider details stats",
				"total_jobs", stats.TotalJobs,
				"completed", stats.CompletedJobs,
				"failed", stats.FailedJobs,
				"total_tokens", stats.TotalTokens,
				"processed_tokens", stats.ProcessedTokens,
				"queue_length", queueLen)
		}
	}
}

// GetStats returns current statistics
func (apdm *AsyncProviderDetailsManager) GetStats() ProviderDetailsStats {
	apdm.mu.RLock()
	defer apdm.mu.RUnlock()
	return apdm.stats
}

// Shutdown gracefully shuts down the manager
func (apdm *AsyncProviderDetailsManager) Shutdown() {
	apdm.log.Info("Shutting down async provider details manager")
	close(apdm.jobQueue)
	apdm.wg.Wait()
	apdm.log.Info("Async provider details manager shutdown complete")
}