package core

import (
	"fmt"
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

// GetTxnDetailsByDID retrieves transaction details based on a given DID and an optional date range.
// - If `role` is provided, it filters transactions where the DID is a sender or receiver.
// - If `startDateStr` is provided, only transactions **on or after** this date are returned.
// - If `endDateStr` is provided, only transactions **before** this date are returned.
// - If neither date is specified, all transactions related to the DID are returned.
// - Date format should be "YYYY-MM-DD".
func (c *Core) GetTxnDetailsByDID(did string, role string, startDateStr string, endDateStr string) ([]model.TransactionDetails, error) {
	var startDate, endDate time.Time
	var err error

	// Parse startDate if provided
	if startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			// Handle invalid date format
			c.log.Error("Invalid StartDate format", err)
			return nil, err
		}
	}
	// Parse endDate if provided
	if endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			// Handle invalid date format
			c.log.Error("Invalid EndDate format", err)
			return nil, err
		}
	}

	// Fetch transactions based on the role (if provided)
	var result []model.TransactionDetails

	switch strings.ToLower(role) {
	case "":
		result, err = c.w.GetTransactionByDID(did)
	case "sender":
		result, err = c.w.GetTransactionBySender(did)
	case "receiver":
		result, err = c.w.GetTransactionByReceiver(did)
	default:
		c.log.Error("invalid role : " + role)
		return nil, fmt.Errorf("invalid role: %s", role)
	}

	if err != nil {
		return nil, err
	}

	// If no date filtering is needed, return the results directly
	if startDateStr == "" && endDateStr == "" {
		return result, nil
	}

	// Apply date range filtering
	filteredTxnDetails, err := c.FilterTxnDetailsByDateRange(result, startDate, endDate)
	if err != nil {
		c.log.Error("failed to filter by date range", err)
		return nil, err
	}
	return filteredTxnDetails, nil
}

func (c *Core) GetTxnDetailsByComment(comment string) ([]model.TransactionDetails, error) {
	res, err := c.w.GetTransactionByComment(comment)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// FilterTxnDetailsByDateRange filters transactions based on a given date range.
// - Only transactions **on or after** `startDate` are included.
// - Only transactions **before** `endDate` are included.
// - If `startDate` or `endDate` is not provided, filtering is skipped for that condition.
func (c *Core) FilterTxnDetailsByDateRange(transactions []model.TransactionDetails, startDate time.Time, endDate time.Time) ([]model.TransactionDetails, error) {
	var filteredTransactions []model.TransactionDetails

	for _, txn := range transactions {
		// Extract only the date part from txn.DateTime (ignoring the time component)
		txnDateTimeStr := txn.DateTime.Format("2006-01-02")
		txnDateTime, _ := time.Parse("2006-01-02", txnDateTimeStr)

		// Skip transactions before the start date (if specified)
		if !startDate.IsZero() && txnDateTime.Before(startDate) {
			continue
		}

		// Skip transactions on or after the end date (end date is exclusive)
		if !endDate.IsZero() && txnDateTime.After(endDate) {
			continue
		}
		filteredTransactions = append(filteredTransactions, txn)
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
