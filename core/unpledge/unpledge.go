package unpledge

import (
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"

	//tkn "github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	PledgePeriodInSeconds int =  7 * 24 * 60 * 60

	UnpledgeQueueTable string = "unpledgequeue"
)

type UnPledge struct {
	s       storage.Storage
	testNet bool
	w       *wallet.Wallet
	cb      UnpledgeCBType
	log     logger.Logger
}

type UnpledgeQueueInfo struct {
	TransactionID string `gorm:"column:tx_id;primaryKey"`
	PledgeTokens  string `gorm:"column:pledge_tokens"`
	Epoch         int64  `gorm:"column:epoch"`
	QuorumDID     string `gorm:"column:quorum_did"`
}

type UnpledgeCBType func(transactionID string, pledgeTokens string, quorumDID string) ([]*PledgeInformation, error)
type ReceiverOwnershipFunc func(transactionId string, receiverPeer string, receiverDID string, quorumDID string) (bool, error)

type TransTokenBlock struct {
	TokenID       string `json:"token_id"`
	TransferBlock string `json:"token_block"`
}

type PledgeUnpledgeBlock struct {
	TokenID       string `json:"token_id"`
	PledgeBlock   string `json:"pledge_block"`
	UnpledgeBlock string `json:"unpledge_block"`
}

type PledgeInformation struct {
	TokenID         string `json:"token_id"`
	TokenType       int    `json:"token_type"`
	PledgeBlockID   string `json:"pledge_block_id"`
	UnpledgeBlockID string `json:"unpledge_block_id"`
	QuorumDID       string `json:"quorum_did"`
	TransactionID   string `json:"transaction_id"`
}

func InitUnPledge(s storage.Storage, w *wallet.Wallet, testNet bool, log logger.Logger) (*UnPledge, error) {
	up := &UnPledge{
		s:       s,
		testNet: testNet,
		w:       w,
		log:     log.Named("unpledge"),
	}
	err := up.s.Init(UnpledgeQueueTable, UnpledgeQueueInfo{}, true)
	if err != nil {
		up.log.Error("failed to init unpledge token list table", "err", err)
		return nil, err
	}

	return up, nil
}

func (up *UnPledge) AddUnPledge(txId string, pledgeTokens string, epoch int64, quorumDID string) error {
	var unpledgeQueueInfo UnpledgeQueueInfo
	err := up.s.Read(UnpledgeQueueTable, &unpledgeQueueInfo, "tx_id = ?", txId)
	if err == nil {
		up.log.Error("Tokens are already in the unpledge list")
		return err
	}

	unpledgeQueueInfo.TransactionID = txId
	unpledgeQueueInfo.PledgeTokens = pledgeTokens
	unpledgeQueueInfo.Epoch = epoch
	unpledgeQueueInfo.QuorumDID = quorumDID

	err = up.s.Write(UnpledgeQueueTable, &unpledgeQueueInfo)
	if err != nil {
		up.log.Error("Error adding tx "+txId+" to unpledge list", "err", err)
		return err
	}

	return nil
}

