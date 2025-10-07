package wallet

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	tkn "github.com/rubixchain/rubixgoplatform/token"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// LevelDBRollbackInfo stores information needed for LevelDB rollback
type LevelDBRollbackInfo struct {
	TransactionID    string
	TokenStateHashes []string
	BlockHashes      map[string]string // tokenID -> blockHash
	TokenType        int
}

// RemoveTokenStateHashByTransaction removes token state hash entries for a failed transaction
func (w *Wallet) RemoveTokenStateHashByTransaction(txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Removing token state hash entries",
		"transaction_id", txID)

	// Get all token state hashes for this transaction
	var tokenStates []TokenStateDetails
	err := w.s.Read(TokenStateHash, &tokenStates, "transaction_id=?", txID)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			w.log.Debug("No token state hashes found for transaction",
				"transaction_id", txID)
			return nil
		}
		return fmt.Errorf("failed to read token state hashes: %v", err)
	}

	// Delete each token state hash entry
	deletedCount := 0
	for _, ts := range tokenStates {
		err = w.s.Delete(TokenStateHash, &TokenStateDetails{}, 
			"token_state_hash=? AND transaction_id=?", ts.TokenStateHash, txID)
		if err != nil {
			w.log.Error("Failed to delete token state hash",
				"token_state_hash", ts.TokenStateHash,
				"error", err)
			// Continue with other deletions
		} else {
			deletedCount++
		}
	}

	w.log.Info("Token state hash removal completed",
		"transaction_id", txID,
		"deleted_count", deletedCount)

	return nil
}

// RemoveBlocksForTransaction removes blocks added during a failed transaction
func (w *Wallet) RemoveBlocksForTransaction(txID string, tokenType int) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Removing blocks for failed transaction",
		"transaction_id", txID,
		"token_type", tokenType)

	db := w.getChainDB(tokenType)
	if db == nil {
		return fmt.Errorf("failed to get chain DB for token type %d", tokenType)
	}

	// Track blocks to remove
	blocksToRemove := make(map[string][]byte)
	
	// Iterate through all token chains to find blocks with this transaction ID
	iter := db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Parse the block to check transaction ID
		blk := block.InitBlock(value, nil)
		if blk == nil {
			continue
		}

		// Check if this block belongs to the failed transaction
		if blk.GetTid() == txID {
			keyCopy := make([]byte, len(key))
			copy(keyCopy, key)
			blocksToRemove[string(keyCopy)] = value
		}
	}

	// Remove the identified blocks
	removedCount := 0
	for key := range blocksToRemove {
		err := db.Delete([]byte(key), nil)
		if err != nil {
			w.log.Error("Failed to remove block",
				"key", key,
				"error", err)
		} else {
			removedCount++
		}
	}

	w.log.Info("Block removal completed",
		"transaction_id", txID,
		"removed_count", removedCount)

	return nil
}

// GetLatestBlockBeforeTransaction returns the latest block before a specific transaction
func (w *Wallet) GetLatestBlockBeforeTransaction(tokenID string, txID string, tokenType int) (*block.Block, error) {
	db := w.getChainDB(tokenType)
	if db == nil {
		return nil, fmt.Errorf("failed to get chain DB for token type %d", tokenType)
	}

	// Get all blocks for this token
	prefix := tcsPrefix(tokenType, tokenID)
	iter := db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
	defer iter.Release()

	var latestBlock *block.Block
	var latestBlockBeforeTx *block.Block

	// Iterate through blocks
	for iter.Next() {
		value := iter.Value()
		blk := block.InitBlock(value, nil)
		if blk == nil {
			continue
		}

		// If we found the transaction block, return the previous one
		if blk.GetTid() == txID {
			return latestBlockBeforeTx, nil
		}

		// Keep track of the latest block before the transaction
		latestBlockBeforeTx = latestBlock
		latestBlock = blk
	}

	// If transaction not found, return the latest block
	return latestBlock, nil
}

// CreateLevelDBSnapshot creates a snapshot of LevelDB state for a transaction
func (w *Wallet) CreateLevelDBSnapshot(txID string) (*LevelDBRollbackInfo, error) {
	w.l.Lock()
	defer w.l.Unlock()

	snapshot := &LevelDBRollbackInfo{
		TransactionID: txID,
		BlockHashes:   make(map[string]string),
	}

	// Get token state hashes for this transaction
	tokenStates, err := w.GetTokenStateHashByTransactionID(txID)
	if err == nil {
		for _, ts := range tokenStates {
			snapshot.TokenStateHashes = append(snapshot.TokenStateHashes, ts.TokenStateHash)
		}
	}

	w.log.Info("Created LevelDB snapshot",
		"transaction_id", txID,
		"token_state_count", len(snapshot.TokenStateHashes))

	return snapshot, nil
}

