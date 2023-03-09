package wallet

import (
	"time"
)

const (
	SendMode int = iota
	RecvMode
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
}

func (w *Wallet) AddTransactionHistory(td *TransactionDetails) error {
	err := w.s.Write(TransactionStorage, td)
	if err != nil {
		w.log.Error("Failed to store transaction history", "err", err)
		return err
	}
	return nil
}

// func (w *Wallet) GetTransactionDetailsbyTransactionId(transactionId string) ([]TransactionDetails, error) {
// 	var th []TransactionHistory
// 	var tt []TokensTransferred
// 	var ql []QuorumList
// 	err := w.s.Read(TransactionStorage, &th, "transaction_id=?", transactionId)
// 	if err != nil {
// 		w.log.Error("Failed to get transaction details", "err", err)
// 		return nil, err
// 	}
// 	err = w.s.Read(TokensArrayStorage, &tt, "transaction_id=?", transactionId)
// 	if err != nil {
// 		w.log.Error("Failed to get tokens transferred", "err", err)
// 		return nil, err
// 	}
// 	err = w.s.Read(QuorumListStorage, &ql, "transaction_id=?", transactionId)
// 	if err != nil {
// 		w.log.Error("Failed to get quorum list", "err", err)
// 		return nil, err
// 	}
// 	var td []TransactionDetails
// 	for i := range th {
// 		var t TransactionDetails
// 		t.TransactionID = th[i].TransactionID
// 		t.SenderDID = th[i].SenderDID
// 		t.ReceiverDID = th[i].ReceiverDID
// 		t.Amount = th[i].Amount
// 		t.TotalTime = th[i].TotalTime
// 		t.Comment = th[i].Comment
// 		t.DateTime = th[i].DateTime
// 		for j := range tt {
// 			if tt[j].TransactionID == th[i].TransactionID {
// 				t.WholeTokens = append(t.WholeTokens, tt[i].Tokens)
// 			}
// 		}
// 		for l := range tt {
// 			if tt[l].TransactionID == th[i].TransactionID {
// 				t.PartTokens = append(t.PartTokens, tt[i].Tokens)
// 			}
// 		}
// 		for k := range ql {
// 			if ql[k].TransactionID == th[i].TransactionID {
// 				t.QuorumList = append(t.QuorumList, ql[k].QuorumList)
// 			}
// 		}
// 		td = append(td, t)
// 	}

// 	return td, nil
// }

// func (w *Wallet) GetTransactionByComment(comment string) ([]TransactionDetails, error) {
// 	var th []TransactionHistory
// 	var td []TransactionDetails

// 	err := w.s.Read(TransactionStorage, &th, "comment=?", comment)
// 	if err != nil {
// 		w.log.Error("Failed to get transaction id", "err", err)
// 		return nil, err
// 	}
// 	transactionId := th[0].TransactionID
// 	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
// 	return td, err

// }

// func (w *Wallet) GetTransactionByReceiver(receiver string) ([]TransactionDetails, error) {
// 	var th []TransactionHistory
// 	var td []TransactionDetails

// 	err := w.s.Read(TransactionStorage, &th, "receiver_did=?", receiver)
// 	if err != nil {
// 		w.log.Error("Failed to get transaction id", "err", err)
// 		return nil, err
// 	}
// 	transactionId := th[0].TransactionID
// 	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
// 	return td, err

// }

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
