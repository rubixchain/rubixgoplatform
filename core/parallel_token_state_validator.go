package core

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// ParallelTokenStateValidator performs token state checks with maximum parallelism
type ParallelTokenStateValidator struct {
	core            *Core
	log             logger.Logger
	maxConcurrent   int
	memoryLimit     uint64
	
	// Batch processors for different phases
	blockFetcher    *BatchBlockFetcher
	hashComputer    *BatchHashComputer
	pinChecker      *BatchPinChecker
	dhtChecker      *BatchDHTChecker
	
	// Caches to avoid repetitive operations
	quorumPeerCache []string
	senderPeerID    string
	cacheMu         sync.RWMutex
}

// BatchBlockFetcher handles parallel block fetching
type BatchBlockFetcher struct {
	core       *Core
	log        logger.Logger
	maxWorkers int
}

// BatchHashComputer handles parallel hash computation
type BatchHashComputer struct {
	core       *Core
	log        logger.Logger
	maxWorkers int
}

// BatchPinChecker handles parallel pin checking
type BatchPinChecker struct {
	core       *Core
	log        logger.Logger
	maxWorkers int
}

// BatchDHTChecker handles parallel DHT lookups
type BatchDHTChecker struct {
	core       *Core
	log        logger.Logger
	maxWorkers int
}

// TokenValidationInput holds input data for validation
type TokenValidationInput struct {
	Index     int
	TokenInfo contract.TokenInfo
}

// TokenValidationResult holds intermediate validation results
type TokenValidationResult struct {
	Index                 int
	Token                 string
	BlockID               string
	TokenStateData        string
	TokenStateHash        string
	IsPinnedLocally       bool
	DHTProviders          []string
	Error                 error
}

// NewParallelTokenStateValidator creates an optimized parallel validator
func NewParallelTokenStateValidator(core *Core, did string, quorumList []string) *ParallelTokenStateValidator {
	// Calculate max concurrent operations based on resources
	rm := &ResourceMonitor{}
	totalMB, availableMB := rm.GetMemoryStats()
	
	// Allow more concurrent operations with parallel design
	maxConcurrent := int(availableMB / 1024) // 1GB per concurrent batch
	if maxConcurrent < 10 {
		maxConcurrent = 10 // Minimum 10 concurrent operations
	}
	if maxConcurrent > 100 {
		maxConcurrent = 100 // Cap at 100
	}
	
	// CPU-based limit
	cpuCount := runtime.NumCPU()
	maxCPUConcurrent := cpuCount * 4 // 4x CPU for I/O bound operations
	if maxCPUConcurrent < maxConcurrent {
		maxConcurrent = maxCPUConcurrent
	}
	
	ptsv := &ParallelTokenStateValidator{
		core:          core,
		log:           core.log.Named("ParallelTokenStateValidator"),
		maxConcurrent: maxConcurrent,
		memoryLimit:   totalMB * 95 / 100,
		
		// Initialize batch processors
		blockFetcher: &BatchBlockFetcher{
			core:       core,
			log:        core.log.Named("BatchBlockFetcher"),
			maxWorkers: maxConcurrent / 4, // Distribute workers across phases
		},
		hashComputer: &BatchHashComputer{
			core:       core,
			log:        core.log.Named("BatchHashComputer"),
			maxWorkers: maxConcurrent / 4,
		},
		pinChecker: &BatchPinChecker{
			core:       core,
			log:        core.log.Named("BatchPinChecker"),
			maxWorkers: maxConcurrent / 4,
		},
		dhtChecker: &BatchDHTChecker{
			core:       core,
			log:        core.log.Named("BatchDHTChecker"),
			maxWorkers: maxConcurrent / 4,
		},
	}
	
	// Pre-cache quorum peer IDs
	ptsv.initializeQuorumPeerCache(quorumList)
	ptsv.senderPeerID = core.w.GetPeerID(did)
	
	ptsv.log.Info("Initialized parallel token state validator",
		"max_concurrent", maxConcurrent,
		"available_memory_mb", availableMB,
		"cpu_count", cpuCount)
	
	return ptsv
}

