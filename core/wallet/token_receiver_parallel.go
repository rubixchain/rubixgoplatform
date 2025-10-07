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

// ParallelTokensReceived processes received tokens in parallel for better performance
func (w *Wallet) ParallelTokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, pinningServiceMode bool, ipfsShell *ipfsnode.Shell) ([]string, error) {
	w.l.Lock()
	defer w.l.Unlock()
	
	// Create token block first
	err := w.CreateTokenBlock(b)
	if err != nil {
		blockId, _ := b.GetBlockID(ti[0].Token)
		fmt.Println("failed to create token block, block Id", blockId)
		return nil, err
	}

	// Prepare token state hashes
	updatedtokenhashes := make([]string, len(ti))
	tokenHashMap := make(map[string]string)
	providerMaps := make([]model.TokenProviderMap, len(ti))
	
	// First pass: Add token states to IPFS (can be parallelized)
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
			tokenIDTokenStateHash, tpm, _ := w.AddWithProviderMap(tokenIDTokenStateBuffer, did, OwnerRole)
			
			mu.Lock()
			updatedtokenhashes[i] = tokenIDTokenStateHash
			tokenHashMap[t] = tokenIDTokenStateHash
			// Fill in extra fields for pinning
			tpm.FuncID = PinFunc
			tpm.TransactionID = b.GetTid()
			tpm.Sender = senderPeerId + "." + b.GetSenderDID()
			tpm.Receiver = receiverPeerId + "." + b.GetReceiverDID()
			tpm.TokenValue = tokenInfo.TokenValue
			providerMaps[i] = tpm
			mu.Unlock()
		}(idx, info)
	}
	wg.Wait()

	// Second pass: Process tokens in parallel batches
	tokenJobs := make(chan tokenProcessJob, len(ti))
	results := make(chan tokenProcessResult, len(ti))
	
	// Start workers
	numWorkers := 5
	if len(ti) > 100 {
		numWorkers = 10
	}
	
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go w.tokenProcessWorker(did, b, senderPeerId, receiverPeerId, pinningServiceMode, tokenHashMap, tokenJobs, results, &wg)
	}
	
	// Submit jobs
	for _, tokenInfo := range ti {
		tokenJobs <- tokenProcessJob{tokenInfo: tokenInfo}
	}
	close(tokenJobs)
	
	// Wait for workers to complete
	wg.Wait()
	close(results)
	
	// Check for errors
	var firstErr error
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
	}
	
	if firstErr != nil {
		return nil, firstErr
	}

	// Handle provider details (async for large transfers)
	if len(providerMaps) > 100 && w.asyncProviderMgr != nil {
		err := w.asyncProviderMgr.SubmitProviderDetails(providerMaps, b.GetTid())
		if err != nil {
			w.log.Error("Failed to submit provider details to async queue, falling back to sync", "err", err)
			goto syncProcessing
		}
		w.log.Info("Provider details submitted for async processing", 
			"transaction_id", b.GetTid(),
			"token_count", len(providerMaps))
		return updatedtokenhashes, nil
	}

syncProcessing:
	// Batch write provider details synchronously
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := w.AddProviderDetailsBatch(providerMaps)
		if err == nil {
			return updatedtokenhashes, nil
		}
		w.log.Error("Batch AddProviderDetails failed, retrying", "attempt", attempt+1, "err", err)
		time.Sleep(backoff(attempt))
	}
	return nil, fmt.Errorf("failed to batch add provider details after retries")
}

type tokenProcessJob struct {
	tokenInfo contract.TokenInfo
}

type tokenProcessResult struct {
	token string
	err   error
}

// tokenProcessWorker processes individual tokens
func (w *Wallet) tokenProcessWorker(did string, b *block.Block, senderPeerId, receiverPeerId string, 
	pinningServiceMode bool, tokenHashMap map[string]string, 
	jobs <-chan tokenProcessJob, results chan<- tokenProcessResult, wg *sync.WaitGroup) {
	
	defer wg.Done()
	
	for job := range jobs {
		tokenInfo := job.tokenInfo
		result := tokenProcessResult{token: tokenInfo.Token}
		
		// Check if token already exists
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", tokenInfo.Token)
		if err != nil || t.TokenID == "" {
			// Token doesn't exist, proceed to handle it
			dir := util.GetRandString()
			if err := util.CreateDir(dir); err != nil {
				result.err = fmt.Errorf("failed to create directory: %v", err)
				results <- result
				continue
			}
			defer os.RemoveAll(dir)

			// Get the token from IPFS
			if err := w.Get(tokenInfo.Token, did, OwnerRole, dir); err != nil {
				result.err = fmt.Errorf("failed to get token %s: %v", tokenInfo.Token, err)
				results <- result
				continue
			}

			// Get parent token details
			var parentTokenID string
			gb := w.GetGenesisTokenBlock(tokenInfo.Token, tokenInfo.TokenType)
			if gb != nil {
				parentTokenID, _, _ = gb.GetParentDetials(tokenInfo.Token)
			}

			// Create new token entry
			t = Token{
				TokenID:       tokenInfo.Token,
				TokenValue:    tokenInfo.TokenValue,
				ParentTokenID: parentTokenID,
				DID:           tokenInfo.OwnerDID,
			}

			err = w.s.Write(TokenStorage, &t)
			if err != nil {
				result.err = fmt.Errorf("failed to write token %s to db: %v", tokenInfo.Token, err)
				results <- result
				continue
			}
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
			result.err = fmt.Errorf("failed to update token %s in db: %v", tokenInfo.Token, err)
			results <- result
			continue
		}
		
		senderAddress := senderPeerId + "." + b.GetSenderDID()
		receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
		
		// Pin the token (skip AddProviderDetails as we'll batch them later)
		_, err = w.Pin(tokenInfo.Token, role, did, b.GetTid(), senderAddress, receiverAddress, tokenInfo.TokenValue, true)
		if err != nil {
			result.err = fmt.Errorf("failed to pin token %s: %v", tokenInfo.Token, err)
			results <- result
			continue
		}
		
		results <- result
	}
}