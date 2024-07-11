package wallet

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	SendMode int = iota
	RecvMode
	DeployMode
	ExecuteMode
)



func (w *Wallet) AddTransactionHistory(td *model.TransactionDetails) error {
	err := w.s.Write(TransactionStorage, td)
	if err != nil {
		w.log.Error("Failed to store transaction history", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetTransactionDetailsbyTransactionId(transactionId string) (model.TransactionDetails, error) {
	var th model.TransactionDetails
	//var tt []w.TokensTransferred
	//var ql []w.QuorumList
	err := w.s.Read(TransactionStorage, &th, "transaction_id=?", transactionId)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return th, err
	}
	return th, nil
}

func (w *Wallet) GetTransactionByComment(comment string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "comment=?", comment)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return nil, err
	}
	return td, err
}

func (w *Wallet) GetTransactionByReceiver(receiver string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "receiver_did=?", receiver)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as Receiver ", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionBySender(sender string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "sender_did=?", sender)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as sender", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionByDID(did string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "sender_did=? OR receiver_did=?", did, did)
	if err != nil {
		w.log.Error("Failed to get transaction details with did", did, "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionByDIDAndDateRange(did string, startDate time.Time, endDate time.Time) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails
	err := w.s.Read(TransactionStorage, &td, "date_time >= ? AND date_time <= ? AND sender_did=? OR receiver_did=?", startDate, endDate, did, did)
	if err != nil {
		w.log.Error("Failed to get transaction details with did and date range", did, startDate, endDate, "err", err)
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
