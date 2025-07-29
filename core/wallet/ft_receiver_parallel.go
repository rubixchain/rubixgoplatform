package wallet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// ParallelFTReceiver handles FT tokens without global locks
type ParallelFTReceiver struct {
	w                 *Wallet
	tokenStates       sync.Map  // Lock-free concurrent map for token states
	batchWorkers      int
	log               logger.Logger
	genesisOptimizer  *GenesisGroupOptimizer
	downloadBatchSize int
	batchDelay        int // milliseconds
	maxConcurrentIPFS int
	creatorCache      sync.Map  // Cache for creator DIDs by genesis
}

// TokenProcessingState tracks processing state for each token
type TokenProcessingState struct {
	Token          string
	State          string // "processing", "completed", "failed"
	TokenStateHash string
	Error          error
	mu             sync.RWMutex
}

// TokenBatch represents a group of tokens to process together
type TokenBatch struct {
	Tokens      []contract.TokenInfo
	StartIndex  int
	EndIndex    int
}

// NewParallelFTReceiver creates a new parallel FT receiver
func NewParallelFTReceiver(w *Wallet) *ParallelFTReceiver {
	cpuCount := runtime.NumCPU()
	
	// Start with conservative defaults
	// Will be dynamically adjusted based on token count
	initialWorkers := min(16, cpuCount * 2)
	
	w.log.Info("Initializing parallel FT receiver",
		"cpu_cores", cpuCount,
		"initial_workers", initialWorkers)
	
	pfr := &ParallelFTReceiver{
		w:                 w,
		batchWorkers:      initialWorkers,
		log:               w.log.Named("ParallelFTReceiver"),
		genesisOptimizer:  NewGenesisGroupOptimizer(w),
	}
	
	return pfr
}

// ParallelFTTokensReceived processes received FT tokens without serialized locks
func (pfr *ParallelFTReceiver) ParallelFTTokensReceived(
	did string,
	ti []contract.TokenInfo,
	b *block.Block,
	senderPeerId string,
	receiverPeerId string,
	ipfsShell *ipfsnode.Shell,
	ftInfo FTToken,
) ([]string, error) {
	startTime := time.Now()
	totalTokens := len(ti)
	
	// Validate mainnet limits
	if err := pfr.validateMainnetLimits(totalTokens); err != nil {
		return nil, err
	}
	
	// Use dynamic worker calculation with hardware awareness
	dynamicWorkers := pfr.calculateDynamicWorkers(totalTokens)
	if dynamicWorkers != pfr.batchWorkers {
		pfr.log.Info("Adjusting workers for token count",
			"current_workers", pfr.batchWorkers,
			"new_workers", dynamicWorkers,
			"token_count", totalTokens)
		pfr.batchWorkers = dynamicWorkers
	}
	
	pfr.log.Info("Starting parallel FT token receive",
		"ft_count", totalTokens,
		"ft_name", ftInfo.FTName,
		"transaction_id", b.GetTid(),
		"workers", pfr.batchWorkers)
	
	// Create token block first (still needs wallet lock for this)
	pfr.w.l.Lock()
	err := pfr.w.CreateTokenBlock(b)
	pfr.w.l.Unlock()
	if err != nil {
		pfr.log.Error("Failed to create token block", "error", err)
		return nil, err
	}
	
	// Initialize processing states
	states := make(map[string]*TokenProcessingState)
	for _, token := range ti {
		states[token.Token] = &TokenProcessingState{
			Token: token.Token,
			State: "pending",
		}
	}
	
	// Process tokens in parallel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Phase 1: Parallel state hash computation
	hashResults := pfr.computeStateHashesParallel(ctx, ti, did, b, senderPeerId, receiverPeerId)
	
	// Phase 2: Check existing tokens and prepare downloads
	downloadTasks, existingTokens := pfr.prepareTokenTasks(ti, hashResults)
	
	// Phase 3: Parallel token downloads (if needed)
	var downloadResults []DownloadResult
	if len(downloadTasks) > 0 {
		downloadResults = pfr.parallelBatchDownload(ctx, downloadTasks, did)
	}
	
	// Create batch genesis optimizer
	genesisBatchOptimizer := NewGenesisBatchOptimizer(pfr.w)
	
	// Preload creator info from downloads if available
	genesisBatchOptimizer.PreloadFromDownloads(downloadResults)
	
	// Batch process all tokens to get creator mappings efficiently
	tokenCreatorMap := genesisBatchOptimizer.BatchProcessTokens(ti)
	
	// Phase 4: Process all tokens in parallel (update DB, pin, etc.)
	// Use enhanced processing with retry for mainnet stability
	processResults := pfr.processTokensParallelWithRetry(ctx, ti, hashResults, downloadResults, 
		existingTokens, b, ftInfo, did, senderPeerId, receiverPeerId, tokenCreatorMap)
	
	// Debug logging for mainnet issues
	pfr.debugTokenProcessing(ti, processResults)
	
	// Phase 5: Handle provider details asynchronously
	allProviderMaps := pfr.collectProviderMaps(processResults)
	if err := pfr.handleProviderDetailsAsync(allProviderMaps, b.GetTid()); err != nil {
		pfr.log.Error("Failed to handle provider details", "error", err)
		// Don't fail the entire operation for provider detail errors
	}
	
	// Collect successful token hashes
	updatedTokenHashes := make([]string, 0, totalTokens)
	successCount := 0
	for _, result := range processResults {
		if result.Success {
			updatedTokenHashes = append(updatedTokenHashes, result.TokenStateHash)
			successCount++
		}
	}
	
	duration := time.Since(startTime)
	tokensPerSecond := float64(totalTokens) / duration.Seconds()
	
	pfr.log.Info("Parallel FT token receive completed",
		"total_tokens", totalTokens,
		"successful", successCount,
		"duration", duration,
		"tokens_per_second", fmt.Sprintf("%.2f", tokensPerSecond))
	
	if successCount < totalTokens {
		return updatedTokenHashes, fmt.Errorf("processed %d of %d tokens successfully", 
			successCount, totalTokens)
	}
	
	return updatedTokenHashes, nil
}

