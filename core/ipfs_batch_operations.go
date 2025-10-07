package core

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	ipfsapi "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// IPFSBatchManager handles batch operations for IPFS
type IPFSBatchManager struct {
	shell    *ipfsapi.Shell
	log      logger.Logger
	maxBatch int
	timeout  time.Duration
	mu       sync.Mutex
}

// NewIPFSBatchManager creates a new batch manager
func NewIPFSBatchManager(shell *ipfsapi.Shell, log logger.Logger) *IPFSBatchManager {
	return &IPFSBatchManager{
		shell:    shell,
		log:      log.Named("IPFSBatch"),
		maxBatch: 50, // Process 50 items at once
		timeout:  30 * time.Second,
	}
}

// BatchAdd adds multiple items to IPFS in a single operation
func (bm *IPFSBatchManager) BatchAdd(items [][]byte, pin bool) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	bm.log.Debug("Starting batch add", "items", len(items), "pin", pin)
	start := time.Now()

	// Split into manageable batches
	var allHashes []string
	for i := 0; i < len(items); i += bm.maxBatch {
		end := i + bm.maxBatch
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		hashes, err := bm.processBatchAdd(batch, pin)
		if err != nil {
			return allHashes, fmt.Errorf("batch %d failed: %w", i/bm.maxBatch, err)
		}

		allHashes = append(allHashes, hashes...)
		
		// Small delay between batches to prevent overwhelming IPFS
		if end < len(items) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	bm.log.Info("Batch add complete",
		"total_items", len(items),
		"duration", time.Since(start),
		"hashes", len(allHashes))

	return allHashes, nil
}

// processBatchAdd handles a single batch
func (bm *IPFSBatchManager) processBatchAdd(items [][]byte, pin bool) ([]string, error) {
	// Use goroutines for parallel adds within batch
	results := make(chan struct {
		index int
		hash  string
		err   error
	}, len(items))

	ctx, cancel := context.WithTimeout(context.Background(), bm.timeout)
	defer cancel()

	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, data []byte) {
			defer wg.Done()

			hash, err := bm.addSingleItem(ctx, data, pin)
			results <- struct {
				index int
				hash  string
				err   error
			}{idx, hash, err}
		}(i, item)
	}

	// Close results channel when done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	hashes := make([]string, len(items))
	var firstErr error
	
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
		hashes[result.index] = result.hash
	}

	if firstErr != nil {
		return hashes, firstErr
	}

	return hashes, nil
}

// addSingleItem adds one item with retry logic
func (bm *IPFSBatchManager) addSingleItem(ctx context.Context, data []byte, pin bool) (string, error) {
	reader := bytes.NewReader(data)
	
	// Retry logic
	var hash string
	var err error
	
	for attempts := 0; attempts < 3; attempts++ {
		if attempts > 0 {
			time.Sleep(time.Duration(attempts) * time.Second)
		}

		hash, err = bm.shell.Add(reader, ipfsapi.Pin(pin))
		if err == nil {
			return hash, nil
		}

		// Check if context expired
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}

	return "", fmt.Errorf("failed after 3 attempts: %w", err)
}

// BatchPin pins multiple hashes efficiently
func (bm *IPFSBatchManager) BatchPin(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}

	bm.log.Debug("Starting batch pin", "hashes", len(hashes))
	start := time.Now()

	// Process in batches
	for i := 0; i < len(hashes); i += bm.maxBatch {
		end := i + bm.maxBatch
		if end > len(hashes) {
			end = len(hashes)
		}

		batch := hashes[i:end]
		if err := bm.processBatchPin(batch); err != nil {
			return fmt.Errorf("batch pin failed at %d: %w", i, err)
		}

		// Progress update
		bm.log.Debug("Batch pin progress",
			"completed", end,
			"total", len(hashes),
			"percent", (end*100)/len(hashes))
	}

	bm.log.Info("Batch pin complete",
		"total_hashes", len(hashes),
		"duration", time.Since(start))

	return nil
}

