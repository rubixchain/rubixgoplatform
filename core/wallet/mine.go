package wallet

import "fmt"

type MiningRecord struct {
	MiningID     string `gorm:"column:mining_id"`
	MinedTokenID string `gorm:"column:mined_token_id"`
	MinerDID     string `gorm:"column:miner_did"`
	TokenLevel   int    `gorm:"column:token_level"`
	TokenNumber  int    `gorm:"column:token_number"`
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
