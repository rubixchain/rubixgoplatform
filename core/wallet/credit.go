package wallet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// TODO: Credit structure needs to be worked upon
type Credit struct {
	DID    string `gorm:"column:did"`
	Credit string `gorm:"column:credit;size:4000"`
	Tx     string `gorm:"column:tx"`
}

// TODO: Credit structure needs to be worked upon
type PledgeInformation struct {
	TokenID         string `json:"token_id"`
	TokenType       int    `json:"token_type"`
	PledgeBlockID   string `json:"pledge_block_id"`
	UnpledgeBlockID string `json:"unpledge_block_id"`
	QuorumDID       string `json:"quorum_did"`
	TransactionID   string `json:"transaction_id"`
}

func (w *Wallet) StoreCredit(transactionID string, quorumDID string, pledgeInfo []*PledgeInformation) error {
	pledgeInfoBytes, err := json.Marshal(pledgeInfo)
	if err != nil {
		return fmt.Errorf("failed while marshalling credits: %v", err.Error())
	}
	pledgeInfoEncoded := base64.StdEncoding.EncodeToString(pledgeInfoBytes)

	credit := &Credit{
		DID:    quorumDID,
		Credit: pledgeInfoEncoded,
		Tx:     transactionID,
	}

	return w.s.Write(CreditStorage, credit)
}

func (w *Wallet) GetCredit(did string) ([]string, error) {
	var c []Credit
	err := w.s.Read(CreditStorage, &c, "did=?", did)
	if err != nil {
		return nil, err
	}
	str := make([]string, 0)
	for i := range c {
		str = append(str, c[i].Credit)
	}
	return str, nil
}

func (w *Wallet) RemoveCredit(transactionID string) error {
	err := w.s.Delete(CreditStorage, &Credit{}, "tx = ?", transactionID)
	if err != nil {
		errMsg := fmt.Errorf("failed to remove Credit for transaction: %v", transactionID)
		w.log.Error(errMsg.Error())
		return errMsg
	}

	return nil
}
