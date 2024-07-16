package core

import (
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) GetTxnDetailsByID(txnID string) (model.TransactionDetails, error) {
	var th model.TransactionDetails
	res, err := c.w.GetTransactionDetailsbyTransactionId(txnID)
	if err != nil {
		return th, err
	}
	return res, nil
}

func (c *Core) GetTxnDetailsByDID(did string, role string, startDateStr string, endDateStr string) ([]model.TransactionDetails, error) {
	var startDate time.Time
	var endDate time.Time
	var err error

	var result []model.TransactionDetails

	if role == "" {
		result, err = c.w.GetTransactionByDID(did)
		if startDateStr == "" && endDateStr == "" {
			if err != nil {
				return nil, err
			}
			return result, nil
		} else {
			if startDateStr != "" {
				startDate, err = time.Parse("2006-01-02", startDateStr)
				if err != nil {
					// Handle invalid date format
					c.log.Error("Invalid StartDate format", err)
					// Return an error response or handle it accordingly
					return nil, err
				}
			}

			if endDateStr != "" {
				endDate, err = time.Parse("2006-01-02", endDateStr)
				if err != nil {
					// Handle invalid date format
					c.log.Error("Invalid EndDate format", err)
					// Return an error response or handle it accordingly
					return nil, err
				}
			}

			filteredTxnDetails, err := c.FilterTxnDetailsByDateRange(result, startDate, endDate)
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

func (c *Core) GetTxnDetailsByComment(comment string) ([]model.TransactionDetails, error) {
	res, err := c.w.GetTransactionByComment(comment)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// FilterTxnDetailsByDateRange filters TransactionDetails by a date range.
func (c *Core) FilterTxnDetailsByDateRange(transactions []model.TransactionDetails, startDate time.Time, endDate time.Time) ([]model.TransactionDetails, error) {
	var filteredTransactions []model.TransactionDetails
	for _, txn := range transactions {
		txnDateTimeStr := txn.DateTime.Format("2006-01-02")
		txnDateTimeParsed, err := time.Parse("2006-01-02", txnDateTimeStr)
		if err != nil {
			return nil, err
		}
		if (txnDateTimeParsed.Equal(startDate) || txnDateTimeParsed.After(startDate)) && txnDateTimeParsed.Before(endDate) {
			filteredTransactions = append(filteredTransactions, txn)
		}
	}

	return filteredTransactions, nil
}

func (c *Core) GetCountofTxn(did string) (model.TransactionCount, error) {
	result := model.TransactionCount{
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
