package core

import (
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) GetTxnDetails(txnId string) (wallet.TxnDetails, error) {

	txnStatusDetails, err := c.w.GetTxnDetails(txnId)
	if err != nil {
		c.log.Error("err", err)
		return wallet.TxnDetails{}, err
	}
	tokens := make([]string, 0)
	for i := range txnStatusDetails {
		tokens = append(tokens, txnStatusDetails[i].Token)
	}

	quorumList := strings.Split(txnStatusDetails[0].QuorumList, "-")
	result := wallet.TxnDetails{
		SenderPeerId:   txnStatusDetails[0].SenderPeerID,
		SenderDID:      txnStatusDetails[0].SenderDID,
		ReceiverPeerId: txnStatusDetails[0].ReceiverPeerID,
		ReceiverDID:    txnStatusDetails[0].ReceiverDID,
		Tokens:         tokens,
		QuorumList:     quorumList,
	}

	return result, nil

}

func (c *Core) GetFinalityPendingTxnDetails() ([]string, error) {
	txnStatusDetails, err := c.w.GetFinalityPendingTxn()
	if err != nil {
		c.log.Error("err", err)
		return nil, err
	}

	txnIds := make([]string, 0)

	for i := range txnStatusDetails {
		txnIds = append(txnIds, txnStatusDetails[i].TxnID)
	}

	txnIds = c.removeDuplicates(txnIds)
	/* result := make([]wallet.TxnDetails, 0)
	for i := range txnIds {
		txnDetails, err := c.GetTxnDetails(txnIds[i])
		if err != nil {
			c.log.Error("err", err)
			return result, err
		}
		result = append(result, txnDetails)
	}
	return result, nil */

	return txnIds, nil
}

func (c *Core) GetFinalityPendingTxns() ([]string, error) {
	txnStatusDetails, err := c.w.GetFinalityPendingTxn()
	if err != nil {
		c.log.Error("err", err)
		return nil, err
	}

	txnIds := make([]string, 0)

	for i := range txnStatusDetails {
		txnIds = append(txnIds, txnStatusDetails[i].TxnID)
	}

	txnIds = c.removeDuplicates(txnIds)
	return txnIds, nil
}

func (c *Core) removeDuplicates(slice []string) []string {
	encountered := map[string]bool{}
	result := []string{}

	for _, item := range slice {
		if !encountered[item] {
			encountered[item] = true
			result = append(result, item)
		}
	}

	return result
}
