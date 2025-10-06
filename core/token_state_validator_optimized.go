package core

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// workItem represents a token to be validated
type workItem struct {
	index int
	info  contract.TokenInfo
}

// TokenStateValidatorOptimized manages token state validation with caching and optimization
type TokenStateValidatorOptimized struct {
	core           *Core
	log            logger.Logger
	maxWorkers     int
	batchSize      int
	memoryLimit    uint64 // MB
	gcInterval     time.Duration
	
	// Caches to avoid repetitive operations
	quorumPeerCache   []string          // Pre-parsed quorum peer IDs
	senderPeerID      string            // Cached sender peer ID
	blockCache        sync.Map          // Cache for token blocks
	hashCache         sync.Map          // Cache for IPFS hashes
	localPinCache     sync.Map          // Cache for local pin checks
	cacheMu           sync.RWMutex      // Protect cache initialization
}

// NewTokenStateValidatorOptimized creates an optimized validator
func NewTokenStateValidatorOptimized(core *Core, did string, quorumList []string) *TokenStateValidatorOptimized {
	// Calculate workers based on available memory
	rm := &ResourceMonitor{}
	totalMB, availableMB := rm.GetMemoryStats()
	
	// Balanced approach: 1 worker per 2GB of available memory
	maxWorkers := int(availableMB / 2048)
	if maxWorkers < 3 {
		maxWorkers = 3 // Minimum 3 workers for reasonable performance
	}
	if maxWorkers > 32 {
		maxWorkers = 32 // Cap at 32 workers
	}
	
	// Allow 1.5x CPU count for I/O bound operations with higher memory
	cpuCount := runtime.NumCPU()
	maxCPUWorkers := int(float64(cpuCount) * 1.5)
	if maxCPUWorkers < maxWorkers {
		maxWorkers = maxCPUWorkers
	}
	
	// Larger batch sizing for 2GB workers
	batchSize := 100 // Default batch size for 2GB workers
	if len(quorumList) > 5 { // Larger transactions typically have more quorums
		batchSize = 150 // Large batch size for 2GB workers
	}
	
	tsv := &TokenStateValidatorOptimized{
		core:        core,
		log:         core.log.Named("TokenStateValidatorOpt"),
		maxWorkers:  maxWorkers,
		batchSize:   batchSize,
		memoryLimit: totalMB * 95 / 100, // Use up to 95% of total memory
		gcInterval:  5 * time.Second,
	}
	
	// Pre-cache quorum peer IDs (expensive operation done once)
	tsv.initializeQuorumPeerCache(quorumList)
	
	// Pre-cache sender peer ID
	tsv.senderPeerID = core.w.GetPeerID(did)
	
	return tsv
}

// initializeQuorumPeerCache pre-parses all quorum addresses
func (tsv *TokenStateValidatorOptimized) initializeQuorumPeerCache(quorumList []string) {
	tsv.cacheMu.Lock()
	defer tsv.cacheMu.Unlock()
	
	tsv.quorumPeerCache = make([]string, 0, len(quorumList)+1)
	
	// Parse quorum addresses once
	for _, addr := range quorumList {
		pId, _, ok := util.ParseAddress(addr)
		if ok {
			tsv.quorumPeerCache = append(tsv.quorumPeerCache, pId)
		}
	}
	
	tsv.log.Debug("Pre-cached quorum peer IDs", "count", len(tsv.quorumPeerCache))
}

