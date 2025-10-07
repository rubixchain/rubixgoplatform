package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// FTTransactionMigrationStatus tracks the progress of migration
type FTTransactionMigrationStatus struct {
	StartTime          time.Time `gorm:"column:start_time"`
	EndTime            time.Time `gorm:"column:end_time"`
	LastProcessedTxnID string    `gorm:"column:last_processed_txn_id"`
	TotalTransactions  int       `gorm:"column:total_transactions"`
	ProcessedCount     int       `gorm:"column:processed_count"`
	SuccessCount       int       `gorm:"column:success_count"`
	FailureCount       int       `gorm:"column:failure_count"`
	Status             string    `gorm:"column:status"` // "running", "completed", "failed"
}

// getDefaultDID gets the wallet's default DID
func (w *Wallet) getDefaultDID() (string, error) {
	var dids []DIDType
	err := w.s.Read(DIDStorage, &dids, "did_dir=?", "Default")
	if err != nil {
		return "", err
	}
	if len(dids) == 0 {
		return "", fmt.Errorf("no default DID found")
	}
	return dids[0].DID, nil
}

// MigrateFTTransactionTokens migrates historical FT transaction data to populate token metadata
func (w *Wallet) MigrateFTTransactionTokens() (*FTTransactionMigrationStatus, error) {
	w.log.Info("Starting FT transaction token migration")
	
	// Check if migration table exists, create if not
	err := w.s.Init("FTMigrationStatus", &FTTransactionMigrationStatus{}, true)
	if err != nil {
		w.log.Error("Failed to initialize migration status table", "err", err)
		return nil, err
	}
	
	// Check if a migration is already running
	var existingStatus FTTransactionMigrationStatus
	err = w.s.Read("FTMigrationStatus", &existingStatus, "status=?", "running")
	if err == nil {
		w.log.Warn("Migration already in progress", "started_at", existingStatus.StartTime)
		return &existingStatus, fmt.Errorf("migration already in progress since %v", existingStatus.StartTime)
	}
	
	// Initialize migration status
	status := &FTTransactionMigrationStatus{
		StartTime: time.Now(),
		Status:    "running",
	}
	
	// Save initial status
	err = w.s.Write("FTMigrationStatus", status)
	if err != nil {
		w.log.Error("Failed to save migration status", "err", err)
		return nil, err
	}
	
	// Get all FT transactions
	var ftTransactions []model.TransactionDetails
	err = w.s.Read(TransactionStorage, &ftTransactions, "mode=?", FTTransferMode)
	if err != nil {
		if err.Error() == "no records found" {
			w.log.Info("No FT transactions found to migrate")
			status.Status = "completed"
			status.EndTime = time.Now()
			w.s.Update("FTMigrationStatus", status, "start_time=?", status.StartTime)
			return status, nil
		}
		w.log.Error("Failed to read FT transactions", "err", err)
		status.Status = "failed"
		status.EndTime = time.Now()
		w.s.Update("FTMigrationStatus", status, "start_time=?", status.StartTime)
		return status, err
	}
	
	status.TotalTransactions = len(ftTransactions)
	w.log.Info("Found FT transactions to migrate", "count", status.TotalTransactions)
	
	// Process each transaction
	for _, tx := range ftTransactions {
		status.ProcessedCount++
		status.LastProcessedTxnID = tx.TransactionID
		
		// Skip if metadata already exists
		var existingMetadata []model.FTTransactionToken
		err = w.s.Read(FTTransactionTokenStorage, &existingMetadata, "transaction_id=?", tx.TransactionID)
		if err == nil && len(existingMetadata) > 0 {
			w.log.Debug("Metadata already exists for transaction", "txn_id", tx.TransactionID)
			status.SuccessCount++
			continue
		}
		
		// Determine if this node was sender or receiver
		myDID, err := w.getDefaultDID()
		if err != nil {
			w.log.Error("Failed to get my DID", "err", err)
			status.FailureCount++
			continue
		}
		
		isSender := tx.SenderDID == myDID
		isReceiver := tx.ReceiverDID == myDID
		
		if !isSender && !isReceiver {
			w.log.Debug("Transaction not related to this node", "txn_id", tx.TransactionID)
			continue
		}
		
		// Try to extract token information from various sources
		tokenMetadata, err := w.extractFTTokenMetadata(tx, isSender)
		if err != nil {
			w.log.Error("Failed to extract token metadata", 
				"txn_id", tx.TransactionID,
				"err", err)
			status.FailureCount++
			continue
		}
		
		// Store the metadata
		for _, metadata := range tokenMetadata {
			err = w.s.Write(FTTransactionTokenStorage, &metadata)
			if err != nil {
				w.log.Error("Failed to store token metadata",
					"txn_id", tx.TransactionID,
					"err", err)
				status.FailureCount++
				break
			}
		}
		
		if err == nil {
			status.SuccessCount++
			w.log.Debug("Successfully migrated transaction",
				"txn_id", tx.TransactionID,
				"token_count", len(tokenMetadata))
		}
		
		// Update status periodically
		if status.ProcessedCount%100 == 0 {
			w.s.Update("FTMigrationStatus", status, "start_time=?", status.StartTime)
			w.log.Info("Migration progress",
				"processed", status.ProcessedCount,
				"total", status.TotalTransactions,
				"success", status.SuccessCount,
				"failure", status.FailureCount)
		}
	}
	
	// Finalize migration
	status.Status = "completed"
	status.EndTime = time.Now()
	err = w.s.Update("FTMigrationStatus", status, "start_time=?", status.StartTime)
	if err != nil {
		w.log.Error("Failed to update final migration status", "err", err)
	}
	
	w.log.Info("FT transaction token migration completed",
		"total", status.TotalTransactions,
		"processed", status.ProcessedCount,
		"success", status.SuccessCount,
		"failure", status.FailureCount,
		"duration", status.EndTime.Sub(status.StartTime))
	
	return status, nil
}

