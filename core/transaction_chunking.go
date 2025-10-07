package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	// MaxTokensPerChunk is the optimal size for parallel processing
	MaxTokensPerChunk = 250
	// MaxParallelChunks limits concurrent chunk processing
	MaxParallelChunks = 4
)

// TransactionChunker handles splitting large transactions
type TransactionChunker struct {
	core *Core
	log  logger.Logger
}

// NewTransactionChunker creates a new chunker
func NewTransactionChunker(core *Core) *TransactionChunker {
	return &TransactionChunker{
		core: core,
		log:  core.log.Named("TxChunker"),
	}
}

// ChunkTokenInfo splits token info into manageable chunks
func (tc *TransactionChunker) ChunkTokenInfo(tokens []contract.TokenInfo) [][]contract.TokenInfo {
	if len(tokens) <= MaxTokensPerChunk {
		return [][]contract.TokenInfo{tokens}
	}

	var chunks [][]contract.TokenInfo
	for i := 0; i < len(tokens); i += MaxTokensPerChunk {
		end := i + MaxTokensPerChunk
		if end > len(tokens) {
			end = len(tokens)
		}
		chunks = append(chunks, tokens[i:end])
	}

	tc.log.Info("Split transaction into chunks",
		"total_tokens", len(tokens),
		"chunk_size", MaxTokensPerChunk,
		"num_chunks", len(chunks))

	return chunks
}

// ProcessChunksParallel processes chunks with parallelism control
func (tc *TransactionChunker) ProcessChunksParallel(
	chunks [][]contract.TokenInfo,
	processFunc func([]contract.TokenInfo) error,
) error {
	if len(chunks) == 0 {
		return nil
	}

	// Limit parallelism
	semaphore := make(chan struct{}, MaxParallelChunks)
	errors := make(chan error, len(chunks))
	var wg sync.WaitGroup

	startTime := time.Now()

	for i, chunk := range chunks {
		wg.Add(1)
		go func(chunkIndex int, tokens []contract.TokenInfo) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			chunkStart := time.Now()
			tc.log.Debug("Processing chunk",
				"chunk", chunkIndex+1,
				"total_chunks", len(chunks),
				"tokens", len(tokens))

			err := processFunc(tokens)
			
			tc.log.Debug("Chunk processed",
				"chunk", chunkIndex+1,
				"duration", time.Since(chunkStart),
				"success", err == nil)

			if err != nil {
				errors <- fmt.Errorf("chunk %d failed: %w", chunkIndex, err)
			}
		}(i, chunk)
	}

	// Wait for all chunks
	wg.Wait()
	close(errors)

	// Check for errors
	var firstErr error
	for err := range errors {
		if firstErr == nil {
			firstErr = err
		}
		tc.log.Error("Chunk processing error", "error", err)
	}

	tc.log.Info("All chunks processed",
		"total_duration", time.Since(startTime),
		"success", firstErr == nil)

	return firstErr
}

// OptimizeChunkSize dynamically adjusts chunk size based on performance
func (tc *TransactionChunker) OptimizeChunkSize(tokenCount int) int {
	rm := &ResourceMonitor{}
	_, availableMB := rm.GetMemoryStats()

	// Base chunk size on available resources
	var optimalSize int
	switch {
	case availableMB > 32768: // 32GB+
		optimalSize = 500
	case availableMB > 16384: // 16GB+
		optimalSize = 250
	case availableMB > 8192: // 8GB+
		optimalSize = 150
	default:
		optimalSize = 100
	}

	// Adjust based on token count
	if tokenCount < optimalSize {
		return tokenCount
	}

	// Ensure even distribution
	numChunks := (tokenCount + optimalSize - 1) / optimalSize
	if numChunks > MaxParallelChunks*2 {
		// Too many chunks, increase size
		optimalSize = tokenCount / (MaxParallelChunks * 2)
	}

	tc.log.Debug("Optimized chunk size",
		"token_count", tokenCount,
		"chunk_size", optimalSize,
		"available_memory_mb", availableMB)

	return optimalSize
}

// ChunkResult represents the result of processing a chunk
type ChunkResult struct {
	ChunkIndex int
	Success    bool
	Error      error
	Duration   time.Duration
	TokenCount int
}

// ProcessChunksWithResults processes chunks and returns detailed results
func (tc *TransactionChunker) ProcessChunksWithResults(
	chunks [][]contract.TokenInfo,
	processFunc func([]contract.TokenInfo) error,
) ([]ChunkResult, error) {
	results := make([]ChunkResult, len(chunks))
	resultsChan := make(chan ChunkResult, len(chunks))

	// Process chunks
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxParallelChunks)

	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, tokens []contract.TokenInfo) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			start := time.Now()
			err := processFunc(tokens)
			
			resultsChan <- ChunkResult{
				ChunkIndex: index,
				Success:    err == nil,
				Error:      err,
				Duration:   time.Since(start),
				TokenCount: len(tokens),
			}
		}(i, chunk)
	}

	// Collect results
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var hasError bool
	for result := range resultsChan {
		results[result.ChunkIndex] = result
		if !result.Success {
			hasError = true
		}
	}

	// Log summary
	var totalDuration time.Duration
	var successCount int
	for _, r := range results {
		totalDuration += r.Duration
		if r.Success {
			successCount++
		}
	}

	tc.log.Info("Chunk processing complete",
		"total_chunks", len(chunks),
		"successful", successCount,
		"failed", len(chunks)-successCount,
		"avg_duration", totalDuration/time.Duration(len(chunks)))

	if hasError {
		return results, fmt.Errorf("some chunks failed processing")
	}

	return results, nil
}