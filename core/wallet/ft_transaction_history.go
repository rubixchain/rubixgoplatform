package wallet

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	FTTransactionHistoryStorage string = "FTTransactionHistory"
)

// InitFTTransactionHistory initializes the FT transaction history table
func (w *Wallet) InitFTTransactionHistory() error {
	return w.s.Init(FTTransactionHistoryStorage, &model.FTTransactionHistory{}, true)
}

// AddFTTransactionHistory stores FT transaction in dedicated table
func (w *Wallet) AddFTTransactionHistory(td *model.TransactionDetails, ftName, creatorDID string, tokenCount int) error {
	ftTx := &model.FTTransactionHistory{
		TransactionID:   td.TransactionID,
		TransactionType: td.TransactionType,
		BlockID:         td.BlockID,
		Mode:            td.Mode,
		SenderDID:       td.SenderDID,
		ReceiverDID:     td.ReceiverDID,
		Amount:          td.Amount,
		TotalTime:       td.TotalTime,
		Comment:         td.Comment,
		DateTime:        td.DateTime,
		Status:          td.Status,
		DeployerDID:     td.DeployerDID,
		Epoch:           td.Epoch,
		FTName:          ftName,
		CreatorDID:      creatorDID,
		TokenCount:      tokenCount,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	
	err := w.s.Write(FTTransactionHistoryStorage, ftTx)
	if err != nil {
		w.log.Error("Failed to store FT transaction history", "err", err)
		return err
	}
	
	w.log.Debug("FT transaction history added", 
		"transaction_id", td.TransactionID,
		"ft_name", ftName,
		"token_count", tokenCount)
	
	return nil
}

// GetAllFTTransactionsByDID retrieves all FT transactions for a DID
func (w *Wallet) GetAllFTTransactionsByDID(did string) ([]model.FTTransactionHistory, error) {
	var ftTransactions []model.FTTransactionHistory
	err := w.s.Read(FTTransactionHistoryStorage, &ftTransactions, "(sender_did=? OR receiver_did=?)", did, did)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Info("No FT transactions found for DID", "did", did)
			return []model.FTTransactionHistory{}, nil
		}
		w.log.Error("Failed to get FT transaction history", "did", did, "err", err)
		return nil, err
	}
	return ftTransactions, nil
}

// GetFTTransactionsBySender retrieves FT transactions where DID is sender
func (w *Wallet) GetFTTransactionsBySender(sender string) ([]model.FTTransactionHistory, error) {
	var ftTransactions []model.FTTransactionHistory
	err := w.s.Read(FTTransactionHistoryStorage, &ftTransactions, "sender_did=?", sender)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Info("No FT transactions found for sender", "DID", sender)
			return []model.FTTransactionHistory{}, nil
		}
		w.log.Error("Failed to get FT transactions by sender", "err", err)
		return nil, err
	}
	return ftTransactions, nil
}

// GetFTTransactionsByReceiver retrieves FT transactions where DID is receiver
func (w *Wallet) GetFTTransactionsByReceiver(receiver string) ([]model.FTTransactionHistory, error) {
	var ftTransactions []model.FTTransactionHistory
	err := w.s.Read(FTTransactionHistoryStorage, &ftTransactions, "receiver_did=?", receiver)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Info("No FT transactions found for receiver", "DID", receiver)
			return []model.FTTransactionHistory{}, nil
		}
		w.log.Error("Failed to get FT transactions by receiver", "err", err)
		return nil, err
	}
	return ftTransactions, nil
}

// MigrateFTTransactionsToNewTable migrates FT transactions from TransactionHistory to FTTransactionHistory
func (w *Wallet) MigrateFTTransactionsToNewTable() error {
	w.log.Info("Starting migration of FT transactions to new table")
	
	// Initialize the new table
	err := w.InitFTTransactionHistory()
	if err != nil {
		w.log.Error("Failed to initialize FT transaction history table", "err", err)
		return err
	}
	
	// Get all FT transactions from TransactionHistory
	var ftTransactions []model.TransactionDetails
	err = w.s.Read(TransactionStorage, &ftTransactions, "mode=?", FTTransferMode)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Info("No FT transactions found to migrate")
			return nil
		}
		w.log.Error("Failed to read FT transactions", "err", err)
		return err
	}
	
	w.log.Info("Found FT transactions to migrate", "count", len(ftTransactions))
	
	migratedCount := 0
	for _, tx := range ftTransactions {
		// Check if already migrated
		var existing model.FTTransactionHistory
		err := w.s.Read(FTTransactionHistoryStorage, &existing, "transaction_id=?", tx.TransactionID)
		if err == nil {
			w.log.Debug("Transaction already migrated", "txn_id", tx.TransactionID)
			continue
		}
		
		// Try to get FT info from current tokens
		ftName := ""
		creatorDID := ""
		tokenCount := int(tx.Amount)
		
		// Check if we have tokens from this transaction
		var ftTokens []FTToken
		err = w.s.Read(FTTokenStorage, &ftTokens, "transaction_id=?", tx.TransactionID)
		if err == nil && len(ftTokens) > 0 {
			ftName = ftTokens[0].FTName
			creatorDID = ftTokens[0].CreatorDID
			tokenCount = len(ftTokens)
		}
		
		// Create FT transaction history entry
		ftTx := &model.FTTransactionHistory{
			TransactionID:   tx.TransactionID,
			TransactionType: tx.TransactionType,
			BlockID:         tx.BlockID,
			Mode:            tx.Mode,
			SenderDID:       tx.SenderDID,
			ReceiverDID:     tx.ReceiverDID,
			Amount:          tx.Amount,
			TotalTime:       tx.TotalTime,
			Comment:         tx.Comment,
			DateTime:        tx.DateTime,
			Status:          tx.Status,
			DeployerDID:     tx.DeployerDID,
			Epoch:           tx.Epoch,
			FTName:          ftName,
			CreatorDID:      creatorDID,
			TokenCount:      tokenCount,
			CreatedAt:       tx.DateTime,
			UpdatedAt:       time.Now(),
		}
		
		err = w.s.Write(FTTransactionHistoryStorage, ftTx)
		if err != nil {
			w.log.Error("Failed to migrate transaction", "txn_id", tx.TransactionID, "err", err)
			continue
		}
		
		migratedCount++
		w.log.Debug("Migrated FT transaction", "txn_id", tx.TransactionID)
	}
	
	w.log.Info("FT transaction migration completed", "total", len(ftTransactions), "migrated", migratedCount)
	return nil
}