// ValidateTokenStatesOptimized validates tokens with extensive caching
func (tsv *TokenStateValidatorOptimized) ValidateTokenStatesOptimized(
	ti []contract.TokenInfo,
	did string,
) []TokenStateCheckResult {
	total := len(ti)
	
	// Optimize batch size based on worker count and token count
	workers := tsv.calculateOptimalWorkers(total)
	optimalBatchSize := total / workers
	if optimalBatchSize < 10 {
		optimalBatchSize = 10 // Minimum batch size
	}
	if optimalBatchSize > 100 {
		optimalBatchSize = 100 // Maximum batch size to prevent memory issues
	}
	
	// Override default batch size with optimal
	if total > 500 {
		tsv.batchSize = minInt(75, optimalBatchSize)
	} else if total > 250 {
		tsv.batchSize = minInt(50, optimalBatchSize)
	} else {
		tsv.batchSize = optimalBatchSize
	}
	
	// Already at 95%, no need to increase further
	if total > 2000 {
		tsv.log.Warn("Very large transaction, ensure sufficient memory", "tokens", total)
	}
	
	tsv.log.Info("Starting optimized token validation", 
		"total_tokens", total,
		"max_workers", tsv.maxWorkers,
		"batch_size", tsv.batchSize)
	
	// Pre-allocate result array
	tokenStateCheckResult := make([]TokenStateCheckResult, total)
	
	// Group tokens by type for better cache hit rate
	tokensByType := tsv.groupTokensByType(ti)
	
	// Workers already calculated above for batch size optimization
	tsv.log.Debug("Using optimized workers", "workers", workers, "tokens", total, "batch_size", tsv.batchSize)
	
	// Create channels for work distribution
	workChan := make(chan workItem, workers*2)
	var wg sync.WaitGroup
	var completed int32
	var cacheHits int32
	var lastLoggedPercent int32
	
	// Start memory monitor
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go tsv.monitorMemoryUsage(monitorCtx)
	
	// Start workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			// Process items in batches
			batch := make([]workItem, 0, tsv.batchSize)
			
			for item := range workChan {
				batch = append(batch, item)
				
				// Process batch when full or channel is empty
				if len(batch) >= tsv.batchSize || len(workChan) == 0 {
					batchStart := time.Now()
					hits := tsv.processBatchOptimized(batch, did, tokenStateCheckResult)
					atomic.AddInt32(&cacheHits, int32(hits))
					
					// Update progress
					newCount := atomic.AddInt32(&completed, int32(len(batch)))
					tsv.logProgress(newCount, int32(total), &lastLoggedPercent)
					
					// Log slow batches
					batchDuration := time.Since(batchStart)
					if batchDuration > 5*time.Second {
						tsv.log.Warn("Slow batch processing", 
							"worker", workerID,
							"batch_size", len(batch),
							"duration", batchDuration,
							"completed", newCount,
							"total", total)
					}
					
					// Clear batch
					batch = batch[:0]
				}
			}
			
			// Process any remaining items
			if len(batch) > 0 {
				hits := tsv.processBatchOptimized(batch, did, tokenStateCheckResult)
				atomic.AddInt32(&cacheHits, int32(hits))
				newCount := atomic.AddInt32(&completed, int32(len(batch)))
				tsv.logProgress(newCount, int32(total), &lastLoggedPercent)
			}
		}(w)
	}
	
	// Queue work items by type for better cache locality
	for tokenType, tokens := range tokensByType {
		tsv.log.Debug("Processing token type batch", "type", tokenType, "count", len(tokens))
		for _, item := range tokens {
			workChan <- item
		}
	}
	close(workChan)
	
	// Wait for completion
	wg.Wait()
	
	hitRate := float64(cacheHits) / float64(total) * 100
	tsv.log.Info("Optimized validation completed", 
		"total_tokens", total,
		"workers_used", workers,
		"cache_hit_rate", fmt.Sprintf("%.1f%%", hitRate))
	
	return tokenStateCheckResult
}

// groupTokensByType groups tokens by type for better cache efficiency
func (tsv *TokenStateValidatorOptimized) groupTokensByType(ti []contract.TokenInfo) map[int][]workItem {
	groups := make(map[int][]workItem)
	
	for i, tokenInfo := range ti {
		item := workItem{index: i, info: tokenInfo}
		groups[tokenInfo.TokenType] = append(groups[tokenInfo.TokenType], item)
	}
	
	return groups
}

// processBatchOptimized processes a batch with caching
func (tsv *TokenStateValidatorOptimized) processBatchOptimized(
	batch []workItem,
	did string,
	results []TokenStateCheckResult,
) int {
	cacheHits := 0
	
	// Pre-fetch blocks for the batch to improve cache efficiency
	tsv.prefetchBlocks(batch)
	
	for _, item := range batch {
		hits := tsv.validateSingleTokenOptimized(
			item.info.Token,
			did,
			item.index,
			results,
			item.info.TokenType,
		)
		cacheHits += hits
	}
	
	return cacheHits
}

// prefetchBlocks pre-loads blocks for a batch
func (tsv *TokenStateValidatorOptimized) prefetchBlocks(batch []workItem) {
	// Group by token type for efficient batch fetching
	byType := make(map[int][]string)
	for _, item := range batch {
		byType[item.info.TokenType] = append(byType[item.info.TokenType], item.info.Token)
	}
	
	// Prefetch blocks by type (implementation depends on wallet interface)
	// This is a placeholder for potential batch block fetching
}

