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

// TokenStateValidator manages token state validation with resource control
type TokenStateValidator struct {
	core           *Core
	log            logger.Logger
	maxWorkers     int
	batchSize      int
	memoryLimit    uint64 // MB
	gcInterval     time.Duration
}

// NewTokenStateValidator creates a new validator with resource management
func NewTokenStateValidator(core *Core) *TokenStateValidator {
	// Calculate workers based on available memory
	rm := &ResourceMonitor{}
	totalMB, availableMB := rm.GetMemoryStats()
	
	// Balanced approach: 1 worker per 2GB of available memory
	maxWorkers := int(availableMB / 2048)
	if maxWorkers < 3 {
		maxWorkers = 3 // Minimum 3 workers for reasonable performance
	}
	if maxWorkers > 32 {
		maxWorkers = 32 // Cap at 32 workers (64GB RAM)
	}
	
	// Allow 1.5x CPU count for I/O bound operations with higher memory
	cpuCount := runtime.NumCPU()
	maxCPUWorkers := int(float64(cpuCount) * 1.5)
	if maxCPUWorkers < maxWorkers {
		maxWorkers = maxCPUWorkers
	}
	
	return &TokenStateValidator{
		core:        core,
		log:         core.log.Named("TokenStateValidator"),
		maxWorkers:  maxWorkers,
		batchSize:   40, // Process 40 tokens per batch for better throughput
		memoryLimit: totalMB * 95 / 100, // Use max 95% of total memory
		gcInterval:  5 * time.Second,
	}
}

// ValidateTokenStates validates multiple token states with resource management
func (tsv *TokenStateValidator) ValidateTokenStates(
	ti []contract.TokenInfo,
	did string,
	quorumList []string,
) []TokenStateCheckResult {
	total := len(ti)
	tsv.log.Info("Starting token state validation with resource management", 
		"total_tokens", total,
		"max_workers", tsv.maxWorkers,
		"batch_size", tsv.batchSize)
	
	// Pre-allocate result array
	tokenStateCheckResult := make([]TokenStateCheckResult, total)
	
	// Adjust workers based on token count
	workers := tsv.calculateOptimalWorkers(total)
	tsv.log.Debug("Calculated optimal workers", "workers", workers, "tokens", total)
	
	// Create channels for work distribution
	type workItem struct {
		index int
		info  contract.TokenInfo
	}
	
	workChan := make(chan workItem, workers*2)
	var wg sync.WaitGroup
	var completed int32
	var lastLoggedPercent int32
	
	// Start memory monitor
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go tsv.monitorMemoryUsage(monitorCtx)
	
	// Start garbage collector
	gcCtx, gcCancel := context.WithCancel(context.Background())
	defer gcCancel()
	go tsv.periodicGC(gcCtx)
	
	// Start workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			// Process items in batches to reduce memory pressure
			batch := make([]struct {
				index int
				info  contract.TokenInfo
			}, 0, tsv.batchSize)
			
			for item := range workChan {
				batch = append(batch, item)
				
				// Process batch when full or channel is closed
				if len(batch) >= tsv.batchSize || len(workChan) == 0 {
					tsv.processBatch(batch, did, tokenStateCheckResult, quorumList)
					
					// Update progress
					newCount := atomic.AddInt32(&completed, int32(len(batch)))
					tsv.logProgress(newCount, int32(total), &lastLoggedPercent)
					
					// Clear batch
					batch = batch[:0]
					
					// Tiny delay between batches to allow GC
					time.Sleep(5 * time.Millisecond)
				}
			}
			
			// Process any remaining items
			if len(batch) > 0 {
				tsv.processBatch(batch, did, tokenStateCheckResult, quorumList)
				newCount := atomic.AddInt32(&completed, int32(len(batch)))
				tsv.logProgress(newCount, int32(total), &lastLoggedPercent)
			}
		}(w)
	}
	
	// Queue work items
	for i, tokenInfo := range ti {
		workChan <- workItem{index: i, info: tokenInfo}
	}
	close(workChan)
	
	// Wait for completion
	wg.Wait()
	
	tsv.log.Info("Token state validation completed", 
		"total_tokens", total,
		"workers_used", workers)
	
	return tokenStateCheckResult
}

// processBatch processes a batch of tokens
func (tsv *TokenStateValidator) processBatch(
	batch []struct {
		index int
		info  contract.TokenInfo
	},
	did string,
	results []TokenStateCheckResult,
	quorumList []string,
) {
	for _, item := range batch {
		tsv.validateSingleToken(
			item.info.Token,
			did,
			item.index,
			results,
			quorumList,
			item.info.TokenType,
		)
	}
}