// extractFTTokenMetadata attempts to extract FT token metadata from transaction
func (w *Wallet) extractFTTokenMetadata(tx model.TransactionDetails, isSender bool) ([]model.FTTransactionToken, error) {
	var metadata []model.FTTransactionToken
	
	// Method 1: Check if tokens are still in wallet (for received transactions)
	if !isSender {
		var currentTokens []FTToken
		err := w.s.Read(FTTokenStorage, &currentTokens, "transaction_id=?", tx.TransactionID)
		if err == nil && len(currentTokens) > 0 {
			// Group by creator and FT name
			tokenGroups := make(map[string]*model.FTTransactionToken)
			for _, token := range currentTokens {
				key := token.CreatorDID + "|" + token.FTName
				if existing, ok := tokenGroups[key]; ok {
					existing.TokenCount++
				} else {
					tokenGroups[key] = &model.FTTransactionToken{
						TransactionID: tx.TransactionID,
						CreatorDID:    token.CreatorDID,
						FTName:        token.FTName,
						TokenCount:    1,
						Direction:     "received",
					}
				}
			}
			
			for _, tokenMeta := range tokenGroups {
				metadata = append(metadata, *tokenMeta)
			}
			return metadata, nil
		}
	}
	
	// Method 2: Try to parse from transaction comment or other fields
	// This is a best-effort approach since we don't have the original token details
	if strings.Contains(tx.Comment, "FT Transfer") || strings.Contains(tx.Comment, "FT:") {
		// Try to extract FT name from comment if it follows a pattern
		ftName := extractFTNameFromComment(tx.Comment)
		if ftName != "" {
			// For sent transactions, we can only estimate based on amount
			// Assuming 1 RBT = 1 FT token (this may not be accurate for all FTs)
			tokenCount := int(tx.Amount)
			if tokenCount > 0 {
				direction := "sent"
				if !isSender {
					direction = "received"
				}
				
				metadata = append(metadata, model.FTTransactionToken{
					TransactionID: tx.TransactionID,
					CreatorDID:    tx.SenderDID, // Best guess for historical data
					FTName:        ftName,
					TokenCount:    tokenCount,
					Direction:     direction,
				})
			}
		}
	}
	
	// Method 3: Check TokensTransferred table if available
	var transferredTokens []struct {
		TransactionID string `gorm:"column:transaction_id"`
		TokenID       string `gorm:"column:token_id"`
	}
	err := w.s.Read(TokensArrayStorage, &transferredTokens, "transaction_id=?", tx.TransactionID)
	if err == nil && len(transferredTokens) > 0 {
		// Try to get FT info for these tokens
		tokenGroups := make(map[string]*model.FTTransactionToken)
		for _, tt := range transferredTokens {
			// Try to find FT info from token chain
			ftInfo, err := w.getFTInfoFromTokenChain(tt.TokenID)
			if err == nil && ftInfo != nil {
				key := ftInfo.CreatorDID + "|" + ftInfo.FTName
				if existing, ok := tokenGroups[key]; ok {
					existing.TokenCount++
				} else {
					direction := "sent"
					if !isSender {
						direction = "received"
					}
					tokenGroups[key] = &model.FTTransactionToken{
						TransactionID: tx.TransactionID,
						CreatorDID:    ftInfo.CreatorDID,
						FTName:        ftInfo.FTName,
						TokenCount:    1,
						Direction:     direction,
					}
				}
			}
		}
		
		for _, tokenMeta := range tokenGroups {
			metadata = append(metadata, *tokenMeta)
		}
	}
	
	if len(metadata) == 0 {
		return nil, fmt.Errorf("could not extract token metadata for transaction %s", tx.TransactionID)
	}
	
	return metadata, nil
}