// validateSingleTokenOptimized validates with caching
func (tsv *TokenStateValidatorOptimized) validateSingleTokenOptimized(
	tokenId, did string,
	index int,
	resultArray []TokenStateCheckResult,
	tokenType int,
) int {
	var result TokenStateCheckResult
	var cacheHits int
	result.Token = tokenId
	
	// Check block cache first
	blockKey := fmt.Sprintf("%s:%d", tokenId, tokenType)
	var blockId string
	
	if cachedBlock, ok := tsv.blockCache.Load(blockKey); ok {
		blockId = cachedBlock.(string)
		cacheHits++
	} else {
		// Get the latest blockId
		block := tsv.core.w.GetLatestTokenBlock(tokenId, tokenType)
		if block == nil {
			result.Error = fmt.Errorf("Invalid token chain block, Block is nil")
			result.Message = "Invalid token chain block"
			resultArray[index] = result
			return cacheHits
		}
		
		var err error
		blockId, err = block.GetBlockID(tokenId)
		if err != nil {
			result.Error = err
			result.Message = "Error fetching block Id"
			resultArray[index] = result
			return cacheHits
		}
		
		// Cache the block ID
		tsv.blockCache.Store(blockKey, blockId)
	}
	
	// Check hash cache
	tokenIDTokenStateData := tokenId + blockId
	hashKey := tokenIDTokenStateData
	var tokenIDTokenStateHash string
	
	if cachedHash, ok := tsv.hashCache.Load(hashKey); ok {
		tokenIDTokenStateHash = cachedHash.(string)
		cacheHits++
	} else {
		// Calculate hash
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
		
		var err error
		tokenIDTokenStateHash, err = IpfsAddWithBackoff(
			tsv.core.ipfs,
			tokenIDTokenStateBuffer,
			ipfsnode.Pin(false),
			ipfsnode.OnlyHash(true),
		)
		if err != nil {
			result.Error = err
			result.Message = "Error adding data to ipfs"
			resultArray[index] = result
			return cacheHits
		}
		
		// Cache the hash
		tsv.hashCache.Store(hashKey, tokenIDTokenStateHash)
	}
	
	result.tokenIDTokenStateHash = tokenIDTokenStateHash
	
	// Check local pin cache
	if cachedPin, ok := tsv.localPinCache.Load(tokenIDTokenStateHash); ok {
		if cachedPin.(bool) {
			result.Message = "Tokenstate already pinned locally (cached)"
			resultArray[index] = result
			return cacheHits + 1
		}
		cacheHits++
	} else {
		// Check if already pinned
		tokenStatePinInfo, err := tsv.core.w.GetStatePinnedInfo(tokenIDTokenStateHash)
		if err != nil {
			result.Error = err
			result.Message = "Error checking if tokenstate pinned earlier"
			resultArray[index] = result
			return cacheHits
		}
		
		isPinned := tokenStatePinInfo != nil
		tsv.localPinCache.Store(tokenIDTokenStateHash, isPinned)
		
		if isPinned {
			result.Message = "Tokenstate already pinned locally"
			resultArray[index] = result
			return cacheHits
		}
	}
	
	// Skip DHT check in trusted network mode
	if tsv.core.cfg.CfgData.TrustedNetwork {
		tsv.log.Debug("Skipping DHT check in trusted network mode", "token", tokenId)
	} else {
		// Check DHT (this cannot be easily cached as it changes)
		list, err := tsv.core.GetDHTddrs(tokenIDTokenStateHash)
		if err != nil {
			tsv.log.Error("Error fetching content for tokenstate hash", 
				"hash", tokenIDTokenStateHash, 
				"error", err)
			result.Exhausted = true
			result.Message = "Error fetching content for tokenstate hash: " + tokenIDTokenStateHash
			resultArray[index] = result
			return cacheHits
		}
		
		// Use pre-cached quorum peers
		qPeerIds := tsv.getQuorumPeers()
		
		// Remove quorum peers from DHT list
		updatedList := tsv.removeStrings(list, qPeerIds)
		
		// If pin exists elsewhere, token is exhausted
		if len(updatedList) > 1 {
			tsv.log.Debug("Token state exhausted", "token", tokenId)
			result.Exhausted = true
			result.Message = "Token state is exhausted, Token being Double spent: " + tokenId
			resultArray[index] = result
			return cacheHits
		}
	}
	
	// Token state is free
	result.Exhausted = false
	result.Message = "Token state is free, Unique Txn"
	result.tokenIDTokenStateData = tokenIDTokenStateData
	resultArray[index] = result
	
	return cacheHits
}

