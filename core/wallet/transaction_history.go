package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	SendMode int = iota
	RecvMode
	DeployMode
	ExecuteMode
	PinningServiceMode
	FTTransferMode
)

func (w *Wallet) AddTransactionHistory(td *model.TransactionDetails) error {
	err := w.s.Write(TransactionStorage, td)
	if err != nil {
		w.log.Error("Failed to store transaction history", "err", err)
		return err
	} else {
		w.log.Info("Transaction history added", "transaction_id", td.TransactionID)
	}
	return nil
}

// AddFTTransactionTokens stores FT token metadata for a transaction
func (w *Wallet) AddFTTransactionTokens(transactionID string, creatorDID string, ftName string, tokenCount int, direction string) error {
	ftTxToken := &model.FTTransactionToken{
		TransactionID: transactionID,
		CreatorDID:    creatorDID,
		FTName:        ftName,
		TokenCount:    tokenCount,
		Direction:     direction,
		CreatedAt:     time.Now(),
	}
	
	err := w.s.Write(FTTransactionTokenStorage, ftTxToken)
	if err != nil {
		// Check if it's a table doesn't exist error
		if strings.Contains(err.Error(), "no such table") {
			w.log.Warn("FT transaction token table doesn't exist, skipping metadata storage", "err", err)
			// Don't fail the transaction for backward compatibility
			return nil
		}
		w.log.Error("Failed to store FT transaction token metadata", "err", err)
		// Still don't fail the transaction, just log the error
		return nil
	}
	
	w.log.Debug("FT transaction token metadata stored", 
		"transaction_id", transactionID,
		"ft_name", ftName,
		"token_count", tokenCount,
		"direction", direction)
	
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

	err := w.s.Read(TransactionStorage, &td, "receiver_did=? AND (mode=? OR mode=?)", receiver, SendMode, RecvMode)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as Receiver ", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionBySender(sender string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "sender_did=? AND (mode=? OR mode=?)", sender, SendMode, RecvMode)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as sender", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionByDID(did string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails
	err := w.s.Read(TransactionStorage, &td, "(sender_did=? OR receiver_did=?) AND (mode=? OR mode=?)", did, did, SendMode, RecvMode)
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

func (w *Wallet) GetAllFTTransactionDetailsByDID(did string) ([]model.TransactionDetails, error) {
	// Try new FT transaction history table first
	ftHistories, err := w.GetAllFTTransactionsByDID(did)
	if err == nil && len(ftHistories) > 0 {
		// Convert to TransactionDetails format
		var ftTransactions []model.TransactionDetails
		for _, ft := range ftHistories {
			td := model.TransactionDetails{
				TransactionID:   ft.TransactionID,
				TransactionType: ft.TransactionType,
				BlockID:         ft.BlockID,
				Mode:            ft.Mode,
				SenderDID:       ft.SenderDID,
				ReceiverDID:     ft.ReceiverDID,
				Amount:          ft.Amount,
				TotalTime:       ft.TotalTime,
				Comment:         ft.Comment,
				DateTime:        ft.DateTime,
				Status:          ft.Status,
				DeployerDID:     ft.DeployerDID,
				Epoch:           ft.Epoch,
				Tokens: []model.FTTokenSummary{
					{
						CreatorDID: ft.CreatorDID,
						FTName:     ft.FTName,
						Count:      ft.TokenCount,
					},
				},
			}
			ftTransactions = append(ftTransactions, td)
		}
		return ftTransactions, nil
	}
	
	// Fallback to old method
	var ftTransactions []model.TransactionDetails
	err = w.s.Read(TransactionStorage, &ftTransactions, "mode=? AND (sender_did=? OR receiver_did=?)", FTTransferMode, did, did)
	if err != nil {
		w.log.Error("Failed to get FT transaction details with did", did, "err", err)
		return nil, err
	}
	if len(ftTransactions) == 0 {
		w.log.Info("No FT transactions found for DID", "did", did)
		return []model.TransactionDetails{}, nil
	}

	// Get token summaries for all transactions
	tokenSummaryMap, err := w.getFTTokenSummariesGroupedByTransactionID()
	if err != nil {
		w.log.Error("Failed to retrieve FT token summaries", "error", err)
		return nil, err
	}

	// Attach token summaries to each transaction
	for i := range ftTransactions {
		txID := ftTransactions[i].TransactionID
		if summaries, ok := tokenSummaryMap[txID]; ok {
			ftTransactions[i].Tokens = summaries
		} else {
			ftTransactions[i].Tokens = []model.FTTokenSummary{}
		}
	}

	return ftTransactions, nil
}

func (w *Wallet) GetFTTransactionByReceiver(receiver string) ([]model.TransactionDetails, error) {
	// Try new FT transaction history table first
	ftHistories, err := w.GetFTTransactionsByReceiver(receiver)
	if err == nil && len(ftHistories) > 0 {
		// Convert to TransactionDetails format
		var ftTransactions []model.TransactionDetails
		for _, ft := range ftHistories {
			td := model.TransactionDetails{
				TransactionID:   ft.TransactionID,
				TransactionType: ft.TransactionType,
				BlockID:         ft.BlockID,
				Mode:            ft.Mode,
				SenderDID:       ft.SenderDID,
				ReceiverDID:     ft.ReceiverDID,
				Amount:          ft.Amount,
				TotalTime:       ft.TotalTime,
				Comment:         ft.Comment,
				DateTime:        ft.DateTime,
				Status:          ft.Status,
				DeployerDID:     ft.DeployerDID,
				Epoch:           ft.Epoch,
				Tokens: []model.FTTokenSummary{
					{
						CreatorDID: ft.CreatorDID,
						FTName:     ft.FTName,
						Count:      ft.TokenCount,
					},
				},
			}
			ftTransactions = append(ftTransactions, td)
		}
		return ftTransactions, nil
	}
	
	// Fallback to old method
	var ftTransactions []model.TransactionDetails
	err = w.s.Read(TransactionStorage, &ftTransactions, "receiver_did=? AND mode=?", receiver, FTTransferMode)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as Receiver ", "err", err)
		return nil, err
	}
	if len(ftTransactions) == 0 {
		w.log.Info("No FT transactions found for receiver", "DID", receiver)
		return []model.TransactionDetails{}, nil
	}

	// Get token summaries for all transactions
	tokenSummaryMap, err := w.getFTTokenSummariesGroupedByTransactionID()
	if err != nil {
		w.log.Error("Failed to retrieve FT token summaries", "error", err)
		return nil, err
	}

	// Attach token summaries to each transaction
	for i := range ftTransactions {
		txID := ftTransactions[i].TransactionID
		if summaries, ok := tokenSummaryMap[txID]; ok {
			ftTransactions[i].Tokens = summaries
		} else {
			ftTransactions[i].Tokens = []model.FTTokenSummary{}
		}
	}

	return ftTransactions, nil
}

func (w *Wallet) GetFTTransactionBySender(sender string) ([]model.TransactionDetails, error) {
	// Try new FT transaction history table first
	ftHistories, err := w.GetFTTransactionsBySender(sender)
	if err == nil && len(ftHistories) > 0 {
		// Convert to TransactionDetails format
		var ftTransactions []model.TransactionDetails
		for _, ft := range ftHistories {
			td := model.TransactionDetails{
				TransactionID:   ft.TransactionID,
				TransactionType: ft.TransactionType,
				BlockID:         ft.BlockID,
				Mode:            ft.Mode,
				SenderDID:       ft.SenderDID,
				ReceiverDID:     ft.ReceiverDID,
				Amount:          ft.Amount,
				TotalTime:       ft.TotalTime,
				Comment:         ft.Comment,
				DateTime:        ft.DateTime,
				Status:          ft.Status,
				DeployerDID:     ft.DeployerDID,
				Epoch:           ft.Epoch,
				Tokens: []model.FTTokenSummary{
					{
						CreatorDID: ft.CreatorDID,
						FTName:     ft.FTName,
						Count:      ft.TokenCount,
					},
				},
			}
			ftTransactions = append(ftTransactions, td)
		}
		return ftTransactions, nil
	}
	
	// Fallback to old method
	var ftTransactions []model.TransactionDetails

	err = w.s.Read(TransactionStorage, &ftTransactions, "sender_did=? AND mode=?", sender, FTTransferMode)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as sender", "err", err)
		return nil, err
	}
	if len(ftTransactions) == 0 {
		w.log.Info("No FT transactions found for sender", "DID", sender)
		return []model.TransactionDetails{}, nil
	}

	// Get token summaries for all transactions
	tokenSummaryMap, err := w.getFTTokenSummariesGroupedByTransactionID()
	if err != nil {
		w.log.Error("Failed to retrieve FT token summaries", "error", err)
		return nil, err
	}

	// Attach token summaries to each transaction
	for i := range ftTransactions {
		txID := ftTransactions[i].TransactionID
		if summaries, ok := tokenSummaryMap[txID]; ok {
			ftTransactions[i].Tokens = summaries
		} else {
			ftTransactions[i].Tokens = []model.FTTokenSummary{}
		}
	}

	return ftTransactions, nil
}

