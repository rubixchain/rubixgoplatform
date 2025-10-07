package core

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// PinStatus represents the status of a pinning operation
type PinStatus int

const (
	PinStatusQueued PinStatus = iota
	PinStatusInProgress
	PinStatusCompleted
	PinStatusFailed
)

// AsyncPinJob represents a background pinning job
type AsyncPinJob struct {
	TransactionID    string
	TokenCount       int
	StartTime        time.Time
	CompletedCount   int32
	FailedCount      int32
	Status           PinStatus
	Error            error
	TokenStateChecks []TokenStateCheckResult
	DID              string
	Sender           string
	Receiver         string
	TokenValue       float64
}

// AsyncPinManager manages background token pinning
type AsyncPinManager struct {
	core      *Core
	log       logger.Logger
	jobs      sync.Map // transactionID -> *AsyncPinJob
	jobQueue  chan *AsyncPinJob
	workers   int
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
}

// NewAsyncPinManager creates a new async pin manager
func NewAsyncPinManager(core *Core, workers int) *AsyncPinManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	if workers <= 0 {
		workers = 4 // Default workers
	}
	
	apm := &AsyncPinManager{
		core:     core,
		log:      core.log.Named("AsyncPinManager"),
		jobQueue: make(chan *AsyncPinJob, 100),
		workers:  workers,
		ctx:      ctx,
		cancel:   cancel,
	}
	
	// Start worker pool
	for i := 0; i < workers; i++ {
		apm.wg.Add(1)
		go apm.worker(i)
	}
	
	// Start status monitor
	apm.wg.Add(1)
	go apm.statusMonitor()
	
	return apm
}

// SubmitPinJob submits a new pinning job to the background queue
func (apm *AsyncPinManager) SubmitPinJob(
	tokenStateCheckResult []TokenStateCheckResult,
	did, transactionId, sender, receiver string,
	tokenValue float64,
) error {
	job := &AsyncPinJob{
		TransactionID:    transactionId,
		TokenCount:       len(tokenStateCheckResult),
		StartTime:        time.Now(),
		Status:           PinStatusQueued,
		TokenStateChecks: tokenStateCheckResult,
		DID:              did,
		Sender:           sender,
		Receiver:         receiver,
		TokenValue:       tokenValue,
	}
	
	// Store job
	apm.jobs.Store(transactionId, job)
	
	// Queue for processing
	select {
	case apm.jobQueue <- job:
		apm.log.Info("Pin job queued", 
			"transaction_id", transactionId,
			"token_count", job.TokenCount)
		return nil
	case <-time.After(5 * time.Second):
		job.Status = PinStatusFailed
		job.Error = fmt.Errorf("job queue full, timeout submitting job")
		return job.Error
	}
}

// GetJobStatus returns the status of a pinning job
func (apm *AsyncPinManager) GetJobStatus(transactionID string) (*AsyncPinJob, bool) {
	if job, ok := apm.jobs.Load(transactionID); ok {
		return job.(*AsyncPinJob), true
	}
	return nil, false
}

// worker processes pinning jobs
func (apm *AsyncPinManager) worker(id int) {
	defer apm.wg.Done()
	
	for {
		select {
		case <-apm.ctx.Done():
			return
		case job := <-apm.jobQueue:
			apm.processJob(job)
		}
	}
}

