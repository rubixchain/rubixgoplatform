package wallet

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
)

// ParallelFTReceiverFix contains fixes for mainnet FT transfer issues
// This file contains targeted fixes for the 250 token transfer failure

// processTokensParallelWithRetry adds retry logic for failed tokens
func (pfr *ParallelFTReceiver) processTokensParallelWithRetry(
	ctx context.Context,
	tokens []contract.TokenInfo,
	hashResults map[string]HashResult,
	downloadResults []DownloadResult,
	existingTokenIndices []int,
	b *block.Block,
	ftInfo FTToken,
	did string,
	senderPeerId string,
	receiverPeerId string,
	tokenCreatorMap map[string]string,
) []TokenProcessingResult {
	
	// First attempt
	results := pfr.processTokensParallel(ctx, tokens, hashResults, downloadResults, 
		existingTokenIndices, b, ftInfo, did, senderPeerId, receiverPeerId, tokenCreatorMap)
	
	// Count failures
	var failedCount int32
	failedItems := make([]workItem, 0)
	
	for i, result := range results {
		if result.Error != nil {
			atomic.AddInt32(&failedCount, 1)
			// Reconstruct work item for retry
			isNewToken := true
			for _, existingIdx := range existingTokenIndices {
				if existingIdx == i {
					isNewToken = false
					break
				}
			}
			
			var downloadResult *DownloadResult
			for _, dr := range downloadResults {
				if dr.Task.Index == i {
					downloadResult = &dr
					break
				}
			}
			
			failedItems = append(failedItems, workItem{
				Index:          i,
				Token:          tokens[i],
				IsNewToken:     isNewToken,
				DownloadResult: downloadResult,
			})
		}
	}
	
	// Retry failed tokens with smaller batch size and delay
	if len(failedItems) > 0 {
		pfr.log.Warn("Retrying failed tokens", 
			"failed_count", failedCount,
			"total_tokens", len(tokens))
		
		// Process failed tokens one by one with delay
		for _, item := range failedItems {
			time.Sleep(100 * time.Millisecond) // Small delay between retries
			
			hashResult := hashResults[item.Token.Token]
			retryResult := pfr.processSingleToken(item, hashResult, b, ftInfo, 
				did, senderPeerId, receiverPeerId, tokenCreatorMap)
			
			if retryResult.Success {
				results[item.Index] = retryResult
				pfr.log.Debug("Token retry successful", "token", item.Token.Token)
			} else {
				pfr.log.Error("Token retry failed", 
					"token", item.Token.Token,
					"error", retryResult.Error)
			}
		}
	}
	
	return results
}

// enhancedDatabaseWrite adds retry logic for database writes
func (pfr *ParallelFTReceiver) enhancedDatabaseWrite(storage string, data interface{}) error {
	const maxRetries = 3
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := pfr.withMinimalLock(func() error {
			return pfr.w.s.Write(storage, data)
		})
		
		if err == nil {
			return nil
		}
		
		lastErr = err
		pfr.log.Warn("Database write failed, retrying", 
			"attempt", attempt,
			"storage", storage,
			"error", err)
		
		// Exponential backoff
		time.Sleep(time.Duration(attempt*100) * time.Millisecond)
	}
	
	return fmt.Errorf("database write failed after %d attempts: %w", maxRetries, lastErr)
}

// validateMainnetLimits checks if the transfer size is appropriate for mainnet
func (pfr *ParallelFTReceiver) validateMainnetLimits(tokenCount int) error {
	// Check worker scaling for mainnet
	workers := pfr.calculateDynamicWorkers(tokenCount)
	
	// For mainnet, be more conservative with concurrency
	if tokenCount > 100 && workers > 16 {
		pfr.log.Info("Limiting workers for mainnet stability", 
			"requested_workers", workers,
			"limited_to", 16)
		pfr.batchWorkers = 16
	}
	
	// Warn about large transfers
	if tokenCount > 200 {
		pfr.log.Warn("Large FT transfer on mainnet may experience issues", 
			"token_count", tokenCount,
			"recommended_max", 200)
	}
	
	return nil
}

// debugTokenProcessing adds detailed logging for debugging mainnet issues
func (pfr *ParallelFTReceiver) debugTokenProcessing(
	tokens []contract.TokenInfo,
	results []TokenProcessingResult,
) {
	// Log summary
	var successCount, downloadFailures, dbFailures, otherFailures int
	
	for _, result := range results {
		if result.Success {
			successCount++
		} else if result.Error != nil {
			errStr := result.Error.Error()
			switch {
			case contains(errStr, "download failed"):
				downloadFailures++
			case contains(errStr, "database") || contains(errStr, "write"):
				dbFailures++
			default:
				otherFailures++
			}
		}
	}
	
	pfr.log.Info("Token processing summary",
		"total", len(tokens),
		"success", successCount,
		"download_failures", downloadFailures,
		"db_failures", dbFailures,
		"other_failures", otherFailures)
	
	// Log first few failures for debugging
	failureCount := 0
	for i, result := range results {
		if !result.Success && result.Error != nil && failureCount < 5 {
			pfr.log.Error("Token processing failed",
				"index", i,
				"token", result.Token,
				"error", result.Error)
			failureCount++
		}
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}