func (w *Wallet) getFTTokenSummariesGroupedByTransactionID() (map[string][]model.FTTokenSummary, error) {
	// First try to read from the new FTTransactionTokenStorage (historical data)
	var txTokens []model.FTTransactionToken
	err := w.s.Read(FTTransactionTokenStorage, &txTokens, "1 = 1")
	
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return nil, fmt.Errorf("failed to read FT transaction tokens: %w", err)
	}

	// If we have transaction token data, use it
	if err == nil && len(txTokens) > 0 {
		// Group tokens by transaction_id and aggregate by creator_did, ft_name
		grouped := make(map[string]map[string]*model.FTTokenSummary)

		for _, token := range txTokens {
			txID := token.TransactionID
			key := token.CreatorDID + "|" + token.FTName

			if _, exists := grouped[txID]; !exists {
				grouped[txID] = make(map[string]*model.FTTokenSummary)
			}

			if summary, exists := grouped[txID][key]; exists {
				// Add the token count from the stored record
				summary.Count += token.TokenCount
			} else {
				grouped[txID][key] = &model.FTTokenSummary{
					CreatorDID: token.CreatorDID,
					FTName:     token.FTName,
					Count:      token.TokenCount,
				}
			}
		}

		// Convert inner maps to slices
		finalResult := make(map[string][]model.FTTokenSummary)
		for txID, tokenMap := range grouped {
			for _, summary := range tokenMap {
				finalResult[txID] = append(finalResult[txID], *summary)
			}
		}

		return finalResult, nil
	}

	// Fall back to reading from FTTokenStorage for backward compatibility
	w.log.Debug("Falling back to FTTokenStorage for backward compatibility")
	
	var allTokens []FTToken

	err = w.s.Read(FTTokenStorage, &allTokens, "1 = 1")
	if err != nil {
		w.log.Error("Failed to read from FTTokenStorage", "error", err)
		return nil, fmt.Errorf("failed to read FT tokens: %w", err)
	}

	if len(allTokens) == 0 {
		return map[string][]model.FTTokenSummary{}, nil
	}

	// Group and count tokens per transaction_id, creator_did, ft_name
	grouped := make(map[string]map[string]*model.FTTokenSummary)

	for _, token := range allTokens {
		// Skip tokens without transaction ID
		if token.TransactionID == "" {
			continue
		}
		
		txID := token.TransactionID
		key := token.CreatorDID + "|" + token.FTName

		if _, exists := grouped[txID]; !exists {
			grouped[txID] = make(map[string]*model.FTTokenSummary)
		}

		if summary, exists := grouped[txID][key]; exists {
			summary.Count++
		} else {
			grouped[txID][key] = &model.FTTokenSummary{
				CreatorDID: token.CreatorDID,
				FTName:     token.FTName,
				Count:      1,
			}
		}
	}

	// Convert inner maps to slices
	finalResult := make(map[string][]model.FTTokenSummary)
	for txID, tokenMap := range grouped {
		for _, summary := range tokenMap {
			finalResult[txID] = append(finalResult[txID], *summary)
		}
	}

	return finalResult, nil
}