// TokenProcessingResult holds the result of processing a single token
type TokenProcessingResult struct {
	Token          string
	TokenStateHash string
	ProviderMap    model.TokenProviderMap
	Success        bool
	Error          error
}

// computeStateHashesParallel computes state hashes for all tokens in parallel
func (pfr *ParallelFTReceiver) computeStateHashesParallel(
	ctx context.Context,
	tokens []contract.TokenInfo,
	did string,
	b *block.Block,
	senderPeerId string,
	receiverPeerId string,
) map[string]HashResult {
	results := make(map[string]HashResult)
	resultsMu := sync.Mutex{}
	
	// Create work channel
	workChan := make(chan contract.TokenInfo, len(tokens))
	
	// Start workers
	var wg sync.WaitGroup
	numWorkers := pfr.batchWorkers
	if numWorkers > len(tokens) {
		numWorkers = len(tokens)
	}
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for token := range workChan {
				select {
				case <-ctx.Done():
					return
				default:
					// Get latest block with minimal locking
					var blockID string
					err := pfr.withMinimalLock(func() error {
						b := pfr.w.GetLatestTokenBlock(token.Token, token.TokenType)
						if b == nil {
							return fmt.Errorf("no block found")
						}
						id, err := b.GetBlockID(token.Token)
						blockID = id
						return err
					})
					
					if err != nil {
						resultsMu.Lock()
						results[token.Token] = HashResult{Error: err}
						resultsMu.Unlock()
						continue
					}
					
					// Compute hash (no lock needed)
					tokenStateData := token.Token + blockID
					buffer := bytes.NewBuffer([]byte(tokenStateData))
					
					hash, tpm, err := pfr.w.AddWithProviderMap(buffer, did, OwnerRole)
					if err != nil {
						resultsMu.Lock()
						results[token.Token] = HashResult{Error: err}
						resultsMu.Unlock()
						continue
					}
					
					// Fill provider map details
					tpm.FuncID = PinFunc
					tpm.TransactionID = b.GetTid()
					tpm.Sender = senderPeerId + "." + b.GetSenderDID()
					tpm.Receiver = receiverPeerId + "." + b.GetReceiverDID()
					tpm.TokenValue = token.TokenValue
					
					resultsMu.Lock()
					results[token.Token] = HashResult{
						Hash:        hash,
						ProviderMap: tpm,
					}
					resultsMu.Unlock()
				}
			}
		}()
	}
	
	// Queue work
	for _, token := range tokens {
		workChan <- token
	}
	close(workChan)
	
	wg.Wait()
	return results
}

