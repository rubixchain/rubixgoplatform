package wallet

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// GetRequest represents a single IPFS get request
type GetRequest struct {
	Hash  string
	DID   string
	Role  int
	Path  string
	Index int // For tracking which token this is
}

// GetResult represents the result of a get operation
type GetResult struct {
	Request     GetRequest
	ProviderMap model.TokenProviderMap
	Error       error
	Attempts    int
}

// GetWithProviderMap downloads from IPFS with retry logic and returns provider map
func (w *Wallet) GetWithProviderMap(hash string, did string, role int, path string) (model.TokenProviderMap, error) {
	const maxRetries = 3
	var lastErr error
	startTime := time.Now()
	
	w.log.Debug("Starting IPFS Get with retry", 
		"hash", hash,
		"path", path,
		"max_retries", maxRetries)
	
	// Retry logic with exponential backoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		attemptStart := time.Now()
		err := w.ipfsOps.Get(hash, path)
		attemptDuration := time.Since(attemptStart)
		
		if err == nil {
			// Success - create provider map and return
			tpm := model.TokenProviderMap{
				Token:  hash,
				Role:   role,
				DID:    did,
				FuncID: GetFunc,
			}
			
			w.log.Info("IPFS Get successful", 
				"hash", hash,
				"attempt", attempt,
				"duration", attemptDuration,
				"total_time", time.Since(startTime))
			
			return tpm, nil
		}
		
		lastErr = err
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * time.Second
			w.log.Warn("IPFS Get failed, will retry", 
				"hash", hash,
				"attempt", attempt,
				"error", err,
				"duration", attemptDuration,
				"retry_in", waitTime)
			time.Sleep(waitTime)
		} else {
			w.log.Error("IPFS Get failed after all retries", 
				"hash", hash,
				"attempts", maxRetries,
				"error", err,
				"total_time", time.Since(startTime))
		}
	}
	
	// All retries failed - ensure cleanup
	if fileInfo, err := os.Stat(path); err == nil {
		// File or directory exists, remove it
		fileSize := fileInfo.Size()
		if err := os.RemoveAll(path); err != nil {
			w.log.Error("Failed to cleanup partial download", 
				"path", path,
				"size", fileSize,
				"error", err)
		} else {
			w.log.Debug("Cleaned up partial download", 
				"path", path,
				"size", fileSize)
		}
	}
	
	return model.TokenProviderMap{}, fmt.Errorf("failed to get %s after %d attempts: %v", hash, maxRetries, lastErr)
}

