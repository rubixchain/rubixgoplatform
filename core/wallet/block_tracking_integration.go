package wallet

import (
	"fmt"
	"sync"

	"github.com/rubixchain/rubixgoplatform/block"
	tkn "github.com/rubixchain/rubixgoplatform/token"
	ut "github.com/rubixchain/rubixgoplatform/util"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// WalletWithTracking extends wallet with block tracking capability
type WalletWithTracking struct {
	*Wallet
	blockTracker    *TransactionBlockTracker
	trackerMutex    sync.RWMutex
	currentTxID     string // Current transaction being processed
}

// EnableBlockTracking adds block tracking to the wallet
func (w *Wallet) EnableBlockTracking() *WalletWithTracking {
	return &WalletWithTracking{
		Wallet:       w,
		blockTracker: NewTransactionBlockTracker(),
	}
}

// SetCurrentTransaction sets the current transaction ID for tracking
func (wt *WalletWithTracking) SetCurrentTransaction(txID string) {
	wt.trackerMutex.Lock()
	defer wt.trackerMutex.Unlock()
	wt.currentTxID = txID
}

// ClearCurrentTransaction clears the current transaction ID
func (wt *WalletWithTracking) ClearCurrentTransaction() {
	wt.trackerMutex.Lock()
	defer wt.trackerMutex.Unlock()
	wt.currentTxID = ""
}

// getCurrentTransaction gets the current transaction ID safely
func (wt *WalletWithTracking) getCurrentTransaction() string {
	wt.trackerMutex.RLock()
	defer wt.trackerMutex.RUnlock()
	return wt.currentTxID
}

// addBlockWithTracking wraps the original addBlock with tracking
func (wt *WalletWithTracking) addBlockWithTracking(token string, b *block.Block) error {
	opt := &opt.WriteOptions{
		Sync: true,
	}
	tt := b.GetTokenType(token)
	db := wt.getChainDB(tt)
	if db == nil {
		wt.log.Error("Failed to add block, invalid token type")
		return fmt.Errorf("failed to get db")
	}

	bid, err := b.GetBlockID(token)
	if err != nil {
		return err
	}
	key := tcsKey(tt, token, bid)
	
	// Get block number and hash for tracking
	bn, err := b.GetBlockNumber(token)
	if err != nil {
		wt.log.Error("Failed to get block number", "err", err)
		return err
	}
	
	blockHash, err := b.GetHash()
	if err != nil {
		wt.log.Error("Failed to get block hash", "err", err)
		return err
	}

	// Perform the actual block addition (existing logic)
	lb := wt.getLatestBlock(tt, token)
	
	// First block check block number start with zero
	if lb == nil {
		if bn != 0 {
			wt.log.Error("Invalid block number, expect 0", "bn", bn)
			return fmt.Errorf("invalid block number")
		}
	} else {
		lbn, err := lb.GetBlockNumber(token)
		if err != nil {
			wt.log.Error("Failed to get block number", "err", err)
			return err
		}

		if bn <= lbn {
			if bn == lbn {
				// Block already exists, check if it's the same block
				existingBlock := wt.getLatestBlock(tt, token)
				if existingBlock != nil {
					existingHash, _ := existingBlock.GetHash()
					if existingHash == blockHash {
						wt.log.Debug("Block already exists with same hash, skipping",
							"token", token,
							"block_number", bn,
							"block_hash", blockHash)
						return nil
					} else {
						return fmt.Errorf("block already exist with different hash")
					}
				}
			}
			return fmt.Errorf("attempting to add older block: current=%d, new=%d", lbn, bn)
		}
		if bn != (lbn + 1) {
			wt.log.Error("Invalid block number sequence", "expected", lbn+1, "got", bn)
			return fmt.Errorf("invalid block number, expect %d but got %d", lbn+1, bn)
		}
	}

	// Add the block
	var writeErr error
	if b.CheckMultiTokenBlock() {
		bs, err := b.GetHash()
		if err != nil {
			return err
		}
		hs := ut.HexToStr(ut.CalculateHash(b.GetBlock(), "SHA3-256"))
		refkey := []byte(ReferenceType + "-" + hs + "-" + bs)
		_, err = wt.getRawBlock(db, refkey)
		// Write only if reference block not exist
		if err != nil {
			db.l.Lock()
			err = db.Put(refkey, b.GetBlock(), opt)
			db.l.Unlock()
			if err != nil {
				return err
			}
		}
		db.l.Lock()
		writeErr = db.Put([]byte(key), refkey, opt)
		db.l.Unlock()
	} else {
		db.l.Lock()
		writeErr = db.Put([]byte(key), b.GetBlock(), opt)
		if tt == tkn.TestTokenType {
			wt.log.Debug("Written", "key", key)
		}
		db.l.Unlock()
	}

	// If write was successful and we have a current transaction, track it
	if writeErr == nil {
		txID := wt.getCurrentTransaction()
		if txID == "" {
			// Try to get transaction ID from the block itself
			txID = b.GetTid()
		}
		
		if txID != "" {
			wt.blockTracker.RecordBlockAddition(
				txID,
				token,
				int(bn),
				blockHash,
				key,
				tt,
			)
			
			wt.log.Debug("Tracked block addition",
				"transaction_id", txID,
				"token", token,
				"block_number", bn,
				"key", key)
		}
	}

	return writeErr
}

// AddTokenBlock with tracking
func (wt *WalletWithTracking) AddTokenBlock(token string, b *block.Block) error {
	return wt.addBlockWithTracking(token, b)
}

// CreateTokenBlock with tracking
func (wt *WalletWithTracking) CreateTokenBlock(b *block.Block) error {
	err := wt.addBlocks(b)
	if err != nil {
		return err
	}

	// Track all tokens in the block
	txID := wt.getCurrentTransaction()
	if txID == "" {
		txID = b.GetTid()
	}

	if txID != "" {
		tokens := b.GetTransTokens()
		for _, token := range tokens {
			bn, _ := b.GetBlockNumber(token)
			tt := b.GetTokenType(token)
			bid, _ := b.GetBlockID(token)
			key := tcsKey(tt, token, bid)
			blockHash, _ := b.GetHash()
			
			wt.blockTracker.RecordBlockAddition(
				txID,
				token,
				int(bn),
				blockHash,
				key,
				tt,
			)
		}
	}

	return nil
}

// BatchAddTokenBlocks with tracking
func (wt *WalletWithTracking) BatchAddTokenBlocks(pairs []struct {
	Token     string
	Block     *block.Block
	TokenType int
}) error {
	// Track transaction ID for the batch
	var txID string
	if len(pairs) > 0 && pairs[0].Block != nil {
		txID = wt.getCurrentTransaction()
		if txID == "" {
			txID = pairs[0].Block.GetTid()
		}
	}

	// Perform batch addition
	err := wt.Wallet.BatchAddTokenBlocks(pairs)
	if err != nil {
		return err
	}

	// Track all successful additions
	if txID != "" {
		for _, pair := range pairs {
			bn, _ := pair.Block.GetBlockNumber(pair.Token)
			tt := pair.Block.GetTokenType(pair.Token)
			bid, _ := pair.Block.GetBlockID(pair.Token)
			key := tcsKey(tt, pair.Token, bid)
			blockHash, _ := pair.Block.GetHash()
			
			wt.blockTracker.RecordBlockAddition(
				txID,
				pair.Token,
				int(bn),
				blockHash,
				key,
				tt,
			)
		}
	}

	return nil
}

// CreateRollbackSnapshot with block tracking
func (wt *WalletWithTracking) CreateRollbackSnapshot(txID string, role string) (*RollbackStateInfo, error) {
	// Call original snapshot creation
	snapshot, err := wt.Wallet.CreateRollbackSnapshot(txID, role)
	if err != nil {
		return nil, err
	}

	// Add tracked blocks to snapshot
	trackedBlocks := wt.blockTracker.GetBlocksForTransaction(txID)
	if len(trackedBlocks) > 0 {
		levelDBSnapshot := &LevelDBRollbackInfo{
			TransactionID: txID,
			TokenType:     -1, // Mixed types
		}
		
		for _, block := range trackedBlocks {
			// Store block keys for rollback
			snapshot.BlockHashes = append(snapshot.BlockHashes, block.Key)
		}
		
		snapshot.LevelDBSnapshot = levelDBSnapshot
	}

	return snapshot, nil
}

// ExecuteSafeRollback performs rollback using tracked blocks
func (wt *WalletWithTracking) ExecuteSafeRollback(snapshot *RollbackStateInfo) error {
	// Execute SQLite rollback first
	err := wt.Wallet.ExecuteRollback(snapshot)
	if err != nil {
		wt.log.Error("Failed to execute base rollback", "error", err)
	}

	// Execute safe LevelDB rollback using tracked blocks
	if snapshot.TransactionID != "" {
		safeRollback := wt.Wallet.NewSafeLevelDBRollback(wt.blockTracker)
		err = safeRollback.RemoveTrackedBlocks(snapshot.TransactionID)
		if err != nil {
			wt.log.Error("Failed to execute safe LevelDB rollback", "error", err)
			return err
		}
	}

	return nil
}

// VerifyTokenAfterRollback verifies a token can be used after rollback
func (wt *WalletWithTracking) VerifyTokenAfterRollback(tokenID string, tokenType int) error {
	// First verify using base wallet
	err := wt.Wallet.VerifyTokenCanBeUsed(tokenID, tokenType)
	if err != nil {
		return err
	}

	// Additional verification for chain integrity
	err = wt.Wallet.VerifyTokenChainIntegrity(tokenID, tokenType)
	if err != nil {
		return fmt.Errorf("token chain integrity check failed: %v", err)
	}

	return nil
}