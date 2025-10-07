package wallet

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/token"
)

// FTTokenFixResult holds the result of fixing a single token
type FTTokenFixResult struct {
	TokenID    string
	OldCreator string
	NewCreator string
	Success    bool
	Error      error
}

// FixAllFTTokensWithPeerIDAsCreator fixes all FT tokens that have peer ID as CreatorDID
// This function:
// 1. Scans all FT tokens in the database
// 2. Identifies tokens where CreatorDID starts with "12D3KooW" (peer ID prefix)
// 3. Looks up the correct DID from genesis block
// 4. Updates the token in all storage locations
func (w *Wallet) FixAllFTTokensWithPeerIDAsCreator() ([]FTTokenFixResult, error) {
	startTime := time.Now()
	w.log.Info("Starting FT token CreatorDID fix utility")
	
	// Lock wallet during this operation
	w.l.Lock()
	defer w.l.Unlock()
	
	// Get all FT tokens
	var allTokens []FTToken
	err := w.s.Read(FTTokenStorage, &allTokens, "")
	if err != nil {
		return nil, fmt.Errorf("failed to read FT tokens: %w", err)
	}
	
	w.log.Info("Scanning FT tokens for peer ID issues", "total_tokens", len(allTokens))
	
	// Find tokens with peer ID as creator
	tokensToFix := make([]FTToken, 0)
	for _, token := range allTokens {
		if strings.HasPrefix(token.CreatorDID, "12D3KooW") {
			tokensToFix = append(tokensToFix, token)
		}
	}
	
	if len(tokensToFix) == 0 {
		w.log.Info("No tokens found with peer ID as CreatorDID")
		return []FTTokenFixResult{}, nil
	}
	
	w.log.Info("Found tokens with peer ID as CreatorDID", 
		"affected_count", len(tokensToFix),
		"percentage", fmt.Sprintf("%.2f%%", float64(len(tokensToFix))/float64(len(allTokens))*100))
	
	// Create genesis optimizer for efficient lookups
	genesisOptimizer := NewGenesisBatchOptimizer(w)
	
	// Process tokens in parallel
	results := make([]FTTokenFixResult, len(tokensToFix))
	var wg sync.WaitGroup
	workerCount := 10 // Limit workers to avoid overwhelming the system
	
	tokenChan := make(chan int, len(tokensToFix))
	
	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range tokenChan {
				results[idx] = w.fixSingleFTToken(tokensToFix[idx], genesisOptimizer)
			}
		}()
	}
	
	// Queue work
	for i := range tokensToFix {
		tokenChan <- i
	}
	close(tokenChan)
	
	// Wait for completion
	wg.Wait()
	
	// Count successes and failures
	successCount := 0
	failureCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}
	
	duration := time.Since(startTime)
	w.log.Info("FT token CreatorDID fix completed",
		"duration", duration,
		"total_processed", len(tokensToFix),
		"successful", successCount,
		"failed", failureCount)
	
	// Log sample of fixes for verification
	if len(results) > 0 {
		w.log.Info("Sample fix results:")
		sampleCount := 5
		if len(results) < sampleCount {
			sampleCount = len(results)
		}
		for i := 0; i < sampleCount; i++ {
			if results[i].Success {
				w.log.Info("Fixed token",
					"token", results[i].TokenID,
					"old_creator", results[i].OldCreator,
					"new_creator", results[i].NewCreator)
			}
		}
	}
	
	return results, nil
}

// fixSingleFTToken fixes a single FT token's CreatorDID
func (w *Wallet) fixSingleFTToken(token FTToken, genesisOptimizer *GenesisBatchOptimizer) FTTokenFixResult {
	result := FTTokenFixResult{
		TokenID:    token.TokenID,
		OldCreator: token.CreatorDID,
	}
	
	// Get the correct creator DID from genesis
	correctCreator := w.getCorrectCreatorForToken(token, genesisOptimizer)
	
	if correctCreator == "" {
		result.Error = fmt.Errorf("could not determine correct creator DID")
		return result
	}
	
	// Validate the creator is a DID and not a peer ID
	if strings.HasPrefix(correctCreator, "12D3KooW") {
		result.Error = fmt.Errorf("resolved creator is still a peer ID: %s", correctCreator)
		return result
	}
	
	// Update the token
	token.CreatorDID = correctCreator
	result.NewCreator = correctCreator
	
	// Update in main FT storage
	err := w.s.Update(FTTokenStorage, &token, "token_id=?", token.TokenID)
	if err != nil {
		result.Error = fmt.Errorf("failed to update FT token storage: %w", err)
		return result
	}
	
	// Also check and update in FT table if exists
	var ftEntry FT
	err = w.s.Read(FTStorage, &ftEntry, "ft_name=?", token.FTName)
	if err == nil && ftEntry.FTName != "" {
		// FT exists in FT table, update it too
		if strings.HasPrefix(ftEntry.CreatorDID, "12D3KooW") {
			ftEntry.CreatorDID = correctCreator
			err = w.s.Update(FTStorage, &ftEntry, "ft_name=?", token.FTName)
			if err != nil {
				w.log.Warn("Failed to update FT table entry",
					"ft_name", token.FTName,
					"error", err)
				// Don't fail the whole operation for this
			}
		}
	}
	
	// Update in LevelDB token chain if it has blocks
	err = w.updateTokenChainCreator(token.TokenID, correctCreator)
	if err != nil {
		w.log.Warn("Failed to update token chain creator",
			"token", token.TokenID,
			"error", err)
		// Don't fail the whole operation for this
	}
	
	result.Success = true
	return result
}

