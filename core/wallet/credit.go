package wallet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// TODO: Credit structure needs to be worked upon
type Credit struct {
	DID    string `gorm:"column:did"`
	Credit string `gorm:"column:credit;size:4000"`
	Tx     string `gorm:"column:tx"`
}
type TokenInfo struct {
	TokenType   int
	BlockNumber int
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

type PledgeHistory struct {
	QuorumDID           string `gorm:"column:quorum_did"`
	TransactionID       string `gorm:"column:transaction_id"`
	SenderDID           string `gorm:"column:sender_did"`
	ReceiverDID         string `gorm:"column:receiver_did"`
	TransferTokenID     string `gorm:"column:transfer_tokens_id"`
	TransferTokenType   int    `gorm:"column:transfer_tokens_type"`
	TransferBlockID     string `gorm:"column:transfer_block_id"`
	TransferBlockNumber uint64 `gorm:"column:transfer_block_number"`
	CurrentBlockNumber  int    `gorm:"column:current_block_number"`
	TokenCredit         int    `gorm:"column:token_credit"`
	Epoch               int    `gorm:"column:epoch"`
	TokenCreditStatus   int    `gorm:"column:token_credit_status"`
}

func (w *Wallet) AddPledgeHistory(pledgeDetails *PledgeHistory) error {
	fmt.Println("************AAAAAaa***88")
	err := w.s.Write(PledgeHistoryTable, &pledgeDetails)
	if err != nil {
		w.log.Error("Failed to add pledge details to pledge history table", "err", err)
		return err
	}
	return nil
}

func (w *Wallet) CheckTokenExistInPledgeHistory(tokenID string, transID string) (bool, error) {
	var existingPledgeHistory PledgeHistory
	pledgeHistoryReadErr := w.s.Read(PledgeHistoryTable, &existingPledgeHistory, "transfer_tokens_id=? AND transaction_id=?", tokenID, transID)
	if pledgeHistoryReadErr != nil {
		readErr := fmt.Sprint(pledgeHistoryReadErr)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No pledge history")
			return false, pledgeHistoryReadErr
		}
		w.log.Error("Failed to read pledge history", "err", pledgeHistoryReadErr)
		return false, pledgeHistoryReadErr
	} else {
		return true, nil
	}
}
func (w *Wallet) GetTokenDetailsByQuorumDID(quorumDID string) (map[string]TokenInfo, error) {
	var pledges []PledgeHistory
	tokenDetails := make(map[string]TokenInfo)

	// Query the database for records matching the given QuorumDID
	err := w.s.Read(PledgeHistoryTable, &pledges, "quorum_did=? and token_credit_status=?", quorumDID, 0)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "no records found") {
			w.log.Info("No pledge history found for given QuorumDID", "quorumDID", quorumDID)
			return nil, nil // Return nil if no records are found
		}
		w.log.Error("Failed to read pledge history", "quorumDID", quorumDID, "err", err)
		return nil, err
	}

	// Iterate over the results and collect token details
	for _, pledge := range pledges {
		tokenDetails[pledge.TransferTokenID] = TokenInfo{
			TokenType:   pledge.TransferTokenType,
			BlockNumber: pledge.CurrentBlockNumber,
		}
	}

	return tokenDetails, nil
}
func (w *Wallet) UpdateTokenCreditStatus(tokenID string, status int) error {
	return w.s.Update(PledgeHistoryTable, map[string]interface{}{"token_credit_status": status}, "transfer_tokens_id = ?", tokenID)
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
