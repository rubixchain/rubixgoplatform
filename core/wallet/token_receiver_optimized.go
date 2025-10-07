package wallet

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	ipfsnode "github.com/ipfs/go-ipfs-api"
)

// OptimizedTokensReceived processes received tokens with batch IPFS Get operations
func (w *Wallet) OptimizedTokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, pinningServiceMode bool, ipfsShell *ipfsnode.Shell) ([]string, error) {
	startTime := time.Now()
	w.l.Lock()
	defer w.l.Unlock()
	
	w.log.Info("Starting optimized token receive", 
		"token_count", len(ti),
		"transaction_id", b.GetTid(),
		"sender", senderPeerId,
		"receiver", receiverPeerId)
	
	// Create token block first
	err := w.CreateTokenBlock(b)
	if err != nil {
		blockId, _ := b.GetBlockID(ti[0].Token)
		w.log.Error("Failed to create token block", 
			"block_id", blockId,
			"error", err)
		return nil, err
	}

	// Phase 1: Prepare token state hashes and identify tokens to download
	updatedtokenhashes := make([]string, len(ti))
	tokenHashMap := make(map[string]string)
	providerMaps := make([]model.TokenProviderMap, 0, len(ti))
	
	// First pass: Add token states to IPFS
	addStart := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 10) // Limit concurrent IPFS operations
	
	for idx, info := range ti {
		wg.Add(1)
		go func(i int, tokenInfo contract.TokenInfo) {
			defer wg.Done()
			
			// Rate limiting
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			t := tokenInfo.Token
			b := w.GetLatestTokenBlock(tokenInfo.Token, tokenInfo.TokenType)
			blockId, _ := b.GetBlockID(t)
			tokenIDTokenStateData := t + blockId
			tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
			tokenIDTokenStateHash, tpm, err := w.AddWithProviderMap(tokenIDTokenStateBuffer, did, OwnerRole)
			
			if err != nil {
				w.log.Error("Failed to add token state", 
					"token", t,
					"error", err)
				return
			}
			
			mu.Lock()
			updatedtokenhashes[i] = tokenIDTokenStateHash
			tokenHashMap[t] = tokenIDTokenStateHash
			// Fill in extra fields for pinning
			tpm.FuncID = PinFunc
			tpm.TransactionID = b.GetTid()
			tpm.Sender = senderPeerId + "." + b.GetSenderDID()
			tpm.Receiver = receiverPeerId + "." + b.GetReceiverDID()
			tpm.TokenValue = tokenInfo.TokenValue
			providerMaps = append(providerMaps, tpm)
			mu.Unlock()
		}(idx, info)
	}
	wg.Wait()
	
	w.log.Info("Token state addition completed", 
		"count", len(ti),
		"duration", time.Since(addStart))

	// Phase 2: Identify tokens that need to be downloaded
	getRequests := make([]GetRequest, 0)
	tokenIndexMap := make(map[string]int) // Map token ID to its index in ti
	downloadDirs := make([]string, 0)
	
	checkStart := time.Now()
	for i, tokenInfo := range ti {
		// Check if token already exists
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", tokenInfo.Token)
		if err != nil || t.TokenID == "" {
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
	}
	
	w.log.Info("Token existence check completed", 
		"existing_tokens", len(ti)-len(getRequests),
		"tokens_to_download", len(getRequests),
		"duration", time.Since(checkStart))

	// Phase 3: Batch download tokens with all-or-nothing semantics
	var downloadProviderMaps []model.TokenProviderMap
	if len(getRequests) > 0 {
		downloadStart := time.Now()
		downloadProviderMaps, err = w.BatchGetWithProviderMaps(getRequests)
		if err != nil {
			w.log.Error("Batch download failed, cleaning up", 
				"error", err)
			
			// Cleanup created directories
			for _, dir := range downloadDirs {
				if rmErr := os.RemoveAll(dir); rmErr != nil {
					w.log.Error("Failed to cleanup directory", 
						"dir", dir,
						"error", rmErr)
				}
			}
			
			// Ensure no orphaned DB records
			tokenIDs := make([]string, len(getRequests))
			for i, req := range getRequests {
				tokenIDs[i] = req.Hash
			}
			if cleanupErr := w.ensureNoOrphanedRecords(tokenIDs, b.GetTid()); cleanupErr != nil {
				w.log.Error("Failed to cleanup orphaned records", 
					"error", cleanupErr)
			}
			
			return nil, fmt.Errorf("failed to download tokens: %v", err)
		}
		
		w.log.Info("Batch download completed successfully", 
			"count", len(getRequests),
			"duration", time.Since(downloadStart))
	}
	
	// Merge provider maps
	allProviderMaps := append(providerMaps, downloadProviderMaps...)

	// Phase 4: Process downloaded tokens
	processStart := time.Now()
	processedCount := 0
	
	for _, req := range getRequests {
		tokenInfo := ti[req.Index]
		
		// Get parent token details
		var parentTokenID string
		gb := w.GetGenesisTokenBlock(tokenInfo.Token, tokenInfo.TokenType)
		if gb != nil {
			parentTokenID, _, _ = gb.GetParentDetials(tokenInfo.Token)
		}

		// Create new token entry
		t := Token{
			TokenID:       tokenInfo.Token,
			TokenValue:    tokenInfo.TokenValue,
			ParentTokenID: parentTokenID,
			DID:           tokenInfo.OwnerDID,
		}

		err = w.s.Write(TokenStorage, &t)
		if err != nil {
			w.log.Error("Failed to write token to db", 
				"token", tokenInfo.Token,
				"error", err)
			// Continue processing other tokens
			continue
		}
		
		// Update token status
		tokenStatus := TokenIsPending // Changed from TokenIsFree to prevent premature spending
		role := OwnerRole
		ownerdid := did
		if pinningServiceMode {
			tokenStatus = TokenIsPinnedAsService
			role = PinningRole
			ownerdid = b.GetOwner()
		}

		t.DID = ownerdid
		t.TokenStatus = tokenStatus
		t.TransactionID = b.GetTid()
		t.TokenStateHash = tokenHashMap[tokenInfo.Token]
		t.SyncStatus = SyncIncomplete

		err = w.s.Update(TokenStorage, &t, "token_id=?", tokenInfo.Token)
		if err != nil {
			w.log.Error("Failed to update token in db", 
				"token", tokenInfo.Token,
				"error", err)
			continue
		}
		
		senderAddress := senderPeerId + "." + b.GetSenderDID()
		receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
		
		// Pin the token
		_, err = w.Pin(tokenInfo.Token, role, did, b.GetTid(), senderAddress, receiverAddress, tokenInfo.TokenValue, true)
		if err != nil {
			w.log.Error("Failed to pin token", 
				"token", tokenInfo.Token,
				"error", err)
			continue
		}
		
		processedCount++
		
		// Cleanup the download directory
		os.RemoveAll(req.Path)
	}
	
	w.log.Info("Token processing completed", 
		"processed", processedCount,
		"total", len(getRequests),
		"duration", time.Since(processStart))

	// Phase 5: Handle provider details
	providerStart := time.Now()
	if len(allProviderMaps) > 100 && w.asyncProviderMgr != nil {
		err := w.asyncProviderMgr.SubmitProviderDetails(allProviderMaps, b.GetTid())
		if err != nil {
			w.log.Error("Failed to submit provider details to async queue, falling back to sync", 
				"count", len(allProviderMaps),
				"error", err)
			goto syncProcessing
		}
		w.log.Info("Provider details submitted for async processing", 
			"transaction_id", b.GetTid(),
			"count", len(allProviderMaps),
			"duration", time.Since(providerStart))
		
		w.log.Info("Optimized token receive completed", 
			"total_tokens", len(ti),
			"downloaded", len(getRequests),
			"total_duration", time.Since(startTime))
		
		return updatedtokenhashes, nil
	}

syncProcessing:
	// Batch write provider details synchronously
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := w.AddProviderDetailsBatch(allProviderMaps)
		if err == nil {
			w.log.Info("Provider details batch write successful", 
				"count", len(allProviderMaps),
				"attempt", attempt+1,
				"duration", time.Since(providerStart))
			
			w.log.Info("Optimized token receive completed", 
				"total_tokens", len(ti),
				"downloaded", len(getRequests),
				"total_duration", time.Since(startTime))
			
			return updatedtokenhashes, nil
		}
		w.log.Error("Batch AddProviderDetails failed, retrying", 
			"attempt", attempt+1,
			"error", err)
		time.Sleep(backoff(attempt))
	}
	
	return nil, fmt.Errorf("failed to batch add provider details after retries")
}

