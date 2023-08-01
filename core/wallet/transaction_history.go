package wallet

import (
	"time"
)

const (
	SendMode int = iota
	RecvMode
	DeployMode
	ExecuteMode
)

type TransactionDetails struct {
	TransactionID   string    `gorm:"column:transaction_id;primaryKey"`
	TransactionType string    `gorm:"column:transaction_type"`
	BlockID         string    `gorm:"column:block_id"`
	Mode            int       `gorm:"column:mode"`
	SenderDID       string    `gorm:"column:sender_did"`
	ReceiverDID     string    `gorm:"column:receiver_did"`
	Amount          float64   `gorm:"column:amount"`
	TotalTime       float64   `gorm:"column:total_time"`
	Comment         string    `gorm:"column:comment"`
	DateTime        time.Time `gorm:"column:date_time"`
	Status          bool      `gorm:"column:status"`
	DeployerDID     string    `gorm:"column:deployer_did"`
}

func (w *Wallet) AddTransactionHistory(td *TransactionDetails) error {
	err := w.s.Write(TransactionStorage, td)
	if err != nil {
		w.log.Error("Failed to store transaction history", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetTransactionDetailsbyTransactionId(transactionId string) (TransactionDetails, error) {
	var th TransactionDetails
	//var tt []w.TokensTransferred
	//var ql []w.QuorumList
	err := w.s.Read(TransactionStorage, &th, "transaction_id=?", transactionId)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return th, err
	}
	return th, nil
}

func (w *Wallet) GetTransactionByComment(comment string) ([]TransactionDetails, error) {
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "comment=?", comment)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return nil, err
	}
	return td, err
}

func (w *Wallet) GetTransactionByReceiver(receiver string) ([]TransactionDetails, error) {
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "receiver_did=?", receiver)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionBySender(sender string) ([]TransactionDetails, error) {
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "sender_did=?", sender)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return nil, err
	}
	return td, nil
}

// func (w *Wallet) GetTransactionByDate(date string) ([]TransactionDetails, error) {
// 	var th []TransactionHistory
// 	var td []TransactionDetails

// 	err := w.s.Read(TransactionStorage, &th, "date_time=?", date)
// 	if err != nil {
// 		w.log.Error("Failed to get transaction id", "err", err)
// 		return nil, err
// 	}
// 	transactionId := th[0].TransactionID
// 	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
// 	return td, err

// }

// func (w *Wallet) GetTransactionByStatus(status bool) ([]TransactionDetails, error) {
// 	var th []TransactionHistory
// 	var td []TransactionDetails

// 	err := w.s.Read(TransactionStorage, &th, "transaction_status=?", status)
// 	if err != nil {
// 		w.log.Error("Failed to get transaction id", "err", err)
// 		return nil, err
// 	}
// 	transactionId := th[0].TransactionID
// 	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
// 	return td, err

// }
