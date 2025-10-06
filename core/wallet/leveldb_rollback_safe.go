package wallet

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// BlockAdditionRecord tracks blocks added during a transaction
type BlockAdditionRecord struct {
	TokenID      string    `json:"token_id"`
	BlockNumber  int       `json:"block_number"`
	BlockHash    string    `json:"block_hash"`
	Key          string    `json:"key"`
	AddedAt      time.Time `json:"added_at"`
	TokenType    int       `json:"token_type"`
}

// TransactionBlockTracker tracks all blocks added during a transaction
type TransactionBlockTracker struct {
	mu            sync.RWMutex
	transactions  map[string][]BlockAdditionRecord // txID -> blocks added
}

// NewTransactionBlockTracker creates a new block tracker
func NewTransactionBlockTracker() *TransactionBlockTracker {
	return &TransactionBlockTracker{
		transactions: make(map[string][]BlockAdditionRecord),
	}
}

// RecordBlockAddition records a block that was added during a transaction
func (tbt *TransactionBlockTracker) RecordBlockAddition(txID string, tokenID string, blockNumber int, blockHash string, key string, tokenType int) {
	tbt.mu.Lock()
	defer tbt.mu.Unlock()

	record := BlockAdditionRecord{
		TokenID:     tokenID,
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
		Key:         key,
		AddedAt:     time.Now(),
		TokenType:   tokenType,
	}

	tbt.transactions[txID] = append(tbt.transactions[txID], record)
}

// GetBlocksForTransaction returns all blocks added for a specific transaction
func (tbt *TransactionBlockTracker) GetBlocksForTransaction(txID string) []BlockAdditionRecord {
	tbt.mu.RLock()
	defer tbt.mu.RUnlock()

	records := tbt.transactions[txID]
	// Return a copy to prevent external modification
	result := make([]BlockAdditionRecord, len(records))
	copy(result, records)
	return result
}

// ClearTransaction removes tracking for a completed/rolled back transaction
func (tbt *TransactionBlockTracker) ClearTransaction(txID string) {
	tbt.mu.Lock()
	defer tbt.mu.Unlock()

	delete(tbt.transactions, txID)
}

// SafeLevelDBRollback performs safe rollback using tracked block additions
type SafeLevelDBRollback struct {
	w            *Wallet
	blockTracker *TransactionBlockTracker
}

// NewSafeLevelDBRollback creates a new safe rollback handler
func (w *Wallet) NewSafeLevelDBRollback(tracker *TransactionBlockTracker) *SafeLevelDBRollback {
	return &SafeLevelDBRollback{
		w:            w,
		blockTracker: tracker,
	}
}

// RemoveTrackedBlocks removes only the blocks that were tracked for this transaction
func (slr *SafeLevelDBRollback) RemoveTrackedBlocks(txID string) error {
	blocks := slr.blockTracker.GetBlocksForTransaction(txID)
	if len(blocks) == 0 {
		slr.w.log.Info("No tracked blocks to remove",
			"transaction_id", txID)
		return nil
	}

	slr.w.log.Info("Removing tracked blocks",
		"transaction_id", txID,
		"block_count", len(blocks))

	removedCount := 0
	failedCount := 0

	// Group blocks by token type for efficient removal
	blocksByType := make(map[int][]BlockAdditionRecord)
	for _, blockRecord := range blocks {
		blocksByType[blockRecord.TokenType] = append(blocksByType[blockRecord.TokenType], blockRecord)
	}

	// Remove blocks for each token type
	for tokenType, typeBlocks := range blocksByType {
		db := slr.w.getChainDB(tokenType)
		if db == nil {
			slr.w.log.Error("Failed to get chain DB",
				"token_type", tokenType)
			failedCount += len(typeBlocks)
			continue
		}

		for _, blockRecord := range typeBlocks {
			// Verify the block still exists and matches our record
			value, err := db.Get([]byte(blockRecord.Key), nil)
			if err != nil {
				slr.w.log.Debug("Block already removed or doesn't exist",
					"key", blockRecord.Key,
					"error", err)
				continue
			}

			// Parse and verify it's the same block we added
			blk := block.InitBlock(value, nil)
			if blk == nil {
				slr.w.log.Error("Failed to parse block for verification",
					"key", blockRecord.Key,
					"error", "InitBlock returned nil")
				failedCount++
				continue
			}

			// Verify block hash matches
			currentHash, err := blk.GetHash()
			if err != nil {
				slr.w.log.Error("Failed to get block hash",
					"key", blockRecord.Key,
					"error", err)
				failedCount++
				continue
			}
			if currentHash != blockRecord.BlockHash {
				slr.w.log.Error("Block hash mismatch, not removing",
					"key", blockRecord.Key,
					"expected_hash", blockRecord.BlockHash,
					"current_hash", currentHash)
				failedCount++
				continue
			}

			// Safe to remove - it's the exact block we added
			err = db.Delete([]byte(blockRecord.Key), nil)
			if err != nil {
				slr.w.log.Error("Failed to remove block",
					"key", blockRecord.Key,
					"error", err)
				failedCount++
			} else {
				removedCount++
				slr.w.log.Debug("Successfully removed block",
					"key", blockRecord.Key,
					"token_id", blockRecord.TokenID)
			}
		}
	}

	// Clear the tracking records
	slr.blockTracker.ClearTransaction(txID)

	slr.w.log.Info("Block removal completed",
		"transaction_id", txID,
		"removed_count", removedCount,
		"failed_count", failedCount)

	if failedCount > 0 {
		return fmt.Errorf("failed to remove %d blocks out of %d", failedCount, len(blocks))
	}

	return nil
}

