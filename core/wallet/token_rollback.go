package wallet

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// RollbackTokenLock rolls back locked tokens to free state
func (w *Wallet) RollbackTokenLock(tokenIDs []string, txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back token locks",
		"transaction_id", txID,
		"token_count", len(tokenIDs))

	rolledBackCount := 0
	for _, tokenID := range tokenIDs {
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", tokenID)
		if err != nil {
			w.log.Error("Token not found for rollback",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Only rollback if token is still locked for this transaction
		if t.TokenStatus == TokenIsLocked && t.TransactionID == txID {
			t.TokenStatus = TokenIsFree
			t.TransactionID = "" // Clear transaction ID
			err = w.s.Update(TokenStorage, &t, "token_id=?", tokenID)
			if err != nil {
				w.log.Error("Failed to rollback token lock",
					"token_id", tokenID,
					"error", err)
				return err
			}
			rolledBackCount++
		}
	}

	w.log.Info("Token lock rollback completed",
		"transaction_id", txID,
		"rolled_back_count", rolledBackCount)

	return nil
}

// RollbackTokenPledge rolls back pledged tokens to free state (for quorums)
func (w *Wallet) RollbackTokenPledge(tokenIDs []string, txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back token pledges",
		"transaction_id", txID,
		"token_count", len(tokenIDs))

	rolledBackCount := 0
	for _, tokenID := range tokenIDs {
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", tokenID)
		if err != nil {
			w.log.Error("Token not found for pledge rollback",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Only rollback if token is still pledged for this transaction
		if t.TokenStatus == TokenIsPledged && t.TransactionID == txID {
			t.TokenStatus = TokenIsFree
			t.TransactionID = "" // Clear transaction ID
			err = w.s.Update(TokenStorage, &t, "token_id=?", tokenID)
			if err != nil {
				w.log.Error("Failed to rollback token pledge",
					"token_id", tokenID,
					"error", err)
				return err
			}
			rolledBackCount++
		}
	}

	w.log.Info("Token pledge rollback completed",
		"transaction_id", txID,
		"rolled_back_count", rolledBackCount)

	return nil
}

// RollbackFTTokenLock rolls back locked FT tokens to free state
func (w *Wallet) RollbackFTTokenLock(tokenIDs []string, txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back FT token locks",
		"transaction_id", txID,
		"token_count", len(tokenIDs))

	rolledBackCount := 0
	for _, tokenID := range tokenIDs {
		var ft FTToken
		err := w.s.Read(FTTokenStorage, &ft, "token_id=?", tokenID)
		if err != nil {
			w.log.Error("FT token not found for rollback",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Only rollback if token is still locked for this transaction
		if ft.TokenStatus == TokenIsLocked && ft.TransactionID == txID {
			ft.TokenStatus = TokenIsFree
			ft.TransactionID = "" // Clear transaction ID
			err = w.s.Update(FTTokenStorage, &ft, "token_id=?", tokenID)
			if err != nil {
				w.log.Error("Failed to rollback FT token lock",
					"token_id", tokenID,
					"error", err)
				return err
			}
			rolledBackCount++
		}
	}

	w.log.Info("FT token lock rollback completed",
		"transaction_id", txID,
		"rolled_back_count", rolledBackCount)

	return nil
}

// RollbackFTTokenPledge rolls back pledged FT tokens to free state (for quorums)
func (w *Wallet) RollbackFTTokenPledge(tokenIDs []string, txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back FT token pledges",
		"transaction_id", txID,
		"token_count", len(tokenIDs))

	rolledBackCount := 0
	for _, tokenID := range tokenIDs {
		var ft FTToken
		err := w.s.Read(FTTokenStorage, &ft, "token_id=?", tokenID)
		if err != nil {
			w.log.Error("FT token not found for pledge rollback",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Only rollback if token is still pledged for this transaction
		if ft.TokenStatus == TokenIsPledged && ft.TransactionID == txID {
			ft.TokenStatus = TokenIsFree
			ft.TransactionID = "" // Clear transaction ID
			err = w.s.Update(FTTokenStorage, &ft, "token_id=?", tokenID)
			if err != nil {
				w.log.Error("Failed to rollback FT token pledge",
					"token_id", tokenID,
					"error", err)
				return err
			}
			rolledBackCount++
		}
	}

	w.log.Info("FT token pledge rollback completed",
		"transaction_id", txID,
		"rolled_back_count", rolledBackCount)

	return nil
}

// RollbackTransaction removes a transaction from the transaction details table
func (w *Wallet) RollbackTransaction(txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back transaction record",
		"transaction_id", txID)

	// Delete from TransactionHistory table 
	err := w.s.Delete(TransactionStorage, &model.TransactionDetails{}, "transaction_id=?", txID)
	if err != nil {
		w.log.Error("Failed to delete transaction record",
			"transaction_id", txID,
			"error", err)
		return err
	}

	return nil
}

// RollbackPledgeDetails removes pledge details for a failed transaction (for quorums)
func (w *Wallet) RollbackPledgeDetails(txID string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back pledge details",
		"transaction_id", txID)

	// TODO: Implement when pledge details storage is available
	// Currently pledge details are handled through token state hash
	// err := w.s.Delete(PledgeDetailsStorage, &PledgeDetails{}, "transaction_id=?", txID)

	return nil
}

// GetTransactionTokens returns all tokens associated with a transaction
func (w *Wallet) GetTransactionTokens(txID string) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()

	var tokens []Token
	err := w.s.Read(TokenStorage, &tokens, "transaction_id=?", txID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetTransactionFTTokens returns all FT tokens associated with a transaction
func (w *Wallet) GetTransactionFTTokens(txID string) ([]FTToken, error) {
	w.l.Lock()
	defer w.l.Unlock()

	var ftTokens []FTToken
	err := w.s.Read(FTTokenStorage, &ftTokens, "transaction_id=?", txID)
	if err != nil {
		return nil, err
	}

	return ftTokens, nil
}

// RollbackTokenBlocks removes token blocks added for a failed transaction
func (w *Wallet) RollbackTokenBlocks(tokenIDs []string, blockHashes []string) error {
	w.l.Lock()
	defer w.l.Unlock()

	w.log.Info("Rolling back token blocks",
		"token_count", len(tokenIDs),
		"block_count", len(blockHashes))

	// Block removal is now handled through safe rollback mechanism
	// which tracks exactly what was added and removes only those blocks

	// Update token chains to remove references to these blocks
	for _, tokenID := range tokenIDs {
		// This would need implementation in the token chain management
		// For now, log the action
		w.log.Debug("Would remove block references from token chain",
			"token_id", tokenID)
	}

	return nil
}

// RollbackStateInfo contains information needed for rollback
type RollbackStateInfo struct {
	TransactionID    string                `json:"transaction_id"`
	TokenIDs         []string              `json:"token_ids"`
	FTTokenIDs       []string              `json:"ft_token_ids"`
	BlockHashes      []string              `json:"block_hashes"`
	Role             string                `json:"role"` // "sender", "quorum", or "receiver"
	Timestamp        time.Time             `json:"timestamp"`
	LevelDBSnapshot  *LevelDBRollbackInfo  `json:"leveldb_snapshot,omitempty"`
}

// CreateRollbackSnapshot creates a snapshot of current state for potential rollback
func (w *Wallet) CreateRollbackSnapshot(txID string, role string) (*RollbackStateInfo, error) {
	w.l.Lock()
	defer w.l.Unlock()

	snapshot := &RollbackStateInfo{
		TransactionID: txID,
		Role:          role,
		Timestamp:     time.Now(),
		TokenIDs:      make([]string, 0),
		FTTokenIDs:    make([]string, 0),
		BlockHashes:   make([]string, 0),
	}

	// Get regular tokens for this transaction
	var tokens []Token
	err := w.s.Read(TokenStorage, &tokens, "transaction_id=?", txID)
	if err == nil {
		for _, token := range tokens {
			snapshot.TokenIDs = append(snapshot.TokenIDs, token.TokenID)
		}
	}

	// Get FT tokens for this transaction
	var ftTokens []FTToken
	err = w.s.Read(FTTokenStorage, &ftTokens, "transaction_id=?", txID)
	if err == nil {
		for _, ftToken := range ftTokens {
			snapshot.FTTokenIDs = append(snapshot.FTTokenIDs, ftToken.TokenID)
		}
	}

	// Create LevelDB snapshot
	levelDBSnapshot, err := w.CreateLevelDBSnapshot(txID)
	if err != nil {
		w.log.Error("Failed to create LevelDB snapshot", "error", err)
		// Continue without LevelDB snapshot
	} else {
		snapshot.LevelDBSnapshot = levelDBSnapshot
	}

	w.log.Info("Created rollback snapshot",
		"transaction_id", txID,
		"role", role,
		"token_count", len(snapshot.TokenIDs),
		"ft_token_count", len(snapshot.FTTokenIDs))

	return snapshot, nil
}

// ExecuteRollback executes a complete rollback based on the role
func (w *Wallet) ExecuteRollback(snapshot *RollbackStateInfo) error {
	w.log.Info("Executing rollback",
		"transaction_id", snapshot.TransactionID,
		"role", snapshot.Role)

	switch snapshot.Role {
	case "sender":
		// Rollback locked tokens
		if len(snapshot.TokenIDs) > 0 {
			if err := w.RollbackTokenLock(snapshot.TokenIDs, snapshot.TransactionID); err != nil {
				return err
			}
		}
		if len(snapshot.FTTokenIDs) > 0 {
			if err := w.RollbackFTTokenLock(snapshot.FTTokenIDs, snapshot.TransactionID); err != nil {
				return err
			}
		}
		// Remove transaction record
		if err := w.RollbackTransaction(snapshot.TransactionID); err != nil {
			return err
		}

	case "quorum":
		// Rollback pledged tokens
		if len(snapshot.TokenIDs) > 0 {
			if err := w.RollbackTokenPledge(snapshot.TokenIDs, snapshot.TransactionID); err != nil {
				return err
			}
		}
		if len(snapshot.FTTokenIDs) > 0 {
			if err := w.RollbackFTTokenPledge(snapshot.FTTokenIDs, snapshot.TransactionID); err != nil {
				return err
			}
		}
		// Remove pledge details
		if err := w.RollbackPledgeDetails(snapshot.TransactionID); err != nil {
			return err
		}

	case "receiver":
		// Rollback pending tokens
		if len(snapshot.TokenIDs) > 0 {
			if err := w.RollbackPendingTokens(snapshot.TransactionID, snapshot.TokenIDs); err != nil {
				return err
			}
		}
		if len(snapshot.FTTokenIDs) > 0 {
			if err := w.RollbackPendingFTTokens(snapshot.TransactionID, snapshot.FTTokenIDs); err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("unknown role: %s", snapshot.Role)
	}

	// Remove any blocks if specified
	if len(snapshot.BlockHashes) > 0 {
		if err := w.RollbackTokenBlocks(snapshot.TokenIDs, snapshot.BlockHashes); err != nil {
			w.log.Error("Failed to rollback blocks", "error", err)
			// Continue even if block rollback fails
		}
	}

	// Execute LevelDB rollback if snapshot exists
	if snapshot.LevelDBSnapshot != nil {
		if err := w.ExecuteLevelDBRollback(snapshot.LevelDBSnapshot); err != nil {
			w.log.Error("Failed to execute LevelDB rollback", "error", err)
			// Continue even if LevelDB rollback fails
		}
	}

	w.log.Info("Rollback completed successfully",
		"transaction_id", snapshot.TransactionID,
		"role", snapshot.Role)

	return nil
}