// getQuorumPeers returns cached quorum peers including sender
func (tsv *TokenStateValidatorOptimized) getQuorumPeers() []string {
	tsv.cacheMu.RLock()
	defer tsv.cacheMu.RUnlock()
	
	// Create a copy to avoid modifying the cache
	peers := make([]string, len(tsv.quorumPeerCache))
	copy(peers, tsv.quorumPeerCache)
	
	// Add sender peer ID if available
	if tsv.senderPeerID != "" {
		peers = append(peers, tsv.senderPeerID)
	}
	
	return peers
}

// calculateOptimalWorkers determines worker count
func (tsv *TokenStateValidatorOptimized) calculateOptimalWorkers(tokenCount int) int {
	// Use resource monitor to check current memory
	rm := &ResourceMonitor{}
	_, availableMB := rm.GetMemoryStats()
	
	// Balanced memory estimation: 2GB per worker with larger batches
	memPerWorker := uint64(2048) // 2GB per worker
	maxWorkersByMemory := int(availableMB / memPerWorker)
	if maxWorkersByMemory < 3 {
		maxWorkersByMemory = 3
	}
	
	// Calculate workers based on tokens to avoid over-parallelization
	// Rule: At least 10 tokens per worker for efficiency
	tokenBasedWorkers := tokenCount / 10
	if tokenBasedWorkers < 1 {
		tokenBasedWorkers = 1
	}
	
	// For small token counts, still use reasonable workers
	if tokenCount <= 50 {
		tokenBasedWorkers = minInt(5, maxWorkersByMemory) // At least 5 for small
	} else if tokenCount <= 100 {
		tokenBasedWorkers = minInt(10, maxWorkersByMemory) // At least 10 for 100
	} else if tokenCount <= 200 {
		tokenBasedWorkers = minInt(15, maxWorkersByMemory) // At least 15 for 150-200
	}
	
	// Use the minimum of memory-based, token-based, and max workers
	workers := minInt(minInt(maxWorkersByMemory, tokenBasedWorkers), tsv.maxWorkers)
	if workers < 1 {
		workers = 1
	}
	
	tsv.log.Info("Worker allocation", 
		"available_memory_mb", availableMB,
		"workers_by_memory", maxWorkersByMemory,
		"max_workers_cap", tsv.maxWorkers,
		"allocated_workers", workers,
		"tokens", tokenCount)
	
	return workers
}

// monitorMemoryUsage monitors memory during validation
func (tsv *TokenStateValidatorOptimized) monitorMemoryUsage(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	rm := &ResourceMonitor{}
	criticalCount := 0
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := rm.GetResourceStats()
			usagePct := stats["memory_usage_pct"].(float64)
			
			if usagePct > 98 {
				criticalCount++
				tsv.log.Error("CRITICAL: Memory usage too high", 
					"usage_pct", usagePct,
					"count", criticalCount)
				
				// Force aggressive GC
				runtime.GC()
				runtime.GC()
				
				// Clear caches if memory critical
				if criticalCount >= 2 {
					tsv.clearCaches()
				}
			} else if usagePct > 95 {
				tsv.log.Warn("High memory usage during token validation", "usage_pct", usagePct)
				runtime.GC()
				criticalCount = 0
			} else {
				criticalCount = 0
			}
		}
	}
}

// clearCaches clears all caches to free memory
func (tsv *TokenStateValidatorOptimized) clearCaches() {
	tsv.log.Warn("Clearing caches due to memory pressure")
	
	// Clear all caches
	tsv.blockCache = sync.Map{}
	tsv.hashCache = sync.Map{}
	tsv.localPinCache = sync.Map{}
	
	// Force GC after clearing
	runtime.GC()
}

// logProgress logs validation progress
func (tsv *TokenStateValidatorOptimized) logProgress(completed, total int32, lastLogged *int32) {
	currentPercent := int32(float64(completed*100) / float64(total))
	
	if currentPercent%10 == 0 && atomic.LoadInt32(lastLogged) < currentPercent {
		if atomic.CompareAndSwapInt32(lastLogged, *lastLogged, currentPercent) {
			tsv.log.Info(fmt.Sprintf("Token validation progress: %d%% (%d/%d)", 
				currentPercent, completed, total))
		}
	}
}

// removeStrings removes all occurrences of strings in toRemove from original
func (tsv *TokenStateValidatorOptimized) removeStrings(original []string, toRemove []string) []string {
	// Create a map for O(1) lookup
	removeMap := make(map[string]bool)
	for _, s := range toRemove {
		removeMap[s] = true
	}
	
	// Filter original list
	result := make([]string, 0, len(original))
	for _, s := range original {
		if !removeMap[s] {
			result = append(result, s)
		}
	}
	
	return result
}