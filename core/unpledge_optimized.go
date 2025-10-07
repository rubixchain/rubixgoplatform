package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// InitiateOptimizedUnpledgeProcess performs parallel unpledging with production-grade features
func (c *Core) InitiateOptimizedUnpledgeProcess() (string, error) {
	startTime := time.Now()
	c.log.Info("Starting optimized unpledge process")

	// Get all unpledge sequences
	unpledgeSequenceInfo, err := c.w.GetUnpledgeSequenceDetails()
	if err != nil {
		c.log.Error("Failed to get unpledge sequences", "err", err)
		return "", fmt.Errorf("failed to get unpledge sequences: %w", err)
	}

	if len(unpledgeSequenceInfo) == 0 {
		c.log.Info("No tokens present to unpledge")
		return "No tokens present to unpledge", nil
	}

	c.log.Info("Found unpledge sequences", "count", len(unpledgeSequenceInfo))

	// Pre-filter ready transactions to avoid unnecessary processing
	readyTransactions, err := c.filterReadyToUnpledge(unpledgeSequenceInfo)
	if err != nil {
		c.log.Error("Failed to filter ready transactions", "err", err)
		return "", fmt.Errorf("failed to filter transactions: %w", err)
	}

	if len(readyTransactions) == 0 {
		c.log.Info("No tokens ready to unpledge yet")
		return "No tokens ready to unpledge yet", nil
	}

	c.log.Info("Transactions ready to unpledge", "count", len(readyTransactions))

	// Determine optimal pool configuration based on workload
	poolConfig := c.getOptimalPoolConfig(len(readyTransactions))

	// Create worker pool
	pool, err := NewUnpledgeWorkerPool(poolConfig)
	if err != nil {
		c.log.Error("Failed to create worker pool", "err", err)
		return "", fmt.Errorf("failed to create worker pool: %w", err)
	}

	// Start the pool
	if err := pool.Start(); err != nil {
		c.log.Error("Failed to start worker pool", "err", err)
		return "", fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Ensure pool is stopped on exit
	defer func() {
		if stopErr := pool.Stop(); stopErr != nil {
			c.log.Error("Error stopping worker pool", "err", stopErr)
		}
	}()

	// Process transactions
	results, err := c.processUnpledgeTransactions(pool, readyTransactions)
	if err != nil {
		c.log.Error("Failed to process transactions", "err", err)
		return "", fmt.Errorf("failed to process transactions: %w", err)
	}

	// Generate summary
	summary := c.generateUnpledgeSummary(results, time.Since(startTime))

	// Log final metrics if available
	if poolConfig.EnableMetrics {
		metrics := pool.GetMetrics()
		c.log.Info("Unpledge process completed",
			"total_transactions", metrics.TotalTransactions,
			"successful", metrics.SuccessfulTransactions,
			"failed", metrics.FailedTransactions,
			"total_tokens", metrics.TotalTokens,
			"avg_latency_ms", metrics.AverageLatency,
			"total_time", time.Since(startTime))
	}

	return summary, nil
}

// filterReadyToUnpledge filters transactions that are ready to unpledge
func (c *Core) filterReadyToUnpledge(sequences []*wallet.UnpledgeSequenceInfo) ([]*wallet.UnpledgeSequenceInfo, error) {
	ready := make([]*wallet.UnpledgeSequenceInfo, 0, len(sequences))
	currentTimeEpoch := time.Now().Unix()

	// Use goroutines for parallel filtering of large datasets
	if len(sequences) > 100 {
		return c.filterReadyToUnpledgeParallel(sequences, currentTimeEpoch)
	}

	// Sequential filtering for smaller datasets
	for _, info := range sequences {
		isReady, err := c.isTransactionReadyToUnpledge(info, currentTimeEpoch)
		if err != nil {
			c.log.Error("Error checking transaction readiness",
				"txn_id", info.TransactionID, "err", err)
			continue
		}

		if isReady {
			ready = append(ready, info)
		}
	}

	return ready, nil
}

// filterReadyToUnpledgeParallel performs parallel filtering for large datasets
func (c *Core) filterReadyToUnpledgeParallel(sequences []*wallet.UnpledgeSequenceInfo, currentTimeEpoch int64) ([]*wallet.UnpledgeSequenceInfo, error) {
	ready := make([]*wallet.UnpledgeSequenceInfo, 0, len(sequences))
	readyMu := &sync.Mutex{}

	// Worker pool for filtering
	workers := 10
	if len(sequences) < 1000 {
		workers = 5
	}

	workChan := make(chan *wallet.UnpledgeSequenceInfo, len(sequences))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for info := range workChan {
				isReady, err := c.isTransactionReadyToUnpledge(info, currentTimeEpoch)
				if err != nil {
					c.log.Error("Error checking transaction readiness",
						"txn_id", info.TransactionID, "err", err)
					continue
				}

				if isReady {
					readyMu.Lock()
					ready = append(ready, info)
					readyMu.Unlock()
				}
			}
		}()
	}

	// Send work
	for _, info := range sequences {
		workChan <- info
	}
	close(workChan)

	// Wait for completion
	wg.Wait()

	return ready, nil
}

