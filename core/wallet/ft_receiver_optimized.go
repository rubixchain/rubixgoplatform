package wallet

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
)

// OptimizedFTTokensReceived processes received FT tokens with batch IPFS Get operations
func (w *Wallet) OptimizedFTTokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, ipfsShell *ipfsnode.Shell, ftInfo FTToken) ([]string, error) {
	startTime := time.Now()
	totalTokens := len(ti)
	
	// Set up memory optimization for large operations
	memOptimizer := NewMemoryOptimizer(w.log)
	memOptimizer.OptimizeForLargeOperation(totalTokens)
	defer memOptimizer.RestoreDefaults()
	
	// Start memory monitoring for large operations
	if totalTokens > 100 {
		monitorDone := make(chan struct{})
		go memOptimizer.MonitorMemoryPressure(monitorDone)
		defer close(monitorDone)
	}
	
	// Create token block with minimal locking
	w.l.Lock()
	err := w.CreateTokenBlock(b)
	w.l.Unlock()
	if err != nil {
		w.log.Error("Failed to create token block", "error", err)
		return nil, err
	}

	// Dynamic worker allocation based on resources
	rm := &ResourceMonitor{}
	dynamicWorkers := rm.CalculateDynamicWorkers(totalTokens)
	
	w.log.Info("Starting optimized FT token receive",
		"ft_count", totalTokens,
		"ft_name", ftInfo.FTName,
		"transaction_id", b.GetTid(),
		"sender", senderPeerId,
		"receiver", receiverPeerId,
		"workers", dynamicWorkers)

	// Phase 1: Parallel token state hash computation
	updatedtokenhashes := make([]string, 0, totalTokens)
	tokenHashMap := make(map[string]string)
	providerMaps := make([]model.TokenProviderMap, 0, totalTokens)
	hashMapMu := sync.Mutex{}
	providerMapMu := sync.Mutex{}

	addStart := time.Now()
	
	// Create batches for parallel processing
	// Dynamic batch size to ensure proper parallelism
	var batchSize int
	switch {
	case totalTokens <= 10:
		batchSize = 1 // Process individually for very small counts
	case totalTokens <= 50:
		batchSize = 5 // 10 batches for 50 tokens
	case totalTokens <= 100:
		batchSize = 10 // 10 batches for 100 tokens
	case totalTokens <= 500:
		batchSize = 25 // 20 batches for 500 tokens
	case totalTokens <= 1000:
		batchSize = 50 // 20 batches for 1000 tokens
	default:
		batchSize = 100 // Larger batches for very large transfers
	}
	
	var wg sync.WaitGroup
	sem := make(chan struct{}, dynamicWorkers)
	
	for i := 0; i < len(ti); i += batchSize {
		end := i + batchSize
		if end > len(ti) {
			end = len(ti)
		}
		
		wg.Add(1)
		go func(batch []contract.TokenInfo, startIdx int) {
			defer wg.Done()
			
			sem <- struct{}{}
			defer func() { <-sem }()
			
			localHashes := make([]string, 0, len(batch))
			localProviderMaps := make([]model.TokenProviderMap, 0, len(batch))
			localHashMap := make(map[string]string)
			
			for _, info := range batch {
				t := info.Token
				// Lock only for reading block
				w.l.Lock()
				b := w.GetLatestTokenBlock(info.Token, info.TokenType)
				w.l.Unlock()
				
				if b == nil {
					w.log.Error("No block found for token", "token", t)
					continue
				}
				
				blockId, _ := b.GetBlockID(t)
				tokenIDTokenStateData := t + blockId
				tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
				tokenIDTokenStateHash, tpm, _ := w.AddWithProviderMap(tokenIDTokenStateBuffer, did, OwnerRole)
				
				localHashes = append(localHashes, tokenIDTokenStateHash)
				localHashMap[t] = tokenIDTokenStateHash
				
				// Fill in extra fields for pinning
				tpm.FuncID = PinFunc
				tpm.TransactionID = b.GetTid()
				tpm.Sender = senderPeerId + "." + b.GetSenderDID()
				tpm.Receiver = receiverPeerId + "." + b.GetReceiverDID()
				tpm.TokenValue = info.TokenValue
				localProviderMaps = append(localProviderMaps, tpm)
			}
			
			// Merge results
			hashMapMu.Lock()
			for k, v := range localHashMap {
				tokenHashMap[k] = v
			}
			updatedtokenhashes = append(updatedtokenhashes, localHashes...)
			hashMapMu.Unlock()
			
			providerMapMu.Lock()
			providerMaps = append(providerMaps, localProviderMaps...)
			providerMapMu.Unlock()
			
			// Log progress
			processed := startIdx + len(batch)
			currentPercent := (processed * 100) / totalTokens
			w.log.Debug("Phase 1 - Token state hashes batch completed",
				"batch_start", startIdx,
				"batch_size", len(batch),
				"total_processed", processed,
				"percent", currentPercent)
		}(ti[i:end], i)
	}
	
	wg.Wait()

	w.log.Info("FT token state addition completed",
		"count", len(ti),
		"duration", time.Since(addStart))

	// Phase 2: Identify FT tokens that need to be downloaded
	getRequests := make([]GetRequest, 0)
	tokenIndexMap := make(map[string]int)
	downloadDirs := make([]string, 0)

	checkStart := time.Now()
	lastLoggedPercent := 0
	for i, tokenInfo := range ti {
		var FTInfo FTToken
		err := w.s.Read(FTTokenStorage, &FTInfo, "token_id=?", tokenInfo.Token)
		if err != nil || FTInfo.TokenID == "" {
			// Token doesn't exist, need to download
			dir := util.GetRandString()
			if err := util.CreateDir(dir); err != nil {
				w.log.Error("Failed to create directory",
					"dir", dir,
					"error", err)
				// Cleanup previously created dirs
				for _, d := range downloadDirs {
					os.RemoveAll(d)
				}
				return nil, fmt.Errorf("failed to create directory: %v", err)
			}
			downloadDirs = append(downloadDirs, dir)

			getRequests = append(getRequests, GetRequest{
				Hash:  tokenInfo.Token,
				DID:   did,
				Role:  OwnerRole,
				Path:  dir,
				Index: i,
			})
			tokenIndexMap[tokenInfo.Token] = i
		}

		// Log progress every 10%
		currentPercent := ((i + 1) * 100) / len(ti)
		if currentPercent >= lastLoggedPercent+10 || i == len(ti)-1 {
			w.log.Info("Phase 2 - Token existence check progress",
				"percent", currentPercent,
				"checked", i+1,
				"total", len(ti),
				"to_download", len(getRequests))
			lastLoggedPercent = (currentPercent / 10) * 10
		}
	}

	w.log.Info("FT token existence check completed",
		"existing_tokens", len(ti)-len(getRequests),
		"tokens_to_download", len(getRequests),
		"duration", time.Since(checkStart))

	// Phase 3: Parallel batch download FT tokens
	var downloadProviderMaps []model.TokenProviderMap
	if len(getRequests) > 0 {
		downloadStart := time.Now()
		
		// Split downloads into smaller batches for better parallelism
		// Dynamic download batch size based on number of downloads
		var downloadBatchSize int
		switch {
		case len(getRequests) <= 10:
			downloadBatchSize = 2 // 5 batches for 10 downloads
		case len(getRequests) <= 50:
			downloadBatchSize = 5 // 10 batches for 50 downloads
		case len(getRequests) <= 100:
			downloadBatchSize = 10 // 10 batches for 100 downloads
		default:
			downloadBatchSize = 25 // Larger batches for many downloads
		}
		var downloadWg sync.WaitGroup
		downloadSem := make(chan struct{}, dynamicWorkers/2) // Limit IPFS operations
		downloadResults := make([]model.TokenProviderMap, 0, len(getRequests))
		downloadResultsMu := sync.Mutex{}
		downloadErrors := make([]error, 0)
		downloadErrorsMu := sync.Mutex{}
		
		for i := 0; i < len(getRequests); i += downloadBatchSize {
			end := i + downloadBatchSize
			if end > len(getRequests) {
				end = len(getRequests)
			}
			
			downloadWg.Add(1)
			go func(batch []GetRequest) {
				defer downloadWg.Done()
				
				downloadSem <- struct{}{}
				defer func() { <-downloadSem }()
				
				providerMaps, err := w.BatchGetWithProviderMaps(batch)
				if err != nil {
					downloadErrorsMu.Lock()
					downloadErrors = append(downloadErrors, err)
					downloadErrorsMu.Unlock()
					
					// Cleanup directories for failed batch
					for _, req := range batch {
						os.RemoveAll(req.Path)
					}
				} else {
					downloadResultsMu.Lock()
					downloadResults = append(downloadResults, providerMaps...)
					downloadResultsMu.Unlock()
				}
			}(getRequests[i:end])
		}
		
		downloadWg.Wait()
		
		if len(downloadErrors) > 0 {
			w.log.Error("Some FT batch downloads failed",
				"errors", len(downloadErrors),
				"first_error", downloadErrors[0])
			
			// Cleanup all directories on any failure
			for _, dir := range downloadDirs {
				os.RemoveAll(dir)
			}
			
			return nil, fmt.Errorf("failed to download %d batches of FT tokens", len(downloadErrors))
		}
		
		downloadProviderMaps = downloadResults
		w.log.Info("FT batch download completed successfully",
			"count", len(getRequests),
			"batches", (len(getRequests)+downloadBatchSize-1)/downloadBatchSize,
			"duration", time.Since(downloadStart))
	}

	// Merge provider maps
	allProviderMaps := append(providerMaps, downloadProviderMaps...)

	// Group tokens by genesis for optimized owner lookup
	genesisOptimizer := NewGenesisGroupOptimizer(w)
	genesisGroups := genesisOptimizer.GroupTokensByGenesis(ti)
	
	// Phase 4: Process all tokens in parallel (both downloaded and existing)
	processStart := time.Now()
	
	// Create a map to quickly check if a token was downloaded
	downloadedTokensMap := make(map[string]int) // token -> index in getRequests
	for idx, req := range getRequests {
		downloadedTokensMap[ti[req.Index].Token] = idx
	}
	
	// Determine token status once
	tokenStatus := TokenIsFree
	if senderPeerId != receiverPeerId {
		tokenStatus = TokenIsPending
	}
	
	// Process tokens in parallel
	type processResult struct {
		success bool
		token   string
		err     error
	}
	
	resultChan := make(chan processResult, len(ti))
	processWg := sync.WaitGroup{}
	processSem := make(chan struct{}, dynamicWorkers)
	
	// Process new tokens (downloaded)
	for _, tokenInfo := range ti {
		if downloadIdx, wasDownloaded := downloadedTokensMap[tokenInfo.Token]; wasDownloaded {
			processWg.Add(1)
			go func(tInfo contract.TokenInfo, reqIdx int) {
				defer processWg.Done()
				
				processSem <- struct{}{}
				defer func() { <-processSem }()
				
				req := getRequests[reqIdx]
				
				// Get owner from genesis groups (optimized lookup)
				FTOwner := genesisOptimizer.GetOwnerForToken(tInfo, genesisGroups)
				if FTOwner == "" {
					resultChan <- processResult{
						success: false,
						token:   tInfo.Token,
						err:     fmt.Errorf("failed to get owner from genesis groups"),
					}
					os.RemoveAll(req.Path)
					return
				}
				
				// Create new FT token entry
				FTInfo := FTToken{
					TokenID:        tInfo.Token,
					TokenValue:     tInfo.TokenValue,
					CreatorDID:     FTOwner,
					FTName:         ftInfo.FTName,
					DID:            did,
					TokenStatus:    tokenStatus,
					TransactionID:  b.GetTid(),
					TokenStateHash: tokenHashMap[tInfo.Token],
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}
				
				err := util.RetrySQLiteWrite(func() error {
					return w.s.Write(FTTokenStorage, &FTInfo)
				}, 3, 100*time.Millisecond)
				
				if err != nil {
					resultChan <- processResult{
						success: false,
						token:   tInfo.Token,
						err:     err,
					}
				} else {
					resultChan <- processResult{
						success: true,
						token:   tInfo.Token,
					}
				}
				
				// Cleanup download directory
				os.RemoveAll(req.Path)
			}(tokenInfo, downloadIdx)
		}
	}
	
	// Process existing tokens in parallel
	for _, tokenInfo := range ti {
		if _, wasDownloaded := downloadedTokensMap[tokenInfo.Token]; !wasDownloaded {
			processWg.Add(1)
			go func(tInfo contract.TokenInfo) {
				defer processWg.Done()
				
				processSem <- struct{}{}
				defer func() { <-processSem }()
				
				// Read existing token
				var FTInfo FTToken
				err := w.s.Read(FTTokenStorage, &FTInfo, "token_id=?", tInfo.Token)
				if err != nil {
					resultChan <- processResult{
						success: false,
						token:   tInfo.Token,
						err:     fmt.Errorf("failed to read token: %w", err),
					}
					return
				}
				
				// Update token fields
				FTInfo.FTName = ftInfo.FTName
				FTInfo.DID = did
				FTInfo.TokenStatus = tokenStatus
				FTInfo.TransactionID = b.GetTid()
				FTInfo.TokenStateHash = tokenHashMap[tInfo.Token]
				
				err = util.RetrySQLiteWrite(func() error {
					return w.s.Update(FTTokenStorage, &FTInfo, "token_id=?", tInfo.Token)
				}, 3, 100*time.Millisecond)
				
				if err != nil {
					resultChan <- processResult{
						success: false,
						token:   tInfo.Token,
						err:     err,
					}
				} else {
					resultChan <- processResult{
						success: true,
						token:   tInfo.Token,
					}
				}
			}(tokenInfo)
		}
	}
	
	// Wait for all processing to complete
	go func() {
		processWg.Wait()
		close(resultChan)
	}()
	
	// Collect results
	processedCount := 0
	failedCount := 0
	for result := range resultChan {
		if result.success {
			processedCount++
		} else {
			failedCount++
			w.log.Error("Failed to process token",
				"token", result.token,
				"error", result.err)
		}
		
		// Log progress
		total := processedCount + failedCount
		if total%100 == 0 || total == len(ti) {
			w.log.Info("Phase 4 - Processing tokens progress",
				"processed", processedCount,
				"failed", failedCount,
				"total", len(ti),
				"percent", (total * 100) / len(ti))
		}
	}
	
	// Phase 5: Parallel token pinning (if needed)
	if senderPeerId != receiverPeerId {
		pinStart := time.Now()
		senderAddress := senderPeerId + "." + b.GetSenderDID()
		receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
		
		pinWg := sync.WaitGroup{}
		pinSem := make(chan struct{}, dynamicWorkers)
		pinnedCount := int32(0)
		
		for _, tokenInfo := range ti {
			pinWg.Add(1)
			go func(tInfo contract.TokenInfo) {
				defer pinWg.Done()
				
				pinSem <- struct{}{}
				defer func() { <-pinSem }()
				
				_, err := w.Pin(tInfo.Token, OwnerRole, did, b.GetTid(), 
					senderAddress, receiverAddress, tInfo.TokenValue, true)
				if err != nil {
					w.log.Error("Failed to pin FT token",
						"token", tInfo.Token,
						"error", err)
				} else {
					atomic.AddInt32(&pinnedCount, 1)
				}
			}(tokenInfo)
		}
		
		pinWg.Wait()
		w.log.Info("Token pinning completed",
			"pinned", atomic.LoadInt32(&pinnedCount),
			"total", len(ti),
			"duration", time.Since(pinStart))
	}

	w.log.Info("FT token processing completed",
		"downloaded", len(getRequests),
		"processed", processedCount,
		"total", len(ti),
		"duration", time.Since(processStart))

	// Phase 6: Handle provider details
	providerStart := time.Now()
	// Use async processing for all transfers to avoid blocking on DB operations
	if len(allProviderMaps) >= 10 && w.asyncProviderMgr != nil {
		err := w.asyncProviderMgr.SubmitProviderDetails(allProviderMaps, b.GetTid())
		if err != nil {
			w.log.Error("Failed to submit provider details to async queue, falling back to sync",
				"count", len(allProviderMaps),
				"error", err)
			goto syncProcessing
		}
		w.log.Info("FT provider details submitted for async processing",
			"transaction_id", b.GetTid(),
			"count", len(allProviderMaps),
			"duration", time.Since(providerStart))

		duration := time.Since(startTime)
		tokensPerSecond := float64(len(ti)) / duration.Seconds()
		
		w.log.Info("Optimized FT token receive completed",
			"total_tokens", len(ti),
			"downloaded", len(getRequests),
			"processed", processedCount,
			"failed", failedCount,
			"total_duration", duration,
			"tokens_per_second", fmt.Sprintf("%.2f", tokensPerSecond))

		if failedCount > 0 {
			return updatedtokenhashes, fmt.Errorf("processed %d of %d tokens successfully", 
				processedCount, len(ti))
		}
		
		return updatedtokenhashes, nil
	}

syncProcessing:
	// Batch write provider details synchronously
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := w.AddProviderDetailsBatch(allProviderMaps)
		if err == nil {
			w.log.Info("FT provider details batch write successful",
				"count", len(allProviderMaps),
				"attempt", attempt+1,
				"duration", time.Since(providerStart))

			duration := time.Since(startTime)
			tokensPerSecond := float64(len(ti)) / duration.Seconds()
			
			w.log.Info("Optimized FT token receive completed",
				"total_tokens", len(ti),
				"downloaded", len(getRequests),
				"processed", processedCount,
				"failed", failedCount,
				"total_duration", duration,
				"tokens_per_second", fmt.Sprintf("%.2f", tokensPerSecond))

			if failedCount > 0 {
				return updatedtokenhashes, fmt.Errorf("processed %d of %d tokens successfully", 
					processedCount, len(ti))
			}
			
			return updatedtokenhashes, nil
		}
		w.log.Error("Batch AddProviderDetails failed, retrying",
			"attempt", attempt+1,
			"error", err)
		time.Sleep(backoff(attempt))
	}

	return nil, fmt.Errorf("failed to batch add provider details after retries")
}
