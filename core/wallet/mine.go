package wallet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	tkn "github.com/rubixchain/rubixgoplatform/token"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const MiningChainBlockCountLimit = 100

type PledgeHistoryRecord struct {
	QuorumDID          string  `gorm:"column:quorum_did"`
	TransactionID      string  `gorm:"column:transaction_id"`
	TransactionType    int     `gorm:"column:transaction_type"`
	TransferTokenID    string  `gorm:"column:transfer_tokens_id"`
	TransferTokenType  int     `gorm:"column:transfer_tokens_type"`
	TransferTokenValue float64 `gorm:"column:transfer_token_value"`
	TransferBlockID    string  `gorm:"column:transfer_block_id"`
	Epoch              uint64  `gorm:"column:epoch"`
	NextBlockEpoch     uint64  `gorm:"column:next_epoch"`
	TokenCredit        uint64  `gorm:"column:token_credit"`
	TokenCreditStatus  int     `gorm:"column:token_credit_status"`
}

type MiningRecord struct {
	MiningID     string `gorm:"column:mining_id"`
	MinedTokenID string `gorm:"column:mined_token_id"`
	MinerDID     string `gorm:"column:miner_did"`
	TokenLevel   int    `gorm:"column:token_level"`
	TokenNumber  uint64 `gorm:"column:token_number"`
	Epoch        uint64 `gorm:"column:epoch"`
}

type CreditsDetailsMapValue struct {
	MiningID         string `json:"miningID"`
	RemainingCredits uint64 `json:"remainingCredits"`
}

func (w *Wallet) ConvertPledgeHistoryToRecord(pledgeHistories []model.PledgeHistory) []PledgeHistoryRecord {
	records := make([]PledgeHistoryRecord, 0, len(pledgeHistories))

	for _, ph := range pledgeHistories {
		record := PledgeHistoryRecord{
			QuorumDID:          ph.QuorumDID,
			TransactionID:      ph.TransactionID,
			TransactionType:    ph.TransactionType,
			TransferTokenID:    ph.TransferTokenID,
			TransferTokenType:  ph.TransferTokenType,
			TransferTokenValue: ph.TransferTokenValue,
			TransferBlockID:    ph.TransferBlockID,
			Epoch:              ph.Epoch,
			NextBlockEpoch:     ph.NextBlockEpoch,
			TokenCredit:        ph.TokenCredit,
			TokenCreditStatus:  ph.TokenCreditStatus,
		}
		records = append(records, record)
	}

	return records
}

// ConvertSinglePledgeHistoryToRecord converts a single model.PledgeHistory to a wallet.PledgeHistoryRecord
func (w *Wallet) ConvertSinglePledgeHistoryToRecord(pledgeHistory model.PledgeHistory) PledgeHistoryRecord {
	return PledgeHistoryRecord{
		QuorumDID:          pledgeHistory.QuorumDID,
		TransactionID:      pledgeHistory.TransactionID,
		TransactionType:    pledgeHistory.TransactionType,
		TransferTokenID:    pledgeHistory.TransferTokenID,
		TransferTokenType:  pledgeHistory.TransferTokenType,
		TransferTokenValue: pledgeHistory.TransferTokenValue,
		TransferBlockID:    pledgeHistory.TransferBlockID,
		Epoch:              pledgeHistory.Epoch,
		NextBlockEpoch:     pledgeHistory.NextBlockEpoch,
		TokenCredit:        pledgeHistory.TokenCredit,
		TokenCreditStatus:  pledgeHistory.TokenCreditStatus,
	}
}

func (w *Wallet) ConvertPledgeHistoryRecordToModel(records []PledgeHistoryRecord) []model.PledgeHistory {
	pledgeHistories := make([]model.PledgeHistory, 0, len(records))

	for _, record := range records {
		pledgeHistory := model.PledgeHistory{
			QuorumDID:          record.QuorumDID,
			TransactionID:      record.TransactionID,
			TransactionType:    record.TransactionType,
			TransferTokenID:    record.TransferTokenID,
			TransferTokenType:  record.TransferTokenType,
			TransferTokenValue: record.TransferTokenValue,
			TransferBlockID:    record.TransferBlockID,
			Epoch:              record.Epoch,
			NextBlockEpoch:     record.NextBlockEpoch,
			TokenCredit:        record.TokenCredit,
			TokenCreditStatus:  record.TokenCreditStatus,
			// RemainingCredits is not set explicitly; will default to 0
		}
		pledgeHistories = append(pledgeHistories, pledgeHistory)
	}

	return pledgeHistories
}

// ConvertSinglePledgeHistoryRecordToModel converts a single wallet.PledgeHistoryRecord to a model.PledgeHistory
func (w *Wallet) ConvertSinglePledgeHistoryRecordToModel(record PledgeHistoryRecord) model.PledgeHistory {
	return model.PledgeHistory{
		QuorumDID:          record.QuorumDID,
		TransactionID:      record.TransactionID,
		TransactionType:    record.TransactionType,
		TransferTokenID:    record.TransferTokenID,
		TransferTokenType:  record.TransferTokenType,
		TransferTokenValue: record.TransferTokenValue,
		TransferBlockID:    record.TransferBlockID,
		Epoch:              record.Epoch,
		NextBlockEpoch:     record.NextBlockEpoch,
		TokenCredit:        record.TokenCredit,
		TokenCreditStatus:  record.TokenCreditStatus,
		// RemainingCredits is not set explicitly; will default to 0
	}
}