// ValidateTokenStates performs parallel token state validation
func (ptsv *ParallelTokenStateValidator) ValidateTokenStates(
	tokens []contract.TokenInfo,
	did string,
) []TokenStateCheckResult {
	startTime := time.Now()
	total := len(tokens)
	
	ptsv.log.Info("Starting parallel token state validation",
		"total_tokens", total,
		"max_concurrent", ptsv.maxConcurrent)
	
	// Pre-allocate results
	results := make([]TokenStateCheckResult, total)
	
	// Create input channel
	inputs := make([]TokenValidationInput, total)
	for i, tokenInfo := range tokens {
		inputs[i] = TokenValidationInput{
			Index:     i,
			TokenInfo: tokenInfo,
		}
	}
	
	// Phase 1: Batch fetch all blocks in parallel
	blockResults := ptsv.batchFetchBlocks(inputs)
	
	// Phase 2: Compute hashes in parallel
	hashResults := ptsv.computeHashesParallel(blockResults)
	
	// Phase 3: Check local pins in parallel
	pinResults := ptsv.batchCheckPins(hashResults)
	
	// Phase 4: DHT checks for unpinned tokens (if not trusted network)
	var dhtResults []TokenValidationResult
	if !ptsv.core.cfg.CfgData.TrustedNetwork {
		dhtResults = ptsv.batchDHTChecks(pinResults)
	} else {
		dhtResults = pinResults
	}
	
	// Phase 5: Assemble final results
	ptsv.assembleResults(results, dhtResults)
	
	duration := time.Since(startTime)
	tokensPerSecond := float64(total) / duration.Seconds()
	
	ptsv.log.Info("Parallel token validation completed",
		"total_tokens", total,
		"duration", duration,
		"tokens_per_second", fmt.Sprintf("%.2f", tokensPerSecond))
	
	return results
}

// Phase 1: Batch fetch blocks
func (ptsv *ParallelTokenStateValidator) batchFetchBlocks(inputs []TokenValidationInput) []TokenValidationResult {
	results := make([]TokenValidationResult, len(inputs))
	resultsChan := make(chan TokenValidationResult, len(inputs))
	
	// Use worker pool for block fetching
	workChan := make(chan TokenValidationInput, ptsv.blockFetcher.maxWorkers)
	var wg sync.WaitGroup
	
	// Start workers
	for w := 0; w < ptsv.blockFetcher.maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for input := range workChan {
				result := TokenValidationResult{
					Index: input.Index,
					Token: input.TokenInfo.Token,
				}
				
				// Fetch block
				block := ptsv.core.w.GetLatestTokenBlock(input.TokenInfo.Token, input.TokenInfo.TokenType)
				if block == nil {
					result.Error = fmt.Errorf("invalid token chain block, block is nil")
				} else {
					blockID, err := block.GetBlockID(input.TokenInfo.Token)
					if err != nil {
						result.Error = fmt.Errorf("error fetching block ID: %w", err)
					} else {
						result.BlockID = blockID
						result.TokenStateData = input.TokenInfo.Token + blockID
					}
				}
				
				resultsChan <- result
			}
		}()
	}
	
	// Queue work
	go func() {
		for _, input := range inputs {
			workChan <- input
		}
		close(workChan)
	}()
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect results
	for result := range resultsChan {
		results[result.Index] = result
	}
	
	return results
}

// Phase 2: Compute hashes in parallel
func (ptsv *ParallelTokenStateValidator) computeHashesParallel(inputs []TokenValidationResult) []TokenValidationResult {
	results := make([]TokenValidationResult, len(inputs))
	resultsChan := make(chan TokenValidationResult, len(inputs))
	
	// Use worker pool for hash computation
	workChan := make(chan TokenValidationResult, ptsv.hashComputer.maxWorkers)
	var wg sync.WaitGroup
	
	// Start workers
	for w := 0; w < ptsv.hashComputer.maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for input := range workChan {
				result := input // Copy input
				
				if result.Error == nil && result.TokenStateData != "" {
					// Compute hash
					buffer := bytes.NewBuffer([]byte(result.TokenStateData))
					hash, err := IpfsAddWithBackoff(
						ptsv.core.ipfs,
						buffer,
						ipfsnode.Pin(false),
						ipfsnode.OnlyHash(true),
					)
					if err != nil {
						result.Error = fmt.Errorf("error computing hash: %w", err)
					} else {
						result.TokenStateHash = hash
					}
				}
				
				resultsChan <- result
			}
		}()
	}
	
	// Queue work
	go func() {
		for _, input := range inputs {
			workChan <- input
		}
		close(workChan)
	}()
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect results
	for result := range resultsChan {
		results[result.Index] = result
	}
	
	return results
}

// Phase 3: Check local pins in parallel
func (ptsv *ParallelTokenStateValidator) batchCheckPins(inputs []TokenValidationResult) []TokenValidationResult {
	results := make([]TokenValidationResult, len(inputs))
	resultsChan := make(chan TokenValidationResult, len(inputs))
	
	// Use worker pool for pin checking
	workChan := make(chan TokenValidationResult, ptsv.pinChecker.maxWorkers)
	var wg sync.WaitGroup
	
	// Start workers
	for w := 0; w < ptsv.pinChecker.maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for input := range workChan {
				result := input // Copy input
				
				if result.Error == nil && result.TokenStateHash != "" {
					// Check if already pinned
					pinInfo, err := ptsv.core.w.GetStatePinnedInfo(result.TokenStateHash)
					if err != nil {
						result.Error = fmt.Errorf("error checking pin status: %w", err)
					} else {
						result.IsPinnedLocally = (pinInfo != nil)
					}
				}
				
				resultsChan <- result
			}
		}()
	}
	
	// Queue work
	go func() {
		for _, input := range inputs {
			workChan <- input
		}
		close(workChan)
	}()
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect results
	for result := range resultsChan {
		results[result.Index] = result
	}
	
	return results
}