// validateSingleToken validates a single token (replaces checkTokenState)
func (tsv *TokenStateValidator) validateSingleToken(
	tokenId, did string,
	index int,
	resultArray []TokenStateCheckResult,
	quorumList []string,
	tokenType int,
) {
	var result TokenStateCheckResult
	result.Token = tokenId
	
	// Get the latest blockId
	block := tsv.core.w.GetLatestTokenBlock(tokenId, tokenType)
	if block == nil {
		result.Error = fmt.Errorf("Invalid token chain block, Block is nil")
		result.Message = "Invalid token chain block"
		resultArray[index] = result
		return
	}
	
	blockId, err := block.GetBlockID(tokenId)
	if err != nil {
		result.Error = err
		result.Message = "Error fetching block Id"
		resultArray[index] = result
		return
	}
	
	// Concat tokenId and BlockID
	tokenIDTokenStateData := tokenId + blockId
	tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
	
	// Add to IPFS (only hash)
	tokenIDTokenStateHash, err := IpfsAddWithBackoff(
		tsv.core.ipfs,
		tokenIDTokenStateBuffer,
		ipfsnode.Pin(false),
		ipfsnode.OnlyHash(true),
	)
	result.tokenIDTokenStateHash = tokenIDTokenStateHash
	if err != nil {
		result.Error = err
		result.Message = "Error adding data to ipfs"
		resultArray[index] = result
		return
	}
	
	// Check if already pinned
	tokenStatePinInfo, err := tsv.core.w.GetStatePinnedInfo(tokenIDTokenStateHash)
	if err != nil {
		result.Error = err
		result.Message = "Error checking if tokenstate pinned earlier"
		resultArray[index] = result
		return
	}
	
	if tokenStatePinInfo != nil {
		tsv.log.Debug("Tokenstate already pinned locally")
		result.Message = "Tokenstate already pinned locally"
		resultArray[index] = result
		return
	}
	
	// Skip DHT check in trusted network mode
	if tsv.core.cfg.CfgData.TrustedNetwork {
		tsv.log.Debug("Skipping DHT check in trusted network mode", "token", tokenId)
	} else {
		// Check DHT to see if any pin exists
		list, err := tsv.core.GetDHTddrs(tokenIDTokenStateHash)
		if err != nil {
			tsv.log.Error("Error fetching content for tokenstate hash", 
				"hash", tokenIDTokenStateHash, 
				"error", err)
			result.Exhausted = true
			result.Message = "Error fetching content for tokenstate hash: " + tokenIDTokenStateHash
			resultArray[index] = result
			return
		}
		
		// Remove quorum peer IDs from list
		qPeerIds := make([]string, 0)
		for i := range quorumList {
			pId, _, ok := util.ParseAddress(quorumList[i])
			if !ok {
				result.Error = fmt.Errorf("error parsing address")
				result.Message = "Error parsing address"
				resultArray[index] = result
				return
			}
			qPeerIds = append(qPeerIds, pId)
		}
		
		// Add sender's peer ID
		peerId := tsv.core.w.GetPeerID(did)
		if peerId != "" {
			qPeerIds = append(qPeerIds, peerId)
		}
		
		// Remove quorum peers from DHT list
		updatedList := tsv.removeStrings(list, qPeerIds)
		tsv.log.Debug("DHT check results", 
			"original_list", len(list),
			"excluded_peers", len(qPeerIds),
			"remaining_peers", len(updatedList))
		
		// If pin exists elsewhere, token is exhausted
		if len(updatedList) > 1 {
			tsv.log.Debug("Token state exhausted", "token", tokenId)
			result.Exhausted = true
			result.Message = "Token state is exhausted, Token being Double spent: " + tokenId
			resultArray[index] = result
			return
		}
	}
	
	// Token state is free
	result.Exhausted = false
	result.Message = "Token state is free, Unique Txn"
	result.tokenIDTokenStateData = tokenIDTokenStateData
	resultArray[index] = result
}

// calculateOptimalWorkers determines worker count based on token count and memory
func (tsv *TokenStateValidator) calculateOptimalWorkers(tokenCount int) int {
	// Use resource monitor to check current memory
	rm := &ResourceMonitor{}
	_, availableMB := rm.GetMemoryStats()
	
	// With 2GB per worker, calculate based on that
	memPerWorker := uint64(2048) // 2GB per worker
	maxWorkersByMemory := int(availableMB / memPerWorker)
	if maxWorkersByMemory < 3 {
		maxWorkersByMemory = 3
	}
	
	// Simply use the memory-based limit up to our max
	// No artificial token-based restrictions
	workers := minInt(maxWorkersByMemory, tsv.maxWorkers)
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
func (tsv *TokenStateValidator) monitorMemoryUsage(ctx context.Context) {
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
			
			if usagePct > 95 {
				criticalCount++
				tsv.log.Error("CRITICAL: Memory usage too high", 
					"usage_pct", usagePct,
					"count", criticalCount)
				
				// Force aggressive GC
				runtime.GC()
				runtime.GC() // Double GC for aggressive cleanup
				
				if criticalCount >= 3 {
					tsv.log.Error("Memory usage critical for too long, may cause OOM")
				}
			} else if usagePct > 85 {
				tsv.log.Warn("High memory usage during token validation", "usage_pct", usagePct)
				runtime.GC()
				criticalCount = 0
			} else {
				criticalCount = 0
			}
		}
	}
}

// periodicGC runs garbage collection periodically
func (tsv *TokenStateValidator) periodicGC(ctx context.Context) {
	ticker := time.NewTicker(tsv.gcInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runtime.GC()
			tsv.log.Debug("Periodic GC completed during token validation")
		}
	}
}

// logProgress logs validation progress
func (tsv *TokenStateValidator) logProgress(completed, total int32, lastLogged *int32) {
	currentPercent := int32(float64(completed*100) / float64(total))
	
	if currentPercent%10 == 0 && atomic.LoadInt32(lastLogged) < currentPercent {
		if atomic.CompareAndSwapInt32(lastLogged, *lastLogged, currentPercent) {
			tsv.log.Info(fmt.Sprintf("Token validation progress: %d%% (%d/%d)", 
				currentPercent, completed, total))
		}
	}
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// removeStrings removes all occurrences of strings in toRemove from original
func (tsv *TokenStateValidator) removeStrings(original []string, toRemove []string) []string {
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