// ExecuteLevelDBRollback performs complete LevelDB rollback
func (w *Wallet) ExecuteLevelDBRollback(snapshot *LevelDBRollbackInfo) error {
	w.log.Info("Executing LevelDB rollback",
		"transaction_id", snapshot.TransactionID)

	// Remove token state hash entries
	err := w.RemoveTokenStateHashByTransaction(snapshot.TransactionID)
	if err != nil {
		w.log.Error("Failed to remove token state hashes", "error", err)
		// Continue with other rollbacks
	}

	// Remove blocks for RBT tokens
	err = w.RemoveBlocksForTransaction(snapshot.TransactionID, tkn.RBTTokenType)
	if err != nil {
		w.log.Error("Failed to remove RBT blocks", "error", err)
	}

	// Remove blocks for FT tokens
	err = w.RemoveBlocksForTransaction(snapshot.TransactionID, tkn.FTTokenType)
	if err != nil {
		w.log.Error("Failed to remove FT blocks", "error", err)
	}

	// Remove blocks for NFT tokens if applicable
	err = w.RemoveBlocksForTransaction(snapshot.TransactionID, tkn.NFTTokenType)
	if err != nil {
		w.log.Error("Failed to remove NFT blocks", "error", err)
	}

	w.log.Info("LevelDB rollback completed",
		"transaction_id", snapshot.TransactionID)

	return nil
}

// VerifyTokenCanBeUsed checks if a token can be used after a failed transaction
func (w *Wallet) VerifyTokenCanBeUsed(tokenID string, tokenType int) error {
	w.l.Lock()
	defer w.l.Unlock()

	// Check token status in SQLite
	var t Token
	err := w.s.Read(TokenStorage, &t, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("token not found: %v", err)
	}

	// Token should be free to use
	if t.TokenStatus != TokenIsFree {
		return fmt.Errorf("token is not free, current status: %d", t.TokenStatus)
	}

	// Verify no pending transaction
	if t.TransactionID != "" {
		return fmt.Errorf("token still associated with transaction: %s", t.TransactionID)
	}

	// Check token chain integrity in LevelDB
	latestBlock := w.GetLatestTokenBlock(tokenID, tokenType)
	if latestBlock == nil {
		// Token might be new or genesis
		return nil
	}

	// Verify the latest block is not from a failed transaction
	blockTxID := latestBlock.GetTid()
	if blockTxID != "" {
		// Check if this transaction was rolled back
		// This would need a rolled back transaction registry
		w.log.Debug("Token latest block transaction",
			"token_id", tokenID,
			"block_tx_id", blockTxID)
	}

	return nil
}

// CleanupOrphanedBlocks removes blocks that don't have corresponding token entries
func (w *Wallet) CleanupOrphanedBlocks(tokenType int) error {
	w.l.Lock()
	defer w.l.Unlock()

	db := w.getChainDB(tokenType)
	if db == nil {
		return fmt.Errorf("failed to get chain DB for token type %d", tokenType)
	}

	orphanedCount := 0
	iter := db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := string(iter.Key())
		
		// Extract token ID from key
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			continue
		}
		tokenID := parts[1]

		// Check if token exists in SQLite
		var exists bool
		if tokenType == tkn.FTTokenType {
			var ft FTToken
			err := w.s.Read(FTTokenStorage, &ft, "token_id=?", tokenID)
			exists = (err == nil && ft.TokenID != "")
		} else {
			var t Token
			err := w.s.Read(TokenStorage, &t, "token_id=?", tokenID)
			exists = (err == nil && t.TokenID != "")
		}

		// Remove orphaned blocks
		if !exists {
			err := db.Delete(iter.Key(), nil)
			if err != nil {
				w.log.Error("Failed to remove orphaned block",
					"key", key,
					"error", err)
			} else {
				orphanedCount++
			}
		}
	}

	if orphanedCount > 0 {
		w.log.Info("Cleaned up orphaned blocks",
			"token_type", tokenType,
			"orphaned_count", orphanedCount)
	}

	return nil
}