package core

import (
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) GetTxnDetailsByID(txnID string) (wallet.TransactionDetails, error) {
	var th wallet.TransactionDetails
	res, err := c.w.GetTransactionDetailsbyTransactionId(txnID)
	if err != nil {
		return th, err
	}
	return res, nil
}

func (c *Core) GetTxnDetailsByDID(did string, role string) ([]wallet.TransactionDetails, error) {
	if role == "" {
		txnAsSender, err := c.w.GetTransactionBySender(did)
		if err != nil {
			return nil, err
		}
		txnAsReceiver, err := c.w.GetTransactionByReceiver(did)
		if err != nil {
			return nil, err
		}
		result := make([]wallet.TransactionDetails, 0)

		for i := range txnAsReceiver {
			result = append(result, txnAsSender[i])
		}

		for i := range txnAsSender {
			result = append(result, txnAsReceiver[i])
		}

		return result, nil
	}

	lower := strings.ToLower(role)
	if lower == "sender" {
		txnAsSender, err := c.w.GetTransactionBySender(did)
		if err != nil {
			return nil, err
		}
		return txnAsSender, nil
	}

	if lower == "receiver" {
		txnAsReceiver, err := c.w.GetTransactionByReceiver(did)
		if err != nil {
			return nil, err
		}
		return txnAsReceiver, nil
	}

	return nil, nil
}

func (c *Core) GetTxnDetailsByComment(comment string) ([]wallet.TransactionDetails, error) {
	res, err := c.w.GetTransactionByComment(comment)
	if err != nil {
		return nil, err
	}
	return res, nil
}
