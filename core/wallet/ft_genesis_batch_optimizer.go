package wallet

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// GenesisBatchOptimizer efficiently batches genesis lookups for FT tokens
type GenesisBatchOptimizer struct {
	w              *Wallet
	log            logger.Logger
	creatorCache   sync.Map // genesisID -> creatorDID
	genesisCache   sync.Map // tokenID -> genesisID
	lookupCount    int64
	cacheHitCount  int64
}

// NewGenesisBatchOptimizer creates a new optimizer
func NewGenesisBatchOptimizer(w *Wallet) *GenesisBatchOptimizer {
	return &GenesisBatchOptimizer{
		w:   w,
		log: w.log.Named("GenesisBatchOptimizer"),
	}
}

// BatchProcessTokens processes tokens in batches, grouping by likely genesis
func (gbo *GenesisBatchOptimizer) BatchProcessTokens(tokens []contract.TokenInfo) map[string]string {
	startTime := time.Now()
	tokenToCreator := make(map[string]string)
	
	// Group tokens by patterns (most FTs have sequential or similar token IDs)
	tokenGroups := gbo.groupTokensByPattern(tokens)
	
	gbo.log.Info("Batching tokens for genesis lookup",
		"total_tokens", len(tokens),
		"unique_groups", len(tokenGroups))
	
	// Process each group
	for groupKey, group := range tokenGroups {
		// Try to find one valid genesis in the group
		var creatorDID string
		var genesisID string
		found := false
		
		// Check cache first
		if cachedCreator, ok := gbo.creatorCache.Load(groupKey); ok {
			creatorDID = cachedCreator.(string)
			found = true
			gbo.cacheHitCount++
		} else {
			// Sample a few tokens from the group to find genesis
			samplesToCheck := min(3, len(group)) // Check at most 3 tokens
			
			for i := 0; i < samplesToCheck && !found; i++ {
				token := group[i]
				
				// Check if we already know the genesis for this token
				if cachedGenesis, ok := gbo.genesisCache.Load(token.Token); ok {
					genesisID = cachedGenesis.(string)
					if cachedCreator, ok := gbo.creatorCache.Load(genesisID); ok {
						creatorDID = cachedCreator.(string)
						found = true
						gbo.cacheHitCount++
						break
					}
				}
				
				// If not in cache, do the lookup
				genesis, creator := gbo.lookupGenesisAndCreator(token)
				if creator != "" {
					genesisID = genesis
					creatorDID = creator
					found = true
					
					// Cache the results
					gbo.genesisCache.Store(token.Token, genesisID)
					gbo.creatorCache.Store(genesisID, creatorDID)
					gbo.creatorCache.Store(groupKey, creatorDID)
					break
				}
			}
		}
		
		// Apply the found creator to all tokens in the group
		if found && creatorDID != "" {
			for _, token := range group {
				tokenToCreator[token.Token] = creatorDID
				// Also cache the genesis mapping
				if genesisID != "" {
					gbo.genesisCache.Store(token.Token, genesisID)
				}
			}
		} else {
			gbo.log.Warn("Could not find creator for token group",
				"group", groupKey,
				"token_count", len(group))
		}
	}
	
	duration := time.Since(startTime)
	gbo.log.Info("Genesis batch processing complete",
		"duration", duration,
		"tokens_processed", len(tokens),
		"cache_hits", gbo.cacheHitCount,
		"lookups", gbo.lookupCount,
		"cache_hit_rate", fmt.Sprintf("%.2f%%", float64(gbo.cacheHitCount)/float64(gbo.cacheHitCount+gbo.lookupCount)*100))
	
	return tokenToCreator
}

// groupTokensByPattern groups tokens that likely share the same genesis
func (gbo *GenesisBatchOptimizer) groupTokensByPattern(tokens []contract.TokenInfo) map[string][]contract.TokenInfo {
	groups := make(map[string][]contract.TokenInfo)
	
	for _, token := range tokens {
		// For FT tokens, they often have patterns like:
		// - Same prefix (first 10 chars might indicate same series)
		// - Similar token values (same denomination)
		// - Same token type
		
		// Create a group key based on token characteristics
		groupKey := gbo.getGroupKey(token)
		groups[groupKey] = append(groups[groupKey], token)
	}
	
	return groups
}

// getGroupKey creates a key to group similar tokens
func (gbo *GenesisBatchOptimizer) getGroupKey(token contract.TokenInfo) string {
	// Use token prefix and type as grouping key
	// Most FT tokens from same series have similar prefixes
	prefix := token.Token
	if len(prefix) > 10 {
		prefix = prefix[:10]
	}
	
	return fmt.Sprintf("%s-%d-%.2f", prefix, token.TokenType, token.TokenValue)
}

// lookupGenesisAndCreator performs actual genesis lookup
func (gbo *GenesisBatchOptimizer) lookupGenesisAndCreator(token contract.TokenInfo) (string, string) {
	gbo.lookupCount++
	
	// Get genesis block
	genesisBlock := gbo.w.GetGenesisTokenBlock(token.Token, token.TokenType)
	if genesisBlock == nil {
		return "", ""
	}
	
	// Get parent/genesis ID
	genesisID := token.Token
	parentID, _, err := genesisBlock.GetParentDetials(token.Token)
	if err == nil && parentID != "" {
		genesisID = parentID
	}
	
	// Get creator from genesis block
	// For FT tokens, the creator is the owner from the genesis block
	creator := genesisBlock.GetOwner()
	if creator != "" {
		return genesisID, creator
	}
	
	return genesisID, ""
}

// GetCreatorForToken gets creator for a specific token using cache
func (gbo *GenesisBatchOptimizer) GetCreatorForToken(tokenID string) string {
	// Check if we have direct mapping
	if genesisID, ok := gbo.genesisCache.Load(tokenID); ok {
		if creator, ok := gbo.creatorCache.Load(genesisID.(string)); ok {
			return creator.(string)
		}
	}
	return ""
}

// PreloadFromDownloads preloads creator info from downloaded token data
func (gbo *GenesisBatchOptimizer) PreloadFromDownloads(downloads []DownloadResult) {
	for _, dl := range downloads {
		if dl.Success && dl.CreatorDID != "" {
			// Cache by token ID
			gbo.creatorCache.Store(dl.Task.Token.Token, dl.CreatorDID)
			
			// Also cache by group pattern
			groupKey := gbo.getGroupKey(dl.Task.Token)
			gbo.creatorCache.Store(groupKey, dl.CreatorDID)
		}
	}
}