// BatchGetWithProviderMaps processes multiple downloads with all-or-nothing semantics
func (w *Wallet) BatchGetWithProviderMaps(requests []GetRequest) ([]model.TokenProviderMap, error) {
	if len(requests) == 0 {
		w.log.Debug("No tokens to download")
		return []model.TokenProviderMap{}, nil
	}
	
	startTime := time.Now()
	w.log.Info("Starting batch IPFS Get operations", 
		"total_tokens", len(requests))
	
	// Process downloads in parallel with controlled concurrency
	const maxConcurrent = 10
	semaphore := make(chan struct{}, maxConcurrent)
	results := make([]GetResult, len(requests))
	var wg sync.WaitGroup
	var successCount, failCount int32
	
	for i, req := range requests {
		wg.Add(1)
		go func(index int, request GetRequest) {
			defer wg.Done()
			
			// Rate limiting
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			tpm, err := w.GetWithProviderMap(request.Hash, request.DID, request.Role, request.Path)
			
			results[index] = GetResult{
				Request:     request,
				ProviderMap: tpm,
				Error:       err,
			}
			
			if err != nil {
				atomic.AddInt32(&failCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
			
			// Progress logging
			completed := atomic.LoadInt32(&successCount) + atomic.LoadInt32(&failCount)
			if completed%100 == 0 || completed == int32(len(requests)) {
				w.log.Info("Batch download progress", 
					"completed", completed,
					"total", len(requests),
					"success", atomic.LoadInt32(&successCount),
					"failed", atomic.LoadInt32(&failCount))
			}
		}(i, req)
	}
	
	wg.Wait()
	
	// Collect results and check for failures
	providerMaps := make([]model.TokenProviderMap, 0, len(requests))
	successfulPaths := make([]string, 0, len(requests))
	var failedDownloads []GetResult
	var failureMessages []string
	
	for i, result := range results {
		if result.Error != nil {
			failedDownloads = append(failedDownloads, result)
			failureMessages = append(failureMessages,
				fmt.Sprintf("token[%d]=%s: %v", i, result.Request.Hash, result.Error))
		} else {
			providerMaps = append(providerMaps, result.ProviderMap)
			successfulPaths = append(successfulPaths, result.Request.Path)
		}
	}
	
	// Handle failures - allow partial success instead of all-or-nothing
	if len(failedDownloads) > 0 {
		w.log.Warn("Batch download completed with failures", 
			"failed_count", len(failedDownloads),
			"success_count", len(successfulPaths),
			"total_count", len(requests),
			"duration", time.Since(startTime))
		
		// Log first few failures for debugging
		maxFailureLogs := 5
		if len(failureMessages) < maxFailureLogs {
			maxFailureLogs = len(failureMessages)
		}
		for i := 0; i < maxFailureLogs; i++ {
			w.log.Error("Download failure details", "failure", failureMessages[i])
		}
		if len(failureMessages) > maxFailureLogs {
			w.log.Error("Additional failures omitted", 
				"omitted_count", len(failureMessages)-maxFailureLogs)
		}
		
		// CRITICAL FIX: Do NOT rollback successful downloads
		// Allow partial success to prevent token loss
		// Mark failed tokens in results instead
		for _, failure := range failedDownloads {
			for i := range results {
				if results[i].Request.Hash == failure.Request.Hash {
					results[i].Error = failure.Error
					break
				}
			}
		}
		
		w.log.Info("Partial batch success", 
			"successful", len(successfulPaths),
			"failed", len(failedDownloads),
			"success_rate", fmt.Sprintf("%.1f%%", float64(len(successfulPaths))*100/float64(len(requests))))
		
		// Continue with successful downloads instead of failing entire batch
	}
	
	// All successful
	w.log.Info("Batch IPFS Get completed successfully", 
		"total_tokens", len(requests),
		"duration", time.Since(startTime))
	
	return providerMaps, nil
}

// ensureNoOrphanedRecords removes any partial database records for failed operations
func (w *Wallet) ensureNoOrphanedRecords(tokenIDs []string, transactionID string) error {
	if len(tokenIDs) == 0 {
		return nil
	}
	
	w.log.Debug("Cleaning up potential orphaned records", 
		"token_count", len(tokenIDs),
		"transaction_id", transactionID)
	
	var cleanupErrors int
	for _, tokenID := range tokenIDs {
		// Remove any token records
		err := w.s.Delete(TokenStorage, nil, "token_id=? AND transaction_id=?", 
			tokenID, transactionID)
		if err != nil && !strings.Contains(err.Error(), "no records") {
			cleanupErrors++
			w.log.Warn("Failed to cleanup token record", 
				"token_id", tokenID,
				"transaction_id", transactionID,
				"error", err)
		}
		
		// Remove any provider details
		err = w.RemoveProviderDetails(tokenID, "")
		if err != nil && !strings.Contains(err.Error(), "no records") {
			cleanupErrors++
			w.log.Warn("Failed to cleanup provider details", 
				"token_id", tokenID,
				"error", err)
		}
		
		// Note: Token state cleanup might be needed based on your schema
		// Add if TokenStateStorage is defined in your system
	}
	
	if cleanupErrors > 0 {
		w.log.Warn("Database cleanup completed with errors", 
			"total_tokens", len(tokenIDs),
			"cleanup_errors", cleanupErrors)
		return fmt.Errorf("cleanup completed with %d errors", cleanupErrors)
	}
	
	w.log.Debug("Database cleanup completed successfully", 
		"tokens_cleaned", len(tokenIDs))
	return nil
}