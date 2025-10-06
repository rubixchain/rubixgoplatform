package wallet

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// GenesisGroupOptimizer optimizes token processing by grouping tokens with same genesis
type GenesisGroupOptimizer struct {
	w   *Wallet
	log logger.Logger
}

// TokenGenesisGroup represents tokens sharing the same genesis block
type TokenGenesisGroup struct {
	GenesisID    string
	GenesisBlock *block.Block
	Owner        string
	Tokens       []TokenWithIndex
	TokenType    int
}

// TokenWithIndex tracks token with its original index
type TokenWithIndex struct {
	Token contract.TokenInfo
	Index int
}

// NewGenesisGroupOptimizer creates a new genesis group optimizer
func NewGenesisGroupOptimizer(w *Wallet) *GenesisGroupOptimizer {
	return &GenesisGroupOptimizer{
		w:   w,
		log: w.log.Named("GenesisGroupOptimizer"),
	}
}

// GroupTokensByGenesis groups tokens by their genesis block to minimize lookups
func (ggo *GenesisGroupOptimizer) GroupTokensByGenesis(tokens []contract.TokenInfo) map[string]*TokenGenesisGroup {
	startTime := time.Now()
	groups := make(map[string]*TokenGenesisGroup)
	genesisCache := make(map[string]string) // token -> genesis mapping cache
	
	// First, we need to determine genesis for each token
	// This requires one lookup per unique token (not per token instance)
	uniqueTokens := make(map[string]contract.TokenInfo)
	for _, token := range tokens {
		uniqueTokens[token.Token] = token
	}
	
	ggo.log.Debug("Analyzing tokens for genesis grouping",
		"total_tokens", len(tokens),
		"unique_tokens", len(uniqueTokens))
	
	// Get genesis info for each unique token
	for tokenID, tokenInfo := range uniqueTokens {
		// Get the genesis block to find parent/genesis token
		genesisBlock := ggo.w.GetGenesisTokenBlock(tokenID, tokenInfo.TokenType)
		if genesisBlock == nil {
			ggo.log.Error("Failed to get genesis block", "token", tokenID)
			continue
		}
		
		// The genesis token is typically the parent token for FTs
		// or the token itself if it's the original token
		genesisTokenID := tokenID // Default to self
		
		// Check if this token has a parent (is a part token)
		parentID, _, err := genesisBlock.GetParentDetials(tokenID)
		if err == nil && parentID != "" {
			genesisTokenID = parentID
		}
		
		genesisCache[tokenID] = genesisTokenID
	}
	
	// Now group all tokens by their genesis
	for i, token := range tokens {
		genesisID, found := genesisCache[token.Token]
		if !found {
			ggo.log.Warn("Genesis not found for token", "token", token.Token)
			continue
		}
		
		groupKey := fmt.Sprintf("%s-%d", genesisID, token.TokenType)
		
		if group, exists := groups[groupKey]; exists {
			group.Tokens = append(group.Tokens, TokenWithIndex{
				Token: token,
				Index: i,
			})
		} else {
			groups[groupKey] = &TokenGenesisGroup{
				GenesisID: genesisID,
				TokenType: token.TokenType,
				Tokens: []TokenWithIndex{{
					Token: token,
					Index: i,
				}},
			}
		}
	}
	
	// Fetch genesis block data once per group
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for _, group := range groups {
		wg.Add(1)
		go func(g *TokenGenesisGroup) {
			defer wg.Done()
			
			// Get genesis block once for the entire group
			genesisBlock := ggo.w.GetGenesisTokenBlock(g.GenesisID, g.TokenType)
			if genesisBlock != nil {
				owner := genesisBlock.GetOwner()
				
				mu.Lock()
				g.GenesisBlock = genesisBlock
				g.Owner = owner
				mu.Unlock()
			}
		}(group)
	}
	
	wg.Wait()
	
	duration := time.Since(startTime)
	
	// Log statistics
	totalLookups := len(uniqueTokens) + len(groups) // Initial scan + one per group
	savedLookups := len(tokens) - totalLookups
	
	ggo.log.Info("Genesis grouping completed",
		"duration", duration,
		"total_tokens", len(tokens),
		"unique_genesis_blocks", len(groups),
		"genesis_lookups_saved", savedLookups,
		"savings_percent", fmt.Sprintf("%.1f%%", float64(savedLookups)/float64(len(tokens))*100))
	
	// Log group details
	for key, group := range groups {
		ggo.log.Debug("Genesis group",
			"key", key,
			"genesis_id", group.GenesisID,
			"owner", group.Owner,
			"token_count", len(group.Tokens))
	}
	
	return groups
}

// ProcessTokensWithGenesisGroups processes tokens using genesis grouping optimization
func (ggo *GenesisGroupOptimizer) ProcessTokensWithGenesisGroups(
	tokens []contract.TokenInfo,
	processFunc func(token contract.TokenInfo, owner string, genesisBlock *block.Block) error,
) error {
	
	// Group tokens by genesis
	groups := ggo.GroupTokensByGenesis(tokens)
	
	// Process each group
	var errors []error
	var errorMu sync.Mutex
	var wg sync.WaitGroup
	
	for _, group := range groups {
		// Process all tokens in the group with the same genesis data
		for _, tokenWithIndex := range group.Tokens {
			wg.Add(1)
			go func(twi TokenWithIndex, owner string, genesis *block.Block) {
				defer wg.Done()
				
				err := processFunc(twi.Token, owner, genesis)
				if err != nil {
					errorMu.Lock()
					errors = append(errors, fmt.Errorf("token %s: %w", twi.Token.Token, err))
					errorMu.Unlock()
				}
			}(tokenWithIndex, group.Owner, group.GenesisBlock)
		}
	}
	
	wg.Wait()
	
	if len(errors) > 0 {
		return fmt.Errorf("failed to process %d tokens: %v", len(errors), errors[0])
	}
	
	return nil
}

// GetOwnerForToken efficiently gets owner using genesis grouping
func (ggo *GenesisGroupOptimizer) GetOwnerForToken(token contract.TokenInfo, groups map[string]*TokenGenesisGroup) string {
	// Search through groups to find this token
	for _, group := range groups {
		for _, twi := range group.Tokens {
			if twi.Token.Token == token.Token && twi.Token.TokenType == token.TokenType {
				return group.Owner
			}
		}
	}
	
	// Fallback: do individual lookup
	ggo.log.Warn("Token not found in genesis groups, doing individual lookup", "token", token.Token)
	genesisBlock := ggo.w.GetGenesisTokenBlock(token.Token, token.TokenType)
	if genesisBlock != nil {
		return genesisBlock.GetOwner()
	}
	
	return ""
}