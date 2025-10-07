package core

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// OptimizedFTTransferLocking handles efficient locking of FT tokens for large transfers
func (c *Core) OptimizedFTTransferLocking(ftsForTxn []wallet.FTToken, did string, numTokens int) ([]contract.TokenInfo, error) {
	startTime := time.Now()
	
	// Progress tracking
	var completed int32
	var lastLoggedPercent int32
	
	// Result collection
	tokenInfos := make([]contract.TokenInfo, len(ftsForTxn))
	errors := make(chan error, len(ftsForTxn))
	
	// Process tokens in parallel batches
	batchSize := 100
	// Use CPU-based worker count for better system adaptation
	numWorkers := runtime.NumCPU()
	// Limit workers for SQLite to avoid database locking
	if numWorkers > 4 {
		numWorkers = 4 // SQLite performs better with limited concurrent writes
	}
	if len(ftsForTxn) < 100 {
		numWorkers = 1 // Small transfers don't need parallelism
	} else if len(ftsForTxn) < 500 {
		numWorkers = 2
	}
	
	c.log.Info("Starting optimized FT token locking", 
		"total_tokens", len(ftsForTxn),
		"workers", numWorkers,
		"batch_size", batchSize)
	
	// Create job channel
	type ftLockJob struct {
		index int
		token wallet.FTToken
	}
	jobs := make(chan ftLockJob, len(ftsForTxn))
	
	// Worker function
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for job := range jobs {
				// Lock the token
				job.token.TokenStatus = wallet.TokenIsLocked
				err := c.s.Update(wallet.FTTokenStorage, &job.token, "token_id=?", job.token.TokenID)
				if err != nil {
					errors <- fmt.Errorf("failed to lock FT token %s: %v", job.token.TokenID, err)
					continue
				}
				
				// Get token block info
				tt := c.TokenType(FTString)
				blk := c.w.GetLatestTokenBlock(job.token.TokenID, tt)
				if blk == nil {
					errors <- fmt.Errorf("failed to get latest block for token %s", job.token.TokenID)
					continue
				}
				
				bid, err := blk.GetBlockID(job.token.TokenID)
				if err != nil {
					errors <- fmt.Errorf("failed to get block id for token %s: %v", job.token.TokenID, err)
					continue
				}
				
				// Get token info from pool and populate it
				ti := c.tokenPool.Get()
				ti.Token = job.token.TokenID
				ti.TokenType = tt
				ti.TokenValue = job.token.TokenValue
				ti.OwnerDID = did
				ti.BlockID = bid
				
				// Store in result array
				tokenInfos[job.index] = *ti
				
				// Return to pool
				c.tokenPool.Put(ti)
				
				// Update progress
				newCount := atomic.AddInt32(&completed, 1)
				currentPercent := int32((float64(newCount) * 100) / float64(len(ftsForTxn)))
				
				// Log progress every 10%
				if currentPercent >= lastLoggedPercent+10 {
					if atomic.CompareAndSwapInt32(&lastLoggedPercent, lastLoggedPercent/10*10, currentPercent/10*10) {
						c.log.Info(fmt.Sprintf("FT locking progress: %d%% (%d/%d locked)", 
							currentPercent, newCount, len(ftsForTxn)))
					}
				}
			}
		}(w)
	}
	
	// Submit all jobs
	for i, token := range ftsForTxn {
		jobs <- ftLockJob{index: i, token: token}
	}
	close(jobs)
	
	// Wait for all workers to complete
	wg.Wait()
	close(errors)
	
	// Check for errors
	var firstErr error
	errorCount := 0
	for err := range errors {
		if firstErr == nil {
			firstErr = err
		}
		errorCount++
	}
	
	if errorCount > 0 {
		// Rollback on error - unlock tokens that were locked
		c.log.Error("Errors occurred during FT locking, rolling back", 
			"error_count", errorCount,
			"first_error", firstErr)
		
		c.rollbackFTLocking(ftsForTxn)
		return nil, firstErr
	}
	
	c.log.Info("Optimized FT token locking completed", 
		"total_tokens", len(ftsForTxn),
		"duration", time.Since(startTime))
	
	return tokenInfos, nil
}

// rollbackFTLocking unlocks tokens in case of error
func (c *Core) rollbackFTLocking(ftsForTxn []wallet.FTToken) {
	c.log.Info("Rolling back FT token locks")
	
	var wg sync.WaitGroup
	for i := range ftsForTxn {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := ftsForTxn[idx]
			token.TokenStatus = wallet.TokenIsFree
			err := c.s.Update(wallet.FTTokenStorage, &token, "token_id=?", token.TokenID)
			if err != nil {
				c.log.Error("Failed to rollback FT token lock", 
					"token_id", token.TokenID, 
					"error", err)
			}
		}(i)
	}
	wg.Wait()
}

// shouldUseOptimizedFTLocking determines if optimized locking should be used
func (c *Core) shouldUseOptimizedFTLocking(ftCount int) bool {
	// Use optimized path for transfers with more than 100 FTs
	return ftCount > 100
}