// SafeRemoveTokenStateHash removes only token state hashes we can verify belong to this transaction
func (w *Wallet) SafeRemoveTokenStateHash(txID string, expectedHashes []string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Safely removing token state hashes",
		"transaction_id", txID,
		"expected_count", len(expectedHashes))

	// Create a map for quick lookup
	expectedMap := make(map[string]bool)
	for _, hash := range expectedHashes {
		expectedMap[hash] = true
	}

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

	// Only delete the ones we expect
	deletedCount := 0
	skippedCount := 0

	for _, ts := range tokenStates {
		if expectedMap[ts.TokenStateHash] {
			// This is one we added, safe to remove
			err = w.s.Delete(TokenStateHash, &TokenStateDetails{}, 
				"token_state_hash=? AND transaction_id=?", ts.TokenStateHash, txID)
			if err != nil {
				w.log.Error("Failed to delete expected token state hash",
					"token_state_hash", ts.TokenStateHash,
					"error", err)
			} else {
				deletedCount++
			}
		} else {
			// This wasn't in our expected list, don't remove
			w.log.Warn("Skipping unexpected token state hash",
				"token_state_hash", ts.TokenStateHash,
				"transaction_id", txID)
			skippedCount++
		}
	}

	w.log.Info("Token state hash removal completed",
		"transaction_id", txID,
		"deleted_count", deletedCount,
		"skipped_count", skippedCount)

	return nil
}

// VerifyTokenChainIntegrity verifies the token chain is valid after rollback
func (w *Wallet) VerifyTokenChainIntegrity(tokenID string, tokenType int) error {
	db := w.getChainDB(tokenType)
	if db == nil {
		return fmt.Errorf("failed to get chain DB for token type %d", tokenType)
	}

	prefix := tcsPrefix(tokenType, tokenID)
	iter := db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
	defer iter.Release()

	expectedBlockNum := 0
	var prevBlock *block.Block

	for iter.Next() {
		value := iter.Value()
		blk := block.InitBlock(value, nil)
		if blk == nil {
			return fmt.Errorf("failed to parse block")
		}

		// Verify block number sequence
		blockNum, err := blk.GetBlockNumber(tokenID)
		if err != nil {
			return fmt.Errorf("failed to get block number: %v", err)
		}
		if int(blockNum) != expectedBlockNum {
			return fmt.Errorf("block number mismatch: expected %d, got %d", expectedBlockNum, blockNum)
		}

		// Verify previous block hash (except for genesis)
		if expectedBlockNum > 0 && prevBlock != nil {
			// Get previous block ID from current block's parent reference
			prevBlockID, err := blk.GetPrevBlockID(tokenID)
			if err == nil && prevBlockID != "" {
				// Verify it matches the actual previous block
				actualPrevBlockID, _ := prevBlock.GetBlockID(tokenID)
				if prevBlockID != actualPrevBlockID {
					return fmt.Errorf("previous block ID mismatch at block %d", blockNum)
				}
			}
		}

		prevBlock = blk
		expectedBlockNum++
	}

	return nil
}

// CreateSafeLevelDBSnapshot creates a snapshot with exact tracking
func (w *Wallet) CreateSafeLevelDBSnapshot(txID string, tracker *TransactionBlockTracker) (*SafeLevelDBSnapshot, error) {
	w.l.Lock()
	defer w.l.Unlock()

	snapshot := &SafeLevelDBSnapshot{
		TransactionID: txID,
		Timestamp:     time.Now(),
		TrackedBlocks: tracker.GetBlocksForTransaction(txID),
	}

	// Get token state hashes that we're about to add
	tokenStates, err := w.GetTokenStateHashByTransactionID(txID)
	if err == nil {
		for _, ts := range tokenStates {
			snapshot.TokenStateHashes = append(snapshot.TokenStateHashes, ts.TokenStateHash)
		}
	}

	w.log.Info("Created safe LevelDB snapshot",
		"transaction_id", txID,
		"tracked_blocks", len(snapshot.TrackedBlocks),
		"token_states", len(snapshot.TokenStateHashes))

	return snapshot, nil
}

// SafeLevelDBSnapshot contains exact tracking of what was added
type SafeLevelDBSnapshot struct {
	TransactionID    string                 `json:"transaction_id"`
	Timestamp        time.Time              `json:"timestamp"`
	TrackedBlocks    []BlockAdditionRecord  `json:"tracked_blocks"`
	TokenStateHashes []string               `json:"token_state_hashes"`
}