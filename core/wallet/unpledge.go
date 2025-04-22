package wallet

import (
	"fmt"
	"strings"
)

// Unpledging info associated with PoW Pledging
type migration_UnpledgeQueueInfo struct {
	Token string `gorm:"column:token"`
}

// Unpledging info associated with Periodic Pledging
type UnpledgeSequenceInfo struct {
	TransactionID string `gorm:"column:tx_id;primaryKey"`
	PledgeTokens  string `gorm:"column:pledge_tokens"`
	Epoch         int64  `gorm:"column:epoch"`
	QuorumDID     string `gorm:"column:quorum_did"`
}

// Token sync info associated with Background Syncing of tokens
type TokenSyncInfo struct {
	TokenID      string `gorm:"column:token_id;primaryKey"`
	TokenType    int64  `gorm:"column:token_type"`
	SyncFromPeer string `gorm:"column:sync_from_peer"`
	BlockNumber  int64  `gorm:"column:block_number"`
	BlockID      string `gorm:"column:block_id"`
	Status       string `gorm:"column:status"`
}

// Methods specific to Periodic Pledging
func (w *Wallet) GetUnpledgeSequenceDetails() ([]*UnpledgeSequenceInfo, error) {
	var unpledgeSequenceDetails []*UnpledgeSequenceInfo
	err := w.s.Read(UnpledgeSequence, &unpledgeSequenceDetails, "tx_id != ?", "")
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []*UnpledgeSequenceInfo{}, nil
		} else {
			w.log.Error("Failed to get token states", "err", err)
			return nil, err
		}
	}
	return unpledgeSequenceDetails, nil
}

func (w *Wallet) GetUnpledgeSequenceInfoByTransactionID(transactionID string) (*UnpledgeSequenceInfo, error) {
	var unpledgeSequenceInfo *UnpledgeSequenceInfo
	err := w.s.Read(UnpledgeSequence, &unpledgeSequenceInfo, "tx_id = ?", transactionID)
	if err != nil {
		return nil, err
	}
	return unpledgeSequenceInfo, nil
}

func (w *Wallet) AddUnpledgeSequenceInfo(unpledgeSequenceInfo *UnpledgeSequenceInfo) error {
	err := w.s.Write(UnpledgeSequence, &unpledgeSequenceInfo)
	if err != nil {
		errMsg := fmt.Errorf("error while adding unpledging sequence info for transaction: %v, err: %v", unpledgeSequenceInfo.TransactionID, err)
		w.log.Error(errMsg.Error())
		return err
	}

	return nil
}

func (w *Wallet) RemoveUnpledgeSequenceInfo(transactionID string) error {
	err := w.s.Delete(UnpledgeSequence, &UnpledgeSequenceInfo{}, "tx_id = ?", transactionID)
	if err != nil {
		errMsg := fmt.Errorf("error while removing unpledging sequence info for transaction: %v, err: %v", transactionID, err)
		w.log.Error(errMsg.Error())
		return err
	}

	return nil
}

// Methods specific to migration from PoW based pledging
func (w *Wallet) Migration_GetUnpledgeQueueInfo() ([]migration_UnpledgeQueueInfo, error) {
	var unpledgeQueueInfo []migration_UnpledgeQueueInfo

	err := w.s.Read(UnpledgeQueueTable, &unpledgeQueueInfo, "token != ?", "")
	if err != nil {
		if !strings.Contains(err.Error(), "no records found") {
			if strings.Contains(err.Error(), "no such table") {
				w.log.Info("no PoW based pledged tokens left to unpledge")
				return nil, nil
			} else {
				errMsg := fmt.Errorf("unable to read to pledge tokens from unpledgequeue table: %v", err)
				w.log.Error(errMsg.Error())
				return nil, errMsg
			}
		}
	}

	return unpledgeQueueInfo, nil
}

func (w *Wallet) Migration_DropUnpledgeQueueTable() error {
	err := w.s.Drop(UnpledgeQueueTable, &migration_UnpledgeQueueInfo{})
	if err != nil {
		errMsg := fmt.Errorf("failed to drop the unpledgequeue table: %v", err.Error())
		w.log.Error(errMsg.Error())
		return errMsg
	}

	return nil
}