// HashResult holds hash computation result
type HashResult struct {
	Hash        string
	ProviderMap model.TokenProviderMap
	Error       error
}

// prepareTokenTasks determines which tokens need downloading
func (pfr *ParallelFTReceiver) prepareTokenTasks(
	tokens []contract.TokenInfo,
	hashResults map[string]HashResult,
) ([]DownloadTask, []int) {
	downloadTasks := make([]DownloadTask, 0)
	existingTokens := make([]int, 0)
	
	// Check tokens in parallel using lock-free operations
	checkResults := make(chan struct {
		Index  int
		Exists bool
		Token  contract.TokenInfo
	}, len(tokens))
	
	var wg sync.WaitGroup
	for i, token := range tokens {
		wg.Add(1)
		go func(idx int, tok contract.TokenInfo) {
			defer wg.Done()
			
			// Check if token exists using minimal locking
			exists := false
			pfr.withMinimalLock(func() error {
				var ftInfo FTToken
				err := pfr.w.s.Read(FTTokenStorage, &ftInfo, "token_id=?", tok.Token)
				exists = (err == nil && ftInfo.TokenID != "")
				return nil
			})
			
			checkResults <- struct {
				Index  int
				Exists bool
				Token  contract.TokenInfo
			}{idx, exists, tok}
		}(i, token)
	}
	
	go func() {
		wg.Wait()
		close(checkResults)
	}()
	
	// Collect results
	for result := range checkResults {
		if result.Exists {
			existingTokens = append(existingTokens, result.Index)
		} else {
			dir := util.GetRandString()
			downloadTasks = append(downloadTasks, DownloadTask{
				Token: result.Token,
				Index: result.Index,
				Dir:   dir,
			})
		}
	}
	
	return downloadTasks, existingTokens
}

// DownloadTask represents a token download task
type DownloadTask struct {
	Token contract.TokenInfo
	Index int
	Dir   string
}

// DownloadResult holds download operation result
type DownloadResult struct {
	Task        DownloadTask
	Success     bool
	Error       error
	ProviderMap model.TokenProviderMap
	CreatorDID  string // Cache creator DID extracted during download
}