// Phase 4: DHT checks for unpinned tokens
func (ptsv *ParallelTokenStateValidator) batchDHTChecks(inputs []TokenValidationResult) []TokenValidationResult {
	results := make([]TokenValidationResult, len(inputs))
	resultsChan := make(chan TokenValidationResult, len(inputs))
	
	// Filter tokens that need DHT checks
	var needsDHTCheck []TokenValidationResult
	for _, input := range inputs {
		if input.Error == nil && !input.IsPinnedLocally && input.TokenStateHash != "" {
			needsDHTCheck = append(needsDHTCheck, input)
		} else {
			// Pass through results that don't need DHT check
			results[input.Index] = input
		}
	}
	
	if len(needsDHTCheck) == 0 {
		return results
	}
	
	// Use worker pool for DHT lookups
	workChan := make(chan TokenValidationResult, ptsv.dhtChecker.maxWorkers)
	var wg sync.WaitGroup
	
	// Start workers
	for w := 0; w < ptsv.dhtChecker.maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for input := range workChan {
				result := input // Copy input
				
				// Perform DHT lookup
				providers, err := ptsv.core.GetDHTddrs(result.TokenStateHash)
				if err != nil {
					result.Error = fmt.Errorf("error in DHT lookup: %w", err)
				} else {
					result.DHTProviders = providers
				}
				
				resultsChan <- result
			}
		}()
	}
	
	// Queue work
	go func() {
		for _, input := range needsDHTCheck {
			workChan <- input
		}
		close(workChan)
	}()
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect DHT results
	for result := range resultsChan {
		results[result.Index] = result
	}
	
	return results
}

// Phase 5: Assemble final results
func (ptsv *ParallelTokenStateValidator) assembleResults(
	finalResults []TokenStateCheckResult,
	validationResults []TokenValidationResult,
) {
	// Get cached quorum peers
	quorumPeers := ptsv.getQuorumPeers()
	
	for _, vr := range validationResults {
		result := TokenStateCheckResult{
			Token:                 vr.Token,
			tokenIDTokenStateHash: vr.TokenStateHash,
			tokenIDTokenStateData: vr.TokenStateData,
		}
		
		if vr.Error != nil {
			result.Error = vr.Error
			result.Message = vr.Error.Error()
		} else if vr.IsPinnedLocally {
			result.Message = "Tokenstate already pinned locally"
			result.Exhausted = false
		} else if ptsv.core.cfg.CfgData.TrustedNetwork {
			// Trusted network mode - no DHT check needed
			result.Exhausted = false
			result.Message = "Token state is free, Unique Txn"
		} else {
			// Check DHT providers
			filteredProviders := ptsv.filterProviders(vr.DHTProviders, quorumPeers)
			if len(filteredProviders) > 1 {
				result.Exhausted = true
				result.Message = fmt.Sprintf("Token state is exhausted, Token being Double spent: %s", vr.Token)
			} else {
				result.Exhausted = false
				result.Message = "Token state is free, Unique Txn"
			}
		}
		
		finalResults[vr.Index] = result
	}
}

// Helper functions

func (ptsv *ParallelTokenStateValidator) initializeQuorumPeerCache(quorumList []string) {
	ptsv.cacheMu.Lock()
	defer ptsv.cacheMu.Unlock()
	
	ptsv.quorumPeerCache = make([]string, 0, len(quorumList))
	
	for _, addr := range quorumList {
		pId, _, ok := util.ParseAddress(addr)
		if ok {
			ptsv.quorumPeerCache = append(ptsv.quorumPeerCache, pId)
		}
	}
	
	ptsv.log.Debug("Pre-cached quorum peer IDs", "count", len(ptsv.quorumPeerCache))
}

func (ptsv *ParallelTokenStateValidator) getQuorumPeers() []string {
	ptsv.cacheMu.RLock()
	defer ptsv.cacheMu.RUnlock()
	
	// Create a copy including sender
	peers := make([]string, len(ptsv.quorumPeerCache))
	copy(peers, ptsv.quorumPeerCache)
	
	if ptsv.senderPeerID != "" {
		peers = append(peers, ptsv.senderPeerID)
	}
	
	return peers
}

func (ptsv *ParallelTokenStateValidator) filterProviders(providers []string, excludePeers []string) []string {
	// Create exclusion map for O(1) lookup
	excludeMap := make(map[string]bool)
	for _, peer := range excludePeers {
		excludeMap[peer] = true
	}
	
	// Filter providers
	filtered := make([]string, 0, len(providers))
	for _, provider := range providers {
		if !excludeMap[provider] {
			filtered = append(filtered, provider)
		}
	}
	
	return filtered
}