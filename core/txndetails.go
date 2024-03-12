package core

import (
	"strings"
	"time"

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

func (c *Core) GetTxnDetailsByDID(did string, role string, startDate string, endDate string) ([]wallet.TransactionDetails, error) {
	if role == "" {
		txnAsSender, err := c.w.GetTransactionBySender(did)
		if err != nil && err.Error() != "no records found" {
			return nil, err
		}
		txnAsReceiver, err := c.w.GetTransactionByReceiver(did)
		if err != nil && err.Error() != "no records found" {
			return nil, err
		}
		result := make([]wallet.TransactionDetails, 0)

		if len(txnAsReceiver) > 0 {
			result = append(result, txnAsReceiver...)
		}

		if len(txnAsSender) > 0 {
			result = append(result, txnAsSender...)
		}

		if startDate == "" && endDate == "" {
			return result, nil
		} else {
			var startTime, endTime time.Time
			if startDate != "" {
				startTime, err = time.Parse(time.DateTime, startDate)
				if err != nil {
					// Handle invalid date format
					c.log.Error("Invalid StartDate format", err)
					// Return an error response or handle it accordingly
					return nil, err
				}
			}

			if endDate != "" {
				endTime, err = time.Parse(time.DateTime, endDate)
				if err != nil {
					// Handle invalid date format
					c.log.Error("Invalid EndDate format", err)
					// Return an error response or handle it accordingly
					return nil, err
				}
			}
			filteredTxnDetails, err := c.FilterTxnDetailsByDateRange(result, startTime, endTime)
			if err != nil {
				c.log.Error("failed to filter by date range", err)
				return nil, err
			}
			return filteredTxnDetails, nil
		}

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

// FilterTxnDetailsByDateRange filters TransactionDetails by a date range.
func (c *Core) FilterTxnDetailsByDateRange(transactions []wallet.TransactionDetails, startTime time.Time, endTime time.Time) ([]wallet.TransactionDetails, error) {
	var filteredTransactions []wallet.TransactionDetails

	for _, txn := range transactions {
		txnDateTimeStr := txn.DateTime.Format(time.DateTime)
		txnDateTimeParsed, err := time.Parse(time.DateTime, txnDateTimeStr)
		if err != nil {
			return nil, err
		}
		if txnDateTimeParsed.After(startTime) && txnDateTimeParsed.Before(endTime) {
			filteredTransactions = append(filteredTransactions, txn)
		}
	}

	return filteredTransactions, nil
}

func (c *Core) GetCountofTxn(did string) (wallet.TransactionCount, error) {
	result := wallet.TransactionCount{
		DID: did,
	}
	txnAsSender, err := c.w.GetTransactionBySender(did)
	if err != nil && err.Error() != "no records found" {
		return result, err
	}
	txnAsReceiver, err := c.w.GetTransactionByReceiver(did)
	if err != nil && err.Error() != "no records found" {
		return result, err
	}
	result.TxnSend = len(txnAsSender)
	result.TxnReceived = len(txnAsReceiver)
	return result, nil
}