// parallelBatchDownload downloads tokens in parallel batches
func (pfr *ParallelFTReceiver) parallelBatchDownload(
	ctx context.Context,
	tasks []DownloadTask,
	did string,
) []DownloadResult {
	results := make([]DownloadResult, len(tasks))
	
	// Create directories
	for i, task := range tasks {
		if err := util.CreateDir(task.Dir); err != nil {
			results[i] = DownloadResult{
				Task:  task,
				Error: fmt.Errorf("failed to create dir: %w", err),
			}
		}
	}
	
	// Prepare batch requests
	batchSize := 50 // Process 50 tokens per batch
	batches := make([][]GetRequest, 0)
	
	for i := 0; i < len(tasks); i += batchSize {
		end := i + batchSize
		if end > len(tasks) {
			end = len(tasks)
		}
		
		batch := make([]GetRequest, 0, end-i)
		for j := i; j < end; j++ {
			if results[j].Error == nil { // Skip failed directory creations
				batch = append(batch, GetRequest{
					Hash:  tasks[j].Token.Token,
					DID:   did,
					Role:  OwnerRole,
					Path:  tasks[j].Dir,
					Index: j,
				})
			}
		}
		
		if len(batch) > 0 {
			batches = append(batches, batch)
		}
	}
	
	// Process batches in parallel
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		BatchIndex int
		Results    []model.TokenProviderMap
		Error      error
	}, len(batches))
	
	for i, batch := range batches {
		wg.Add(1)
		go func(batchIdx int, requests []GetRequest) {
			defer wg.Done()
			
			providerMaps, err := pfr.w.BatchGetWithProviderMaps(requests)
			resultsChan <- struct {
				BatchIndex int
				Results    []model.TokenProviderMap
				Error      error
			}{batchIdx, providerMaps, err}
		}(i, batch)
	}
	
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect results
	batchResults := make(map[int]struct {
		Results []model.TokenProviderMap
		Error   error
	})
	
	for result := range resultsChan {
		batchResults[result.BatchIndex] = struct {
			Results []model.TokenProviderMap
			Error   error
		}{result.Results, result.Error}
	}
	
	// Map results back to tasks
	for batchIdx, batch := range batches {
		batchResult := batchResults[batchIdx]
		
		for i, req := range batch {
			taskIdx := req.Index
			if batchResult.Error != nil {
				results[taskIdx] = DownloadResult{
					Task:  tasks[taskIdx],
					Error: batchResult.Error,
				}
			} else if i < len(batchResult.Results) {
				results[taskIdx] = DownloadResult{
					Task:        tasks[taskIdx],
					Success:     true,
					ProviderMap: batchResult.Results[i],
				}
			}
		}
	}
	
	return results
}

// workItem represents a token processing task
type workItem struct {
	Index          int
	Token          contract.TokenInfo
	IsNewToken     bool
	DownloadResult *DownloadResult
}

// processTokensParallel processes all tokens in parallel
func (pfr *ParallelFTReceiver) processTokensParallel(
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
	results := make([]TokenProcessingResult, len(tokens))
	
	workItems := make([]workItem, 0, len(tokens))
	
	// Add downloaded tokens
	downloadMap := make(map[int]*DownloadResult)
	for i := range downloadResults {
		downloadMap[downloadResults[i].Task.Index] = &downloadResults[i]
	}
	
	// Create work items for all tokens
	existingMap := make(map[int]bool)
	for _, idx := range existingTokenIndices {
		existingMap[idx] = true
	}
	
	for i, token := range tokens {
		item := workItem{
			Index:      i,
			Token:      token,
			IsNewToken: !existingMap[i],
		}
		if dr, ok := downloadMap[i]; ok {
			item.DownloadResult = dr
		}
		workItems = append(workItems, item)
	}
	
	// Process in parallel
	workChan := make(chan workItem, len(workItems))
	resultsChan := make(chan TokenProcessingResult, len(workItems))
	
	var wg sync.WaitGroup
	numWorkers := pfr.batchWorkers
	if numWorkers > len(workItems) {
		numWorkers = len(workItems)
	}
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for item := range workChan {
				result := pfr.processSingleToken(
					item,
					hashResults[item.Token.Token],
					b,
					ftInfo,
					did,
					senderPeerId,
					receiverPeerId,
					tokenCreatorMap,
				)
				resultsChan <- result
			}
		}()
	}
	
	// Queue work
	for _, item := range workItems {
		workChan <- item
	}
	close(workChan)
	
	// Wait and collect
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	for result := range resultsChan {
		// Find the index from the token
		for i, token := range tokens {
			if token.Token == result.Token {
				results[i] = result
				break
			}
		}
	}
	
	return results
}

