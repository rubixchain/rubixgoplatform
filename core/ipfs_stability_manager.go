package core

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// IPFSStabilityManager provides stability features for IPFS during high load
type IPFSStabilityManager struct {
	core              *Core
	log               logger.Logger
	maxConcurrent     int32
	currentOperations int32
	operationTimeout  time.Duration
	retryDelay        time.Duration
	mu                sync.RWMutex
}

// NewIPFSStabilityManager creates a new stability manager
func NewIPFSStabilityManager(core *Core) *IPFSStabilityManager {
	return &IPFSStabilityManager{
		core:             core,
		log:              core.log.Named("IPFSStability"),
		maxConcurrent:    20, // Limit concurrent IPFS operations
		operationTimeout: 30 * time.Second,
		retryDelay:       100 * time.Millisecond,
	}
}

// SetMaxConcurrent adjusts the maximum concurrent operations based on token count
func (ism *IPFSStabilityManager) SetMaxConcurrent(tokenCount int) {
	// Dynamically adjust based on transaction size
	switch {
	case tokenCount <= 100:
		atomic.StoreInt32(&ism.maxConcurrent, 50)
	case tokenCount <= 500:
		atomic.StoreInt32(&ism.maxConcurrent, 30)
	case tokenCount <= 1000:
		atomic.StoreInt32(&ism.maxConcurrent, 20)
	case tokenCount <= 1500:
		atomic.StoreInt32(&ism.maxConcurrent, 15)
	default:
		atomic.StoreInt32(&ism.maxConcurrent, 10)
	}
	
	ism.log.Info("Adjusted IPFS concurrency limit", 
		"token_count", tokenCount,
		"max_concurrent", atomic.LoadInt32(&ism.maxConcurrent))
}

// WaitForSlot waits for an available operation slot
func (ism *IPFSStabilityManager) WaitForSlot(ctx context.Context) error {
	for {
		current := atomic.LoadInt32(&ism.currentOperations)
		max := atomic.LoadInt32(&ism.maxConcurrent)
		
		if current < max {
			if atomic.CompareAndSwapInt32(&ism.currentOperations, current, current+1) {
				return nil
			}
		}
		
		// Check context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Retry
		}
	}
}

// ReleaseSlot releases an operation slot
func (ism *IPFSStabilityManager) ReleaseSlot() {
	atomic.AddInt32(&ism.currentOperations, -1)
}

// StableAdd performs IPFS add with stability features
func (ism *IPFSStabilityManager) StableAdd(data []byte, options ...ipfsapi.AddOpts) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ism.operationTimeout)
	defer cancel()
	
	// Wait for slot
	if err := ism.WaitForSlot(ctx); err != nil {
		return "", fmt.Errorf("timeout waiting for IPFS slot: %w", err)
	}
	defer ism.ReleaseSlot()
	
	// Add delay between operations to prevent overwhelming IPFS
	time.Sleep(ism.retryDelay)
	
	// Try the operation with retries
	var hash string
	var lastErr error
	
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			ism.log.Debug("Retrying IPFS add", "attempt", attempt+1)
			time.Sleep(time.Duration(attempt) * ism.retryDelay)
		}
		
		// Check if IPFS is available
		if ism.core.ipfs == nil {
			ism.log.Warn("IPFS not available, waiting")
			time.Sleep(5 * time.Second)
			continue
		}
		
		// Perform the add
		hash, lastErr = IpfsAddWithBackoff(ism.core.ipfs, bytes.NewReader(data), options...)
		if lastErr == nil {
			return hash, nil
		}
		
		ism.log.Debug("IPFS add failed", "attempt", attempt+1, "error", lastErr)
	}
	
	return "", fmt.Errorf("IPFS add failed after 3 attempts: %w", lastErr)
}

// StableGet performs IPFS get with stability features
func (ism *IPFSStabilityManager) StableGet(hash string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ism.operationTimeout)
	defer cancel()
	
	// Wait for slot
	if err := ism.WaitForSlot(ctx); err != nil {
		return nil, fmt.Errorf("timeout waiting for IPFS slot: %w", err)
	}
	defer ism.ReleaseSlot()
	
	// Try the operation with retries
	var data []byte
	var lastErr error
	
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * ism.retryDelay)
		}
		
		// Check if IPFS is available
		if ism.core.ipfs == nil {
			ism.log.Warn("IPFS not available, waiting")
			time.Sleep(5 * time.Second)
			continue
		}
		
		// Perform the get
		reader, err := ism.core.ipfs.Cat(hash)
		if err != nil {
			lastErr = err
			continue
		}
		
		var buf bytes.Buffer
		_, err = buf.ReadFrom(reader)
		reader.Close()
		
		if err != nil {
			lastErr = err
			continue
		}
		
		data = buf.Bytes()
		lastErr = nil
		if lastErr == nil {
			return data, nil
		}
	}
	
	return nil, fmt.Errorf("IPFS get failed after 3 attempts: %w", lastErr)
}

// BatchOperationWithStability performs batch operations with stability
func (ism *IPFSStabilityManager) BatchOperationWithStability(
	items [][]byte,
	operation func([]byte) (string, error),
) ([]string, error) {
	results := make([]string, len(items))
	errors := make([]error, len(items))
	
	// Use a worker pool with limited concurrency
	workers := int(atomic.LoadInt32(&ism.maxConcurrent)) / 2 // Use half the limit for batch ops
	if workers < 1 {
		workers = 1
	}
	
	semaphore := make(chan struct{}, workers)
	var wg sync.WaitGroup
	
	for i, item := range items {
		wg.Add(1)
		go func(index int, data []byte) {
			defer wg.Done()
			
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			// Add jitter to prevent thundering herd
			time.Sleep(time.Duration(index%10) * 10 * time.Millisecond)
			
			result, err := operation(data)
			results[index] = result
			errors[index] = err
		}(i, item)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return results, fmt.Errorf("batch operation failed at index %d: %w", i, err)
		}
	}
	
	return results, nil
}

// MonitorIPFSHealth monitors IPFS health during operations
func (ism *IPFSStabilityManager) MonitorIPFSHealth(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	unhealthyCount := 0
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ism.core.ipfs == nil {
				unhealthyCount++
				ism.log.Warn("IPFS unhealthy during operation", 
					"count", unhealthyCount,
					"current_ops", atomic.LoadInt32(&ism.currentOperations))
				
				// Reduce concurrency if IPFS is struggling
				if unhealthyCount > 2 {
					current := atomic.LoadInt32(&ism.maxConcurrent)
					if current > 5 {
						atomic.StoreInt32(&ism.maxConcurrent, current/2)
						ism.log.Warn("Reduced IPFS concurrency due to health issues", 
							"new_limit", current/2)
					}
				}
			} else {
				unhealthyCount = 0
			}
		}
	}
}

// GetStats returns current statistics
func (ism *IPFSStabilityManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"current_operations": atomic.LoadInt32(&ism.currentOperations),
		"max_concurrent":     atomic.LoadInt32(&ism.maxConcurrent),
		"operation_timeout":  ism.operationTimeout,
	}
}