// processJob processes a single pinning job
func (apm *AsyncPinManager) processJob(job *AsyncPinJob) {
	job.Status = PinStatusInProgress
	startTime := time.Now()
	
	apm.log.Info("Starting async pin job",
		"transaction_id", job.TransactionID,
		"token_count", job.TokenCount)
	
	// Process in batches optimized for different token counts
	batchSize := 100
	if job.TokenCount > 500 {
		batchSize = 50 // Smaller batches for larger transactions to maintain responsiveness
	}
	
	var (
		providerMaps     []model.TokenProviderMap
		providerMapMutex sync.Mutex
		wg               sync.WaitGroup
		semaphore        = make(chan struct{}, 5) // Limit concurrent pins
		lastLoggedPercent int32 = 0
	)
	
	for i := 0; i < job.TokenCount; i += batchSize {
		end := i + batchSize
		if end > job.TokenCount {
			end = job.TokenCount
		}
		
		// Process batch
		for j := i; j < end; j++ {
			if job.TokenStateChecks[j].Error != nil || job.TokenStateChecks[j].Exhausted {
				continue
			}
			
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				
				// Rate limit
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				
				data := job.TokenStateChecks[index].tokenIDTokenStateData
				
				// Pin with retry
				err := retryAsync(func() error {
					_, tpm, err := apm.core.w.AddWithProviderMap(
						bytes.NewBuffer([]byte(data)),
						job.DID,
						wallet.QuorumPinRole,
					)
					if err != nil {
						return err
					}
					
					// Fill in extra fields
					tpm.FuncID = wallet.PinFunc
					tpm.TransactionID = job.TransactionID
					tpm.Sender = job.Sender
					tpm.Receiver = job.Receiver
					tpm.TokenValue = job.TokenValue
					
					// Add to collection
					providerMapMutex.Lock()
					providerMaps = append(providerMaps, tpm)
					providerMapMutex.Unlock()
					
					return nil
				}, 3, 2*time.Second)
				
				if err != nil {
					atomic.AddInt32(&job.FailedCount, 1)
					apm.log.Error("Failed to pin token state",
						"transaction_id", job.TransactionID,
						"error", err)
				} else {
					atomic.AddInt32(&job.CompletedCount, 1)
				}
			}(j)
		}
		
		// Wait for batch to complete
		wg.Wait()
		
		// Log progress based on percentage
		completed := atomic.LoadInt32(&job.CompletedCount)
		failed := atomic.LoadInt32(&job.FailedCount)
		processed := completed + failed
		currentPercent := int32((float64(processed) * 100) / float64(job.TokenCount))
		
		// Log every 20% or at completion for large jobs, every 10% for smaller ones
		logInterval := int32(10)
		if job.TokenCount > 1000 {
			logInterval = 20
		}
		
		if currentPercent >= lastLoggedPercent+logInterval || processed == int32(job.TokenCount) {
			apm.log.Info("Async pin progress",
				"transaction_id", job.TransactionID,
				"percent", currentPercent,
				"completed", completed,
				"failed", failed,
				"total", job.TokenCount)
			lastLoggedPercent = (currentPercent / logInterval) * logInterval
		}
		
		// Small delay between batches
		if end < job.TokenCount {
			time.Sleep(500 * time.Millisecond)
		}
	}
	
	// Update final status
	if job.FailedCount > 0 {
		job.Status = PinStatusFailed
		job.Error = fmt.Errorf("failed to pin %d tokens", job.FailedCount)
	} else {
		job.Status = PinStatusCompleted
	}
	
	duration := time.Since(startTime)
	apm.log.Info("Async pin job completed",
		"transaction_id", job.TransactionID,
		"duration", duration,
		"completed", job.CompletedCount,
		"failed", job.FailedCount,
		"status", job.Status)
	
	// Store provider maps if successful
	if len(providerMaps) > 0 && job.FailedCount == 0 {
		// Provider maps are already stored by AddWithProviderMap
		apm.log.Debug("Provider maps stored successfully",
			"transaction_id", job.TransactionID,
			"count", len(providerMaps))
	}
}

// statusMonitor periodically logs status of active jobs
func (apm *AsyncPinManager) statusMonitor() {
	defer apm.wg.Done()
	
	ticker := time.NewTicker(60 * time.Second) // Reduced frequency to minimize log noise
	defer ticker.Stop()
	
	for {
		select {
		case <-apm.ctx.Done():
			return
		case <-ticker.C:
			apm.logStatus()
		}
	}
}

// logStatus logs the current status of all jobs
func (apm *AsyncPinManager) logStatus() {
	var queued, inProgress, completed, failed int
	
	apm.jobs.Range(func(key, value interface{}) bool {
		job := value.(*AsyncPinJob)
		switch job.Status {
		case PinStatusQueued:
			queued++
		case PinStatusInProgress:
			inProgress++
		case PinStatusCompleted:
			completed++
		case PinStatusFailed:
			failed++
		}
		return true
	})
	
	if queued > 0 || inProgress > 0 {
		apm.log.Info("Async pin manager status",
			"queued", queued,
			"in_progress", inProgress,
			"completed", completed,
			"failed", failed)
	}
}

// WaitForJob waits for a specific job to complete with timeout
func (apm *AsyncPinManager) WaitForJob(transactionID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		if job, ok := apm.GetJobStatus(transactionID); ok {
			switch job.Status {
			case PinStatusCompleted:
				return nil
			case PinStatusFailed:
				return job.Error
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	return fmt.Errorf("timeout waiting for pin job %s", transactionID)
}

// Shutdown gracefully shuts down the async pin manager
func (apm *AsyncPinManager) Shutdown() {
	apm.log.Info("Shutting down async pin manager")
	apm.cancel()
	close(apm.jobQueue)
	apm.wg.Wait()
	apm.log.Info("Async pin manager shutdown complete")
}

// CleanupOldJobs removes completed/failed jobs older than the specified duration
func (apm *AsyncPinManager) CleanupOldJobs(olderThan time.Duration) int {
	cutoff := time.Now().Add(-olderThan)
	cleaned := 0
	
	apm.jobs.Range(func(key, value interface{}) bool {
		job := value.(*AsyncPinJob)
		if (job.Status == PinStatusCompleted || job.Status == PinStatusFailed) && 
			job.StartTime.Before(cutoff) {
			apm.jobs.Delete(key)
			cleaned++
		}
		return true
	})
	
	if cleaned > 0 {
		apm.log.Info("Cleaned up old pin jobs", "count", cleaned)
	}
	
	return cleaned
}

// retryAsync helper function
func retryAsync(fn func() error, attempts int, delay time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(delay)
		}
	}
	return err
}