package wallet

import (
	"strings"
	"time"
)

type TransactionHistory struct {
	TransactionID     string    `gorm:"column:transaction_id;primary_key"`
	SenderDID         string    `gorm:"column:sender_did"`
	ReceiverDID       string    `gorm:"column:receiver_did"`
	Amount            float64   `gorm:"column:amount"`
	TotalTime         int       `gorm:"column:total_time"`
	Comment           string    `gorm:"column:comment"`
	DateTime          time.Time `gorm:"column:date_time"`
	TransactionStatus bool      `gorm:"column:status"`
}

type TokensTransferred struct {
	TransactionID string    `gorm:"column:transaction_id;primary_key"`
	Tokens        string    `gorm:"column:tokens;primary_key"`
	DateTime      time.Time `gorm:"column:date_time"`
	PledgedTokens string    `gorm:"column:pledged_tokens"`
}

type QuorumList struct {
	TransactionID string `gorm:"column:transaction_id;primary_key"`
	QuorumList    string `gorm:"column:quorum_list;primary_key"`
}

type TransactionDetails struct {
	TransactionID   string
	SenderDID       string
	ReceiverDID     string
	Amount          float64
	TotalTime       int
	Comment         string
	DateTime        time.Time
	WholeTokens     []string
	PartTokens      []string
	QuorumList      []string
	PledgedTokenMap map[string]interface{}
}

func (w *Wallet) AddTransactionHistory(td *TransactionDetails) error {
	th := &TransactionHistory{
		TransactionID:     td.TransactionID,
		SenderDID:         td.SenderDID,
		ReceiverDID:       td.ReceiverDID,
		Amount:            td.Amount,
		TotalTime:         td.TotalTime,
		Comment:           td.Comment,
		DateTime:          td.DateTime,
		TransactionStatus: true,
	}
	err := w.s.Write(TransactionStorage, &th)
	if err != nil {
		w.log.Error("Failed to store transaction history", "err", err)
		return err
	}
	pledgedTokenMap := td.PledgedTokenMap
	for k, v := range pledgedTokenMap {
		t := &TokensTransferred{
			TransactionID: td.TransactionID,
			DateTime:      td.DateTime,
			Tokens:        k,
			PledgedTokens: v.(string),
		}
		err := w.s.Write(TokensArrayStorage, &t)
		if err != nil {
			w.log.Error("Failed to store tokens transferred", "err", err)
			return err
		}

	}

	ql := td.QuorumList
	for j := range ql {
		qlsplit := strings.Split(ql[j], ".")
		qdid := qlsplit[1]
		q := &QuorumList{
			TransactionID: td.TransactionID,
			QuorumList:    qdid,
		}
		err = w.s.Write(QuorumListStorage, &q)

	}
	return err

}

func (w *Wallet) GetTransactionDetailsbyTransactionId(transactionId string) ([]TransactionDetails, error) {
	var th []TransactionHistory
	var tt []TokensTransferred
	var ql []QuorumList
	err := w.s.Read(TransactionStorage, &th, "transaction_id=?", transactionId)
	if err != nil {
		w.log.Error("Failed to get transaction details", "err", err)
		return nil, err
	}
	err = w.s.Read(TokensArrayStorage, &tt, "transaction_id=?", transactionId)
	if err != nil {
		w.log.Error("Failed to get tokens transferred", "err", err)
		return nil, err
	}
	err = w.s.Read(QuorumListStorage, &ql, "transaction_id=?", transactionId)
	if err != nil {
		w.log.Error("Failed to get quorum list", "err", err)
		return nil, err
	}
	var td []TransactionDetails
	for i := range th {
		var t TransactionDetails
		t.TransactionID = th[i].TransactionID
		t.SenderDID = th[i].SenderDID
		t.ReceiverDID = th[i].ReceiverDID
		t.Amount = th[i].Amount
		t.TotalTime = th[i].TotalTime
		t.Comment = th[i].Comment
		t.DateTime = th[i].DateTime
		for j := range tt {
			if tt[j].TransactionID == th[i].TransactionID {
				t.WholeTokens = append(t.WholeTokens, tt[i].Tokens)
			}
		}
		for l := range tt {
			if tt[l].TransactionID == th[i].TransactionID {
				t.PartTokens = append(t.PartTokens, tt[i].Tokens)
			}
		}
		for k := range ql {
			if ql[k].TransactionID == th[i].TransactionID {
				t.QuorumList = append(t.QuorumList, ql[k].QuorumList)
			}
		}
		td = append(td, t)
	}

	return td, nil
}

func (w *Wallet) GetTransactionByComment(comment string) ([]TransactionDetails, error) {
	var th []TransactionHistory
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &th, "comment=?", comment)
	if err != nil {
		w.log.Error("Failed to get transaction id", "err", err)
		return nil, err
	}
	transactionId := th[0].TransactionID
	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
	return td, err

}

func (w *Wallet) GetTransactionByReceiver(receiver string) ([]TransactionDetails, error) {
	var th []TransactionHistory
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &th, "receiver_did=?", receiver)
	if err != nil {
		w.log.Error("Failed to get transaction id", "err", err)
		return nil, err
	}
	transactionId := th[0].TransactionID
	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
	return td, err

}

func (w *Wallet) GetTransactionByDate(date string) ([]TransactionDetails, error) {
	var th []TransactionHistory
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &th, "date_time=?", date)
	if err != nil {
		w.log.Error("Failed to get transaction id", "err", err)
		return nil, err
	}
	transactionId := th[0].TransactionID
	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
	return td, err

}

func (w *Wallet) GetTransactionByStatus(status bool) ([]TransactionDetails, error) {
	var th []TransactionHistory
	var td []TransactionDetails

	err := w.s.Read(TransactionStorage, &th, "transaction_status=?", status)
	if err != nil {
		w.log.Error("Failed to get transaction id", "err", err)
		return nil, err
	}
	transactionId := th[0].TransactionID
	td, err = w.GetTransactionDetailsbyTransactionId(transactionId)
	return td, err

}