// extractFTNameFromComment tries to extract FT name from transaction comment
func extractFTNameFromComment(comment string) string {
	// Common patterns in comments
	patterns := []string{
		"FT:", "FT Transfer:", "FT Name:", "FT-", "FungibleToken:",
	}
	
	for _, pattern := range patterns {
		if idx := strings.Index(comment, pattern); idx != -1 {
			// Extract the word after the pattern
			remaining := comment[idx+len(pattern):]
			remaining = strings.TrimSpace(remaining)
			
			// Take the first word (until space or special char)
			for i, ch := range remaining {
				if ch == ' ' || ch == ',' || ch == ';' || ch == '-' {
					if i > 0 {
						return remaining[:i]
					}
					break
				}
			}
			
			// If no delimiter found, take up to 10 chars
			if len(remaining) > 0 && len(remaining) <= 10 {
				return remaining
			}
		}
	}
	
	return ""
}

// getFTInfoFromTokenChain retrieves FT info from token chain
func (w *Wallet) getFTInfoFromTokenChain(tokenID string) (*struct {
	CreatorDID string
	FTName     string
}, error) {
	// Try to get token info from FT chain storage
	data, err := w.FTChainStorage.DB.Get([]byte(tokenID), nil)
	if err != nil {
		return nil, err
	}
	
	// Parse the block data to extract FT info
	// This is a simplified implementation - you may need to adjust based on actual data structure
	if len(data) > 0 {
		// For now, return error as we need more info about the data structure
		return nil, fmt.Errorf("token chain data parsing not implemented")
	}
	
	return nil, fmt.Errorf("no token chain data found")
}

// GetFTMigrationStatus returns the current or last migration status
func (w *Wallet) GetFTMigrationStatus() (*FTTransactionMigrationStatus, error) {
	var status FTTransactionMigrationStatus
	
	// First try to get running status
	err := w.s.Read("FTMigrationStatus", &status, "status=?", "running")
	if err == nil {
		return &status, nil
	}
	
	// Then get the most recent completed status
	var allStatuses []FTTransactionMigrationStatus
	err = w.s.Read("FTMigrationStatus", &allStatuses, "status=?", "completed")
	if err != nil {
		return nil, fmt.Errorf("no migration status found")
	}
	
	if len(allStatuses) == 0 {
		return nil, fmt.Errorf("no migration status found")
	}
	
	// Return the most recent one
	mostRecent := allStatuses[0]
	for _, s := range allStatuses {
		if s.StartTime.After(mostRecent.StartTime) {
			mostRecent = s
		}
	}
	
	return &mostRecent, nil
}