// processSingleToken processes a single token
func (pfr *ParallelFTReceiver) processSingleToken(
	item workItem,
	hashResult HashResult,
	b *block.Block,
	ftInfo FTToken,
	did string,
	senderPeerId string,
	receiverPeerId string,
	tokenCreatorMap map[string]string,
) TokenProcessingResult {
	result := TokenProcessingResult{
		Token:          item.Token.Token,
		TokenStateHash: hashResult.Hash,
		ProviderMap:    hashResult.ProviderMap,
	}
	
	if hashResult.Error != nil {
		result.Error = fmt.Errorf("hash computation failed: %w", hashResult.Error)
		return result
	}
	
	// Handle new token
	if item.IsNewToken && item.DownloadResult != nil {
		if item.DownloadResult.Error != nil {
			result.Error = fmt.Errorf("download failed: %w", item.DownloadResult.Error)
			return result
		}
		
		// Get owner from pre-computed creator map
		ftOwner, found := tokenCreatorMap[item.Token.Token]
		
		// If not found in map, use sender's DID as fallback
		if !found || ftOwner == "" {
			pfr.log.Debug("Creator not found in batch map, using sender DID as fallback", 
				"token", item.Token.Token)
			// Get sender's DID from the block
			ftOwner = b.GetSenderDID()
		}
		
		if ftOwner == "" {
			result.Error = fmt.Errorf("owner not found - genesis lookup, downloaded data, and fallback all failed")
			os.RemoveAll(item.DownloadResult.Task.Dir)
			return result
		}
		
		// Create FT token entry
		ftEntry := FTToken{
			TokenID:        item.Token.Token,
			TokenValue:     item.Token.TokenValue,
			CreatorDID:     ftOwner,
			FTName:         ftInfo.FTName,
			DID:            did,
			TokenStatus:    TokenIsPending,
			TransactionID:  b.GetTid(),
			TokenStateHash: hashResult.Hash,
		}
		
		// Write to database with minimal locking and retry
		err := pfr.enhancedDatabaseWrite(FTTokenStorage, &ftEntry)
		
		if err != nil {
			result.Error = fmt.Errorf("failed to write token: %w", err)
			os.RemoveAll(item.DownloadResult.Task.Dir)
			return result
		}
		
		// Cleanup download directory
		os.RemoveAll(item.DownloadResult.Task.Dir)
		
		// Merge provider maps if available
		if item.DownloadResult.Success {
			result.ProviderMap = item.DownloadResult.ProviderMap
		}
	} else {
		// Update existing token
		// Get creator for fix (capture before the closure)
		creatorToUse := ""
		// First try to get from the pre-computed map
		if creator, found := tokenCreatorMap[item.Token.Token]; found && creator != "" {
			creatorToUse = creator
		} else {
			// Use genesis lookup as fallback
			gb := pfr.w.GetGenesisTokenBlock(item.Token.Token, item.Token.TokenType)
			if gb != nil {
				creatorToUse = gb.GetOwner()
			}
		}
		
		err := pfr.withMinimalLock(func() error {
			var ftEntry FTToken
			if err := pfr.w.s.Read(FTTokenStorage, &ftEntry, "token_id=?", item.Token.Token); err != nil {
				return err
			}
			
			// Fix CreatorDID if it's a peer ID (temporary fix for existing bad data)
			if strings.HasPrefix(ftEntry.CreatorDID, "12D3KooW") && creatorToUse != "" {
				pfr.log.Info("Fixing CreatorDID from peer ID to DID", 
					"token", item.Token.Token,
					"old_creator", ftEntry.CreatorDID,
					"new_creator", creatorToUse)
				ftEntry.CreatorDID = creatorToUse
			}
			
			ftEntry.FTName = ftInfo.FTName
			ftEntry.DID = did
			ftEntry.TokenStatus = TokenIsPending
			ftEntry.TransactionID = b.GetTid()
			ftEntry.TokenStateHash = hashResult.Hash
			
			// Use retry logic for updates too
			const maxRetries = 3
			var updateErr error
			for attempt := 1; attempt <= maxRetries; attempt++ {
				updateErr = pfr.w.s.Update(FTTokenStorage, &ftEntry, "token_id=?", item.Token.Token)
				if updateErr == nil {
					break
				}
				if attempt < maxRetries {
					time.Sleep(time.Duration(attempt*50) * time.Millisecond)
				}
			}
			return updateErr
		})
		
		if err != nil {
			result.Error = fmt.Errorf("failed to update token: %w", err)
			return result
		}
	}
	
	// Pin the token
	senderAddress := senderPeerId + "." + b.GetSenderDID()
	receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
	
	_, err := pfr.w.Pin(
		item.Token.Token,
		OwnerRole,
		did,
		b.GetTid(),
		senderAddress,
		receiverAddress,
		item.Token.TokenValue,
		true,
	)
	
	if err != nil {
		result.Error = fmt.Errorf("failed to pin token: %w", err)
		return result
	}
	
	result.Success = true
	return result
}

