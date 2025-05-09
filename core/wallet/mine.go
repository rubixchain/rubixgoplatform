package wallet

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

type MiningRecord struct {
	MiningID     string `gorm:"column:mining_id"`
	MinedTokenID string `gorm:"column:mined_token_id"`
	MinerDID     string `gorm:"column:miner_did"`
	TokenLevel   int    `gorm:"column:token_level"`
	TokenNumber  int    `gorm:"column:token_number"`
}

type CreditsDetailsMapValue struct {
	MiningID         string `json:"miningID"`
	RemainingCredits uint64 `json:"remainingCredits"`
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

func CreatePledgeHistoryMap(creditDetails []model.PledgeHistory, miningTokenID, minerDID string) (map[string]interface{}, error) {
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

		value := CreditsDetailsMapValue{
			MiningID:         miningTokenID,
			RemainingCredits: pledge.RemainingCredits,
		}

		// Add the struct directly to the result map
		result[key] = value
	}

	return result, nil
}
