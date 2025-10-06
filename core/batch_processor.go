package core

import (
	"context"
	"sync"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// BatchProcessor manages processing of tokens in optimized batches
type BatchProcessor struct {
	log logger.Logger
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(log logger.Logger) *BatchProcessor {
	return &BatchProcessor{
		log: log.Named("BatchProcessor"),
	}
}

// ProcessTokenBatch represents a batch of tokens to process
type ProcessTokenBatch struct {
	StartIdx int
	EndIdx   int
	Tokens   []interface{} // Generic to handle different token types
}

// CalculateOptimalBatches divides tokens into memory-efficient batches
func (bp *BatchProcessor) CalculateOptimalBatches(totalTokens int, maxWorkers int) []ProcessTokenBatch {
	// Determine batch size based on token count and workers
	var batchSize int
	
	switch {
	case totalTokens <= 50:
		batchSize = 10 // Small batches for quick processing
	case totalTokens <= 100:
		batchSize = 20 // Medium batches
	case totalTokens <= 500:
		batchSize = 50 // Larger batches to reduce overhead
	default:
		batchSize = 100 // Maximum batch size for very large sets
	}
	
	// Adjust batch size based on worker count
	// Ensure each worker gets meaningful work
	minBatchesPerWorker := 2
	idealBatchCount := maxWorkers * minBatchesPerWorker
	
	if totalTokens/batchSize < idealBatchCount {
		// Reduce batch size to create more batches
		batchSize = max(1, totalTokens/idealBatchCount)
	}
	
	// Create batches
	batches := make([]ProcessTokenBatch, 0)
	for i := 0; i < totalTokens; i += batchSize {
		endIdx := min(i+batchSize, totalTokens)
		batches = append(batches, ProcessTokenBatch{
			StartIdx: i,
			EndIdx:   endIdx,
		})
	}
	
	bp.log.Debug("Calculated token batches", 
		"total_tokens", totalTokens,
		"batch_size", batchSize,
		"num_batches", len(batches),
		"workers", maxWorkers)
	
	return batches
}

// ProcessInBatches executes work in memory-efficient batches with controlled concurrency
func (bp *BatchProcessor) ProcessInBatches(
	ctx context.Context,
	batches []ProcessTokenBatch,
	maxWorkers int,
	processFn func(batch ProcessTokenBatch) error,
) error {
	// Create worker pool
	batchChan := make(chan ProcessTokenBatch, len(batches))
	errChan := make(chan error, 1)
	var wg sync.WaitGroup
	
	// Start workers
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for batch := range batchChan {
				select {
				case <-ctx.Done():
					return
				default:
					// Process batch with memory cleanup between batches
					startTime := time.Now()
					
					err := processFn(batch)
					
					processingTime := time.Since(startTime)
					bp.log.Debug("Worker processed batch", 
						"worker_id", workerID,
						"batch_start", batch.StartIdx,
						"batch_end", batch.EndIdx,
						"duration", processingTime)
					
					if err != nil {
						select {
						case errChan <- err:
						default:
						}
						return
					}
					
					// Brief pause between batches to allow GC
					if processingTime < 100*time.Millisecond {
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
		}(w)
	}
	
	// Queue all batches
	for _, batch := range batches {
		select {
		case batchChan <- batch:
		case <-ctx.Done():
			close(batchChan)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(batchChan)
	
	// Wait for completion
	wg.Wait()
	
	// Check for errors
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

// EstimateMemoryPerBatch estimates memory usage for a batch
func (bp *BatchProcessor) EstimateMemoryPerBatch(batchSize int) uint64 {
	// Rough estimates based on observed behavior
	baseMemoryMB := uint64(50)  // Base overhead per batch
	perTokenMB := uint64(10)     // Memory per token in batch
	
	return baseMemoryMB + (perTokenMB * uint64(batchSize))
}