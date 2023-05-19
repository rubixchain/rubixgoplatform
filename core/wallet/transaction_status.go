package wallet

import (
	"github.com/rubixchain/rubixgoplatform/contract"
)

type TxnDetails struct {
	SenderPeerId   string
	SenderDID      string
	ReceiverPeerId string
	ReceiverDID    string
	Tokens         []string
	QuorumList     []string
}

type TransactionStatusMap struct {
	Token          string `gorm:"column:token;primaryKey"`
	ReqID          string `gorm:"column:req_id"`
	TxnID          string `gorm:"column:txn_id"`
	SenderDID      string `gorm:"column:sender_did"`
	SenderPeerID   string `gorm:"column:sender_peerid"`
	ReceiverDID    string `gorm:"column:receiver_did"`
	ReceiverPeerID string `gorm:"column:receiver_peerid"`
	TxnStatus      int    `gorm:"column:txn_status"`
	QuorumList     string `gorm:"column:quorum_list"`
}

const (
	ConsensusStart int = iota
	ConsensusFinished
	ConsensusSuccess
	ConsensusFailed
	ReceiverValidation
	FinlaityPending
	FinalityAchieved
)

// method to add txn status details to transaction status storage table
func (w *Wallet) AddTxnStatus(txnStatusDetails TransactionStatusMap) error {
	err := w.s.Read(TransactionStatusStorage, &txnStatusDetails, "token=?", txnStatusDetails.Token)
	if err != nil || txnStatusDetails.Token == "" {
		return w.s.Write(TransactionStatusStorage, &txnStatusDetails)
	}
	return nil
}

// method to update txn status details,
func (w *Wallet) UpdateTxnStatus(tokenInfo []contract.TokenInfo, txnStatus int, txnId string) error {
	for i := range tokenInfo {
		var transactionStatusDetail TransactionStatusMap
		err := w.s.Read(TransactionStatusStorage, &transactionStatusDetail, "txn_id=? AND token=?", txnId, tokenInfo[i].Token)
		if err != nil {
			w.log.Error("Error in UpdateTxnStatus read ", err)
			return err
		}
		w.log.Debug("token ", transactionStatusDetail.Token, "status ", transactionStatusDetail.TxnStatus)
		transactionStatusDetail.TxnStatus = txnStatus
		w.log.Debug("token ", transactionStatusDetail.Token, "status ", transactionStatusDetail.TxnStatus)
		err = w.s.Update(TransactionStatusStorage, &transactionStatusDetail, "txn_id=? AND token=?", txnId, tokenInfo[i].Token)
		if err != nil {
			w.log.Error("Error in UpdateTxnStatus update ", err)
			return err
		}
	}
	return nil
}

func (w *Wallet) GetFinalityPendingTxn() ([]TransactionStatusMap, error) {
	var result []TransactionStatusMap
	err := w.s.Read(TransactionStatusStorage, &result, "txn_status=?", FinlaityPending)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (w *Wallet) GetTxnDetails(txnId string) ([]TransactionStatusMap, error) {
	var result []TransactionStatusMap
	err := w.s.Read(TransactionStatusStorage, &result, "txn_id=?", txnId)
	if err != nil {
		return nil, err
	}
	return result, nil
}