func (w *Wallet) AddMiningRecords(miningRecord MiningRecord) error {
	err := w.s.Write(MiningRecordsTable, miningRecord)
	if err != nil {
		w.log.Error("failed to write mining records to mining records table")
	}
	return nil
}

func (w *Wallet) FindLatestTokenLevelAndNumber() (MiningRecord, error) {
	var records []MiningRecord
	var result MiningRecord

	// Fetch all records ordered by token_level and token_number
	err := w.s.Read(MiningRecordsTable, &records, "ORDER BY token_level DESC, token_number DESC")
	if err != nil {
		w.log.Error("Failed to read mining records", "error", err)
		return result, err
	}
	// Return the first record if available
	if len(records) > 0 {
		result = records[0]
	} else {
		return result, fmt.Errorf("no mining records found")
	}
	return result, nil
}

func (w *Wallet) CreatePledgeHistoryMap(creditDetails []model.PledgeHistory, miningTokenID, minerDID string) (map[string]interface{}, error) {
	if miningTokenID == "" || minerDID == "" {
		return nil, fmt.Errorf("miningTokenID and minerDID must not be empty")
	}

	result := make(map[string]interface{})
	for _, pledge := range creditDetails {
		// Create the key by joining TransactionID, TransferTokenID, and minerDID with hyphens
		keyParts := []string{pledge.TransactionID, pledge.TransferTokenID, minerDID}
		for _, part := range keyParts {
			if part == "" {
				return nil, fmt.Errorf("invalid pledge history: TransactionID, TransferTokenID, or minerDID is empty")
			}
		}
		key := strings.Join(keyParts, "-")

		// Store only RemainingCredits as the value
		result[key] = pledge.RemainingCredits

		w.log.Debug("Created pledge history entry", "key", key, "remainingCredits", pledge.RemainingCredits)
	}

	return result, nil
}

// getLatestBlock get latest block from the storage
func (w *Wallet) GetLatestMiningChainBlock() (*block.MiningChain, error) {
	tt := tkn.MiningChainType
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to get DB, invalid token type")
		return nil, fmt.Errorf("Failed to get DB, invalid token type")
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(w.MiningChainKeyPrefix(block.GetMiningChainID()))), nil)
	defer iter.Release()
	if iter.Last() {
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		b := block.InitMiningBlock(blk, nil)
		return b, nil
	}
	return nil, fmt.Errorf("failed to get mining chain latest block")
}

func (w *Wallet) AddMiningChainBlock(miningChain *block.MiningChain) error {
	opt := &opt.WriteOptions{
		Sync: true,
	}
	tt := tkn.MiningChainType
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to add chain block, invalid token type")
		return fmt.Errorf("failed to get db")
	}

	// Get the latest block number for mining chain key
	// latestBlock, err := w.GetLatestMiningChainBlock()
	// var nextBlockNumber uint64

	// if err != nil || latestBlock == nil {
	// 	w.log.Warn("No existing mining chain block found, adding the first block")
	// 	nextBlockNumber = 1
	// } else {
	// 	latestBlockNumber, err := latestBlock.GetMiningChainBlockNumber()
	// 	if err != nil {
	// 		w.log.Error("Failed to get latest mining chain block number")
	// 		return fmt.Errorf("failed to get latest mining chain block number")
	// 	}
	// 	nextBlockNumber = latestBlockNumber + 1
	// }

	blockNumber, err := miningChain.GetMiningChainBlockNumber()
	if err != nil {
		w.log.Error("Failed to get mining chain block number")
		return fmt.Errorf("failed to get mining chain block number")
	}
	key := w.MiningChainKey(block.GetMiningChainID(), blockNumber)

	db.l.Lock()
	err = db.Put([]byte(key), miningChain.GetMiningBlock(), opt)
	db.l.Unlock()

	if err != nil {
		w.log.Error("Failed to write mining chain block", "err", err)
		return err
	}
	w.log.Info("Mining chain block added successfully", "key", key)
	return nil
}

func (w *Wallet) MiningChainKeyPrefix(token string) string {
	return fmt.Sprintf("mt-%s-", token)
}

func (w *Wallet) MiningChainKey(token string, blockNumber uint64) string {
	return fmt.Sprintf("mt-%s-%010d", token, blockNumber)
}

func (w *Wallet) GetAllMiningChainBlocks(token string, startBlockNumber uint64) ([][]byte, uint64, error) {
	tt := tkn.MiningChainType
	db := w.getChainDB(tt)
	if db == nil {
		return nil, 0, fmt.Errorf("failed to get DB, invalid token type")
	}
	prefix := []byte(w.MiningChainKeyPrefix(token))
	iter := db.NewIterator(util.BytesPrefix(prefix), nil)
	defer iter.Release()

	blks := make([][]byte, 0)
	var nextBlockNumber uint64
	count := 0

	if startBlockNumber != 0 {
		startKey := []byte(w.MiningChainKey(token, startBlockNumber))
		if !iter.Seek(startKey) {
			return nil, 0, nil // Start block not found, return empty result
		}
	} else {
		if !iter.First() {
			return nil, 0, nil // No blocks exist
		}
	}

	for iter.Valid() && count < MiningChainBlockCountLimit {
		key := string(iter.Key())
		blockNumberStr := key[len(key)-10:]
		blockNumber, err := strconv.ParseUint(blockNumberStr, 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid block number in key: %s", key)
		}
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		blks = append(blks, blk)
		count++
		nextBlockNumber = blockNumber + 1
		iter.Next()
	}

	if !iter.Valid() {
		nextBlockNumber = 0 // No more blocks
	}

	return blks, nextBlockNumber, nil
}
