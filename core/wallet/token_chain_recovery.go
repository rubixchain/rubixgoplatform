package wallet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

const (
	// TokenDirName is the directory name for tokens
	TokenDirName = "Tokens"
	// TokenChainFileName is the file name for token chain
	TokenChainFileName = "tokenchain.json"
)

// TokenChainBlock represents a simple token chain block for recovery
type TokenChainBlock struct {
	TokenID       string    `json:"token_id"`
	BlockNumber   int       `json:"block_number"`
	BlockHash     string    `json:"block_hash"`
	PrevBlockHash string    `json:"prev_block_hash"`
	TransactionID string    `json:"transaction_id"`
	SenderDID     string    `json:"sender_did"`
	ReceiverDID   string    `json:"receiver_did"`
	TokenValue    float64   `json:"token_value"`
	Timestamp     time.Time `json:"timestamp"`
}

// GetTokenChainHeight returns the current block height of a token chain
func (w *Wallet) GetTokenChainHeight(tokenID string) (int, error) {
	w.l.Lock()
	defer w.l.Unlock()

	// Read token chain file from IPFS or local storage
	// For simplicity, we'll use a placeholder path
	chainFile := filepath.Join("/tmp", "tokens", tokenID, TokenChainFileName)
	data, err := ioutil.ReadFile(chainFile)
	if err != nil {
		return 0, fmt.Errorf("token chain not found: %v", err)
	}

	var blocks []map[string]interface{}
	err = json.Unmarshal(data, &blocks)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token chain: %v", err)
	}

	return len(blocks), nil
}

// GetCompleteTokenChain returns the complete token chain for a token
func (w *Wallet) GetCompleteTokenChain(tokenID string) ([]interface{}, error) {
	w.l.Lock()
	defer w.l.Unlock()

	// Read token chain file from IPFS or local storage
	// For simplicity, we'll use a placeholder path
	chainFile := filepath.Join("/tmp", "tokens", tokenID, TokenChainFileName)
	data, err := ioutil.ReadFile(chainFile)
	if err != nil {
		return nil, fmt.Errorf("token chain not found: %v", err)
	}

	var blocks []map[string]interface{}
	err = json.Unmarshal(data, &blocks)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token chain: %v", err)
	}

	// Convert to interface slice
	var result []interface{}
	for _, block := range blocks {
		result = append(result, block)
	}

	return result, nil
}

// AddTokenChainBlock adds a new block to the token chain
func (w *Wallet) AddTokenChainBlock(blockData interface{}) error {
	w.l.Lock()
	defer w.l.Unlock()

	// Parse block data as map
	bm, ok := blockData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid block data format")
	}

	// Extract token ID
	tokenID, ok := bm["token_id"].(string)
	if !ok || tokenID == "" {
		// Try alternate key
		if tid, ok := bm["tokenID"].(string); ok {
			tokenID = tid
		} else {
			return fmt.Errorf("token ID is required")
		}
	}

	// Read existing chain from local storage
	tokenDir := filepath.Join("/tmp", "tokens", tokenID)
	
	// Ensure directory exists
	err := os.MkdirAll(tokenDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create token directory: %v", err)
	}
	
	chainFile := filepath.Join(tokenDir, TokenChainFileName)
	var blocks []map[string]interface{}
	
	data, err := ioutil.ReadFile(chainFile)
	if err == nil {
		err = json.Unmarshal(data, &blocks)
		if err != nil {
			w.log.Warn("Failed to parse existing chain, starting new",
				"token_id", tokenID,
				"error", err)
			blocks = []map[string]interface{}{}
		}
	} else {
		// New chain
		blocks = []map[string]interface{}{}
	}

	// Check for duplicate block
	blockNum := 0
	if bn, ok := bm["block_number"].(float64); ok {
		blockNum = int(bn)
	} else if bn, ok := bm["block_number"].(int); ok {
		blockNum = bn
	}

	for _, existing := range blocks {
		existingNum := 0
		if en, ok := existing["block_number"].(float64); ok {
			existingNum = int(en)
		} else if en, ok := existing["block_number"].(int); ok {
			existingNum = en
		}
		
		if existingNum == blockNum {
			w.log.Debug("Block already exists, skipping",
				"token_id", tokenID,
				"block_number", blockNum)
			return nil
		}
	}

	// Add new block
	blocks = append(blocks, bm)

	// Write back to file
	updatedData, err := json.MarshalIndent(blocks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal blocks: %v", err)
	}

	err = ioutil.WriteFile(chainFile, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write token chain: %v", err)
	}

	w.log.Info("Token chain block added successfully",
		"token_id", tokenID,
		"block_number", blockNum)

	return nil
}

// RecoverTokenChain attempts to recover a broken token chain
func (w *Wallet) RecoverTokenChain(tokenID string, missingBlocks []interface{}) error {
	w.log.Info("Recovering token chain",
		"token_id", tokenID,
		"missing_blocks", len(missingBlocks))

	for _, block := range missingBlocks {
		err := w.AddTokenChainBlock(block)
		if err != nil {
			w.log.Error("Failed to add recovered block",
				"token_id", tokenID,
				"error", err)
			return err
		}
	}

	w.log.Info("Token chain recovery completed",
		"token_id", tokenID,
		"blocks_recovered", len(missingBlocks))

	return nil
}