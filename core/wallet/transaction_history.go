package wallet

import (
	"fmt"
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
	}
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

	err := w.s.Read(TransactionStorage, &td, "receiver_did=?", receiver)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as Receiver ", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionBySender(sender string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &td, "sender_did=?", sender)
	if err != nil {
		w.log.Error("Failed to get transaction details with did as sender", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) GetTransactionByDID(did string) ([]model.TransactionDetails, error) {
	var td []model.TransactionDetails
	err := w.s.Read(TransactionStorage, &td, "sender_did=? OR receiver_did=?", did, did)
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
	var ftTransactions []model.TransactionDetails
	err := w.s.Read(TransactionStorage, &ftTransactions, "mode=? AND (sender_did=? OR receiver_did=?)", FTTransferMode, did, did)
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
	var ftTransactions []model.TransactionDetails
	err := w.s.Read(TransactionStorage, &ftTransactions, "receiver_did=? AND mode=?", receiver, FTTransferMode)
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
	var ftTransactions []model.TransactionDetails

	err := w.s.Read(TransactionStorage, &ftTransactions, "sender_did=? AND mode=?", sender, FTTransferMode)
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
	type FTToken struct {
		TransactionID string `gorm:"column:transaction_id"`
		CreatorDID    string `gorm:"column:creator_did"`
		FTName        string `gorm:"column:ft_name"`
	}

	var allTokens []FTToken

	err := w.s.Read(FTTokenStorage, &allTokens, "1 = 1")
	if err != nil {
		return nil, fmt.Errorf("failed to read FT tokens: %w", err)
	}

	if len(allTokens) == 0 {
		return map[string][]model.FTTokenSummary{}, nil
	}

	// Group and count tokens per transaction_id, creator_did, ft_name
	grouped := make(map[string]map[string]*model.FTTokenSummary)

	for _, token := range allTokens {
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