// isTransactionReadyToUnpledge checks if a transaction is ready to unpledge
func (c *Core) isTransactionReadyToUnpledge(info *wallet.UnpledgeSequenceInfo, currentTimeEpoch int64) (bool, error) {
	// Get token state hashes for the transaction
	tokenStateHashDetails, err := c.w.GetTokenStateHashByTransactionID(info.TransactionID)
	if err != nil {
		return false, fmt.Errorf("failed to get token state hashes: %w", err)
	}

	// If no token state hashes found, all tokens have undergone state change
	if len(tokenStateHashDetails) == 0 {
		c.log.Debug("All tokens have undergone state change", "txn_id", info.TransactionID)
		return true, nil
	}

	// Check if pledge period has passed
	if (currentTimeEpoch - info.Epoch) > int64(pledgePeriodInSeconds) {
		c.log.Debug("Pledge period has passed", "txn_id", info.TransactionID)
		return true, nil
	}

	return false, nil
}

// processUnpledgeTransactions processes all ready transactions using the worker pool
func (c *Core) processUnpledgeTransactions(pool *UnpledgeWorkerPool, transactions []*wallet.UnpledgeSequenceInfo) ([]*UnpledgeResult, error) {
	results := make([]*UnpledgeResult, 0, len(transactions))
	resultChan := make(chan *UnpledgeResult, len(transactions))

	// Submit all tasks
	for _, info := range transactions {
		task := &UnpledgeTask{
			Info:   info,
			Core:   c,
			Result: resultChan,
		}

		if err := pool.Submit(task); err != nil {
			c.log.Error("Failed to submit task", "txn_id", info.TransactionID, "err", err)
			// Continue with other tasks
			resultChan <- &UnpledgeResult{
				TransactionID: info.TransactionID,
				Success:       false,
				Error:         err,
			}
		}
	}

	// Collect results with timeout
	timeout := time.After(5 * time.Minute) // Overall timeout
	for i := 0; i < len(transactions); i++ {
		select {
		case result := <-resultChan:
			results = append(results, result)

			// Log result
			if result.Success {
				c.log.Info("Transaction unpledged successfully",
					"txn_id", result.TransactionID,
					"amount", result.UnpledgeAmount,
					"tokens", result.TokenCount,
					"time", result.ProcessingTime)
			} else {
				c.log.Error("Failed to unpledge transaction",
					"txn_id", result.TransactionID,
					"err", result.Error)
			}

		case <-timeout:
			c.log.Error("Timeout waiting for unpledge results")
			return results, fmt.Errorf("timeout processing transactions")
		}
	}

	return results, nil
}

// getOptimalPoolConfig determines optimal configuration based on workload
func (c *Core) getOptimalPoolConfig(transactionCount int) config.UnpledgePoolConfig {
	poolConfig := DefaultUnpledgePoolConfig()

	// Check if custom config is provided
	if c.cfg != nil && c.cfg.CfgData.UnpledgeConfig != nil {
		// Use custom configuration if provided
		custom := c.cfg.CfgData.UnpledgeConfig
		if custom.MaxWorkers > 0 {
			poolConfig.MaxWorkers = custom.MaxWorkers
		}
		if custom.TokenConcurrency > 0 {
			poolConfig.TokenConcurrency = custom.TokenConcurrency
		}
		if custom.BatchSize > 0 {
			poolConfig.BatchSize = custom.BatchSize
		}
	}

	// Adjust based on transaction count
	if transactionCount < 10 {
		poolConfig.MaxWorkers = 2
		poolConfig.TokenConcurrency = 3
	} else if transactionCount < 50 {
		poolConfig.MaxWorkers = min(4, poolConfig.MaxWorkers)
		poolConfig.TokenConcurrency = min(5, poolConfig.TokenConcurrency)
	} else if transactionCount < 100 {
		poolConfig.MaxWorkers = min(6, poolConfig.MaxWorkers)
		poolConfig.TokenConcurrency = min(8, poolConfig.TokenConcurrency)
	} else {
		// For large workloads, use configured maximums
		poolConfig.MaxWorkers = min(poolConfig.MaxWorkers, 16)
		poolConfig.TokenConcurrency = min(poolConfig.TokenConcurrency, 10)
	}

	// Ensure queue size is adequate
	poolConfig.QueueSize = max(transactionCount*2, 1000)

	c.log.Debug("Unpledge pool configuration",
		"workers", poolConfig.MaxWorkers,
		"token_concurrency", poolConfig.TokenConcurrency,
		"queue_size", poolConfig.QueueSize)

	return poolConfig
}

// generateUnpledgeSummary creates a summary of the unpledge operation
func (c *Core) generateUnpledgeSummary(results []*UnpledgeResult, totalTime time.Duration) string {
	var totalAmount float64
	var successCount, failedCount, totalTokens int

	for _, result := range results {
		if result.Success {
			successCount++
			totalAmount += result.UnpledgeAmount
			totalTokens += result.TokenCount
		} else {
			failedCount++
		}
	}

	if successCount == 0 && failedCount == 0 {
		return "No transactions processed"
	}

	if successCount > 0 {
		return fmt.Sprintf(
			"Optimized unpledging completed: %d/%d transactions successful, "+
				"Total Amount: %.6f RBT, Total Tokens: %d, Time: %v",
			successCount, len(results), totalAmount, totalTokens, totalTime)
	}

	return fmt.Sprintf("Unpledging failed: %d transactions failed", failedCount)
}

// Helper functions are already defined in resource_monitor.go