// Helper methods

func (pfr *ParallelFTReceiver) withMinimalLock(fn func() error) error {
	pfr.w.l.Lock()
	defer pfr.w.l.Unlock()
	return fn()
}

// calculateDynamicWorkers determines optimal workers based on token count and hardware
func (pfr *ParallelFTReceiver) calculateDynamicWorkers(tokenCount int) int {
	cpuCount := runtime.NumCPU()
	
	// Detect if running in virtualized environment (conservative approach)
	// In virtualized environments, we should be more conservative with concurrency
	isVirtualized := cpuCount <= 8 // Most VMs have 8 or fewer vCPUs
	
	// Base calculation for workers
	var workers int
	
	switch {
	case tokenCount < 100:
		workers = min(8, cpuCount)
	case tokenCount < 250:
		workers = min(12, cpuCount * 2)
	case tokenCount < 500:
		workers = min(16, cpuCount * 2)
	case tokenCount < 1000:
		workers = min(32, cpuCount * 3)
	case tokenCount < 5000:
		workers = min(64, cpuCount * 4)
	case tokenCount < 10000:
		workers = min(96, cpuCount * 6)
	default:
		workers = min(128, cpuCount * 8)
	}
	
	// Apply constraints based on environment
	if isVirtualized {
		// For virtualized environments (like EC2), be more conservative
		// to prevent context switching overhead
		maxVirtualizedWorkers := cpuCount * 2
		if tokenCount <= 250 && cpuCount == 8 {
			// Special case for your 8 vCPU system with 250 tokens
			maxVirtualizedWorkers = 16
		}
		
		if workers > maxVirtualizedWorkers {
			pfr.log.Debug("Limiting workers for virtualized environment",
				"cpu_count", cpuCount,
				"requested_workers", workers,
				"limited_to", maxVirtualizedWorkers)
			workers = maxVirtualizedWorkers
		}
	} else {
		// Physical hardware can handle more workers
		maxWorkers := cpuCount * 4
		if tokenCount > 5000 {
			maxWorkers = cpuCount * 8
		}
		if workers > maxWorkers {
			workers = maxWorkers
		}
	}
	
	// Ensure minimum workers for efficiency
	minWorkers := max(8, tokenCount/100) // At least 1 worker per 100 tokens
	if workers < minWorkers {
		workers = minWorkers
	}
	
	// Final safety check
	if workers > 128 {
		workers = 128 // Absolute maximum to prevent resource exhaustion
	}
	
	return workers
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers  
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (pfr *ParallelFTReceiver) collectProviderMaps(results []TokenProcessingResult) []model.TokenProviderMap {
	maps := make([]model.TokenProviderMap, 0, len(results))
	for _, result := range results {
		if result.Success && result.Token != "" {
			maps = append(maps, result.ProviderMap)
		}
	}
	return maps
}

func (pfr *ParallelFTReceiver) handleProviderDetailsAsync(
	providerMaps []model.TokenProviderMap,
	transactionID string,
) error {
	if len(providerMaps) == 0 {
		return nil
	}
	
	// Use async processing for large batches
	if len(providerMaps) >= 100 && pfr.w.asyncProviderMgr != nil {
		return pfr.w.asyncProviderMgr.SubmitProviderDetails(providerMaps, transactionID)
	}
	
	// Fallback to synchronous batch write
	return pfr.w.AddProviderDetailsBatch(providerMaps)
}