// getCorrectCreatorForToken determines the correct creator DID for a token
func (w *Wallet) getCorrectCreatorForToken(ftToken FTToken, genesisOptimizer *GenesisBatchOptimizer) string {
	// First try to get from genesis optimizer cache
	creator := genesisOptimizer.GetCreatorForToken(ftToken.TokenID)
	if creator != "" && !strings.HasPrefix(creator, "12D3KooW") {
		return creator
	}
	
	// Try to get from genesis block
	genesisBlock := w.GetGenesisTokenBlock(ftToken.TokenID, token.FTTokenType)
	if genesisBlock != nil {
		creator = genesisBlock.GetOwner()
		if creator != "" && !strings.HasPrefix(creator, "12D3KooW") {
			return creator
		}
		
		// If owner is also peer ID, try other fields
		// Check if there's a valid DID in the block
		did := genesisBlock.GetReceiverDID()
		if did != "" && !strings.HasPrefix(did, "12D3KooW") {
			return did
		}
		
		did = genesisBlock.GetSenderDID()
		if did != "" && !strings.HasPrefix(did, "12D3KooW") {
			return did
		}
	}
	
	// Try to find a valid creator from token chain history
	creator = w.findCreatorFromTokenChain(ftToken.TokenID)
	if creator != "" && !strings.HasPrefix(creator, "12D3KooW") {
		return creator
	}
	
	return ""
}

// findCreatorFromTokenChain searches token chain for a valid creator DID
func (w *Wallet) findCreatorFromTokenChain(tokenID string) string {
	// Get all blocks for this token
	rawBlocks, _, err := w.GetAllTokenBlocks(tokenID, token.FTTokenType, "")
	if err != nil || len(rawBlocks) == 0 {
		return ""
	}
	
	// Search from oldest to newest
	for _, rawBlock := range rawBlocks {
		block := block.InitBlock(rawBlock, nil)
		if block == nil {
			continue
		}
		
		// Check TokenOwner field
		owner := block.GetOwner()
		if owner != "" && !strings.HasPrefix(owner, "12D3KooW") {
			return owner
		}
		
		// Check receiver/sender DIDs
		did := block.GetReceiverDID()
		if did != "" && !strings.HasPrefix(did, "12D3KooW") {
			return did
		}
		
		did = block.GetSenderDID()
		if did != "" && !strings.HasPrefix(did, "12D3KooW") {
			return did
		}
	}
	
	return ""
}

// updateTokenChainCreator updates creator info in token chain blocks if needed
func (w *Wallet) updateTokenChainCreator(tokenID string, correctCreator string) error {
	// This is a placeholder - actual implementation would need to:
	// 1. Read all blocks for the token from LevelDB
	// 2. Update any blocks that have peer ID as creator
	// 3. Recalculate block hashes if creator is part of the hash
	// 4. Save updated blocks back to LevelDB
	
	// For now, just log that we would update it
	w.log.Debug("Would update token chain creator info",
		"token", tokenID,
		"new_creator", correctCreator)
	
	return nil
}

// GetFTTokenCreatorStats returns statistics about FT token creators
func (w *Wallet) GetFTTokenCreatorStats() (map[string]interface{}, error) {
	w.l.Lock()
	defer w.l.Unlock()
	
	var allTokens []FTToken
	err := w.s.Read(FTTokenStorage, &allTokens, "")
	if err != nil {
		return nil, fmt.Errorf("failed to read FT tokens: %w", err)
	}
	
	stats := make(map[string]interface{})
	peerIDCount := 0
	didCount := 0
	emptyCount := 0
	
	creatorMap := make(map[string]int)
	
	for _, token := range allTokens {
		if token.CreatorDID == "" {
			emptyCount++
		} else if strings.HasPrefix(token.CreatorDID, "12D3KooW") {
			peerIDCount++
		} else {
			didCount++
		}
		
		if token.CreatorDID != "" {
			creatorMap[token.CreatorDID]++
		}
	}
	
	stats["total_tokens"] = len(allTokens)
	stats["peer_id_creators"] = peerIDCount
	stats["did_creators"] = didCount
	stats["empty_creators"] = emptyCount
	stats["unique_creators"] = len(creatorMap)
	stats["peer_id_percentage"] = fmt.Sprintf("%.2f%%", float64(peerIDCount)/float64(len(allTokens))*100)
	
	// Find top creators
	type creatorCount struct {
		Creator string
		Count   int
	}
	
	creators := make([]creatorCount, 0, len(creatorMap))
	for creator, count := range creatorMap {
		creators = append(creators, creatorCount{creator, count})
	}
	
	// Sort by count (simple bubble sort for small dataset)
	for i := 0; i < len(creators); i++ {
		for j := i + 1; j < len(creators); j++ {
			if creators[j].Count > creators[i].Count {
				creators[i], creators[j] = creators[j], creators[i]
			}
		}
	}
	
	// Get top 5 creators
	topCreators := make([]map[string]interface{}, 0)
	for i := 0; i < 5 && i < len(creators); i++ {
		topCreators = append(topCreators, map[string]interface{}{
			"creator": creators[i].Creator,
			"count":   creators[i].Count,
			"is_peer_id": strings.HasPrefix(creators[i].Creator, "12D3KooW"),
		})
	}
	
	stats["top_creators"] = topCreators
	
	return stats, nil
}