// processBatchPin handles pinning a batch
func (bm *IPFSBatchManager) processBatchPin(hashes []string) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(hashes))

	ctx, cancel := context.WithTimeout(context.Background(), bm.timeout)
	defer cancel()

	// Limit concurrent pins to prevent overwhelming IPFS
	semaphore := make(chan struct{}, 10)

	for _, hash := range hashes {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := bm.pinWithRetry(ctx, h); err != nil {
				errors <- fmt.Errorf("pin %s failed: %w", h, err)
			}
		}(hash)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var firstErr error
	for err := range errors {
		if firstErr == nil {
			firstErr = err
		}
		bm.log.Error("Pin error", "error", err)
	}

	return firstErr
}

// pinWithRetry pins a hash with retry logic
func (bm *IPFSBatchManager) pinWithRetry(ctx context.Context, hash string) error {
	for attempts := 0; attempts < 3; attempts++ {
		if attempts > 0 {
			time.Sleep(time.Duration(attempts) * time.Second)
		}

		err := bm.shell.Pin(hash)
		if err == nil {
			return nil
		}

		// Check if context expired
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return fmt.Errorf("failed to pin after 3 attempts")
}

// BatchGet retrieves multiple items from IPFS
func (bm *IPFSBatchManager) BatchGet(hashes []string) (map[string][]byte, error) {
	if len(hashes) == 0 {
		return make(map[string][]byte), nil
	}

	bm.log.Debug("Starting batch get", "hashes", len(hashes))
	start := time.Now()

	results := make(map[string][]byte)
	resultsMu := sync.Mutex{}

	// Process in parallel with concurrency limit
	semaphore := make(chan struct{}, 20)
	var wg sync.WaitGroup
	errors := make(chan error, len(hashes))

	for _, hash := range hashes {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			data, err := bm.getWithRetry(h)
			if err != nil {
				errors <- fmt.Errorf("get %s failed: %w", h, err)
				return
			}

			resultsMu.Lock()
			results[h] = data
			resultsMu.Unlock()
		}(hash)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var firstErr error
	for err := range errors {
		if firstErr == nil {
			firstErr = err
		}
	}

	bm.log.Info("Batch get complete",
		"requested", len(hashes),
		"retrieved", len(results),
		"duration", time.Since(start))

	return results, firstErr
}

// getWithRetry retrieves data with retry logic
func (bm *IPFSBatchManager) getWithRetry(hash string) ([]byte, error) {
	for attempts := 0; attempts < 3; attempts++ {
		if attempts > 0 {
			time.Sleep(time.Duration(attempts) * time.Second)
		}

		reader, err := bm.shell.Cat(hash)
		if err != nil {
			continue
		}

		var buf bytes.Buffer
		_, err = buf.ReadFrom(reader)
		reader.Close()

		if err == nil {
			return buf.Bytes(), nil
		}
	}

	return nil, fmt.Errorf("failed to get after 3 attempts")
}

// OptimizeBatchSize determines optimal batch size based on performance
func (bm *IPFSBatchManager) OptimizeBatchSize(itemCount int, avgItemSize int) int {
	// Base calculation on total data size
	totalSizeMB := (itemCount * avgItemSize) / (1024 * 1024)
	
	var optimalBatch int
	switch {
	case totalSizeMB < 10:
		optimalBatch = 100 // Small data, larger batches
	case totalSizeMB < 100:
		optimalBatch = 50  // Medium data
	case totalSizeMB < 1000:
		optimalBatch = 25  // Large data
	default:
		optimalBatch = 10  // Very large data, smaller batches
	}

	// Adjust based on item count
	if optimalBatch > itemCount {
		optimalBatch = itemCount
	}

	bm.log.Debug("Optimized batch size",
		"item_count", itemCount,
		"avg_size", avgItemSize,
		"total_mb", totalSizeMB,
		"batch_size", optimalBatch)

	return optimalBatch
}