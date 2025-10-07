package wallet

import (
	"fmt"
	"time"
)

// ConfirmPendingTokens updates tokens from pending to free status after consensus finality
func (w *Wallet) ConfirmPendingTokens(txID string, tokenIDs []string) error {
	w.l.Lock()
	defer w.l.Unlock()
	
	w.log.Info("Confirming pending tokens",
		"transaction_id", txID,
		"token_count", len(tokenIDs))
	
	confirmedCount := 0
	for _, tokenID := range tokenIDs {
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=? AND transaction_id=?", tokenID, txID)
		if err != nil {
			w.log.Error("Token not found for confirmation",
				"token_id", tokenID,
				"transaction_id", txID,
				"error", err)
			return fmt.Errorf("token not found: %s", tokenID)
		}
		
		if t.TokenStatus != TokenIsPending {
			// If token is already free, it's not an error - just skip it
			if t.TokenStatus == TokenIsFree {
				w.log.Debug("Token already confirmed",
					"token_id", tokenID,
					"current_status", t.TokenStatus)
				confirmedCount++
				continue
			}
			w.log.Warn("Token not in pending state",
				"token_id", tokenID,
				"current_status", t.TokenStatus,
				"expected_status", TokenIsPending)
			return fmt.Errorf("token %s not in pending state (current: %d)", tokenID, t.TokenStatus)
		}
		
		// Update to free status
		t.TokenStatus = TokenIsFree
		err = w.s.Update(TokenStorage, &t, "token_id=?", tokenID)
		if err != nil {
			w.log.Error("Failed to confirm token",
				"token_id", tokenID,
				"error", err)
			return err
		}
		confirmedCount++
	}
	
	w.log.Info("Successfully confirmed tokens",
		"transaction_id", txID,
		"confirmed_count", confirmedCount)
	
	return nil
}

// ConfirmPendingFTTokens updates FT tokens from pending to free status after consensus finality
func (w *Wallet) ConfirmPendingFTTokens(txID string, tokenIDs []string) error {
	w.l.Lock()
	defer w.l.Unlock()
	
	w.log.Info("Confirming pending FT tokens",
		"transaction_id", txID,
		"token_count", len(tokenIDs))
	
	confirmedCount := 0
	for _, tokenID := range tokenIDs {
		var ft FTToken
		err := w.s.Read(FTTokenStorage, &ft, "token_id=? AND transaction_id=?", tokenID, txID)
		if err != nil {
			w.log.Error("FT token not found for confirmation",
				"token_id", tokenID,
				"transaction_id", txID,
				"error", err)
			return fmt.Errorf("FT token not found: %s", tokenID)
		}
		
		if ft.TokenStatus != TokenIsPending {
			// If token is already free, it's not an error - just skip it
			if ft.TokenStatus == TokenIsFree {
				w.log.Debug("FT token already confirmed",
					"token_id", tokenID,
					"current_status", ft.TokenStatus)
				confirmedCount++
				continue
			}
			w.log.Warn("FT token not in pending state",
				"token_id", tokenID,
				"current_status", ft.TokenStatus,
				"expected_status", TokenIsPending)
			return fmt.Errorf("FT token %s not in pending state (current: %d)", tokenID, ft.TokenStatus)
		}
		
		// Update to free status
		ft.TokenStatus = TokenIsFree
		err = w.s.Update(FTTokenStorage, &ft, "token_id=?", tokenID)
		if err != nil {
			w.log.Error("Failed to confirm FT token",
				"token_id", tokenID,
				"error", err)
			return err
		}
		confirmedCount++
	}
	
	w.log.Info("Successfully confirmed FT tokens",
		"transaction_id", txID,
		"confirmed_count", confirmedCount)
	
	return nil
}

// RollbackPendingTokens removes pending tokens if consensus fails
func (w *Wallet) RollbackPendingTokens(txID string, tokenIDs []string) error {
	w.l.Lock()
	defer w.l.Unlock()
	
	w.log.Info("Rolling back pending tokens",
		"transaction_id", txID,
		"token_count", len(tokenIDs))
	
	rolledBackCount := 0
	for _, tokenID := range tokenIDs {
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=? AND transaction_id=?", tokenID, txID)
		if err != nil {
			// Token not found, might already be cleaned up
			w.log.Debug("Token not found for rollback",
				"token_id", tokenID,
				"transaction_id", txID)
			continue
		}
		
		if t.TokenStatus != TokenIsPending {
			w.log.Debug("Token not in pending state, skipping rollback",
				"token_id", tokenID,
				"current_status", t.TokenStatus)
			continue
		}
		
		// Remove the pending token
		err = w.s.Delete(TokenStorage, &Token{}, "token_id=? AND transaction_id=?", tokenID, txID)
		if err != nil {
			w.log.Error("Failed to rollback token",
				"token_id", tokenID,
				"error", err)
			return err
		}
		rolledBackCount++
	}
	
	w.log.Info("Successfully rolled back tokens",
		"transaction_id", txID,
		"rolled_back_count", rolledBackCount)
	
	return nil
}

// RollbackPendingFTTokens removes pending FT tokens if consensus fails
func (w *Wallet) RollbackPendingFTTokens(txID string, tokenIDs []string) error {
	w.l.Lock()
	defer w.l.Unlock()
	
	w.log.Info("Rolling back pending FT tokens",
		"transaction_id", txID,
		"token_count", len(tokenIDs))
	
	rolledBackCount := 0
	for _, tokenID := range tokenIDs {
		var ft FTToken
		err := w.s.Read(FTTokenStorage, &ft, "token_id=? AND transaction_id=?", tokenID, txID)
		if err != nil {
			// Token not found, might already be cleaned up
			w.log.Debug("FT token not found for rollback",
				"token_id", tokenID,
				"transaction_id", txID)
			continue
		}
		
		if ft.TokenStatus != TokenIsPending {
			w.log.Debug("FT token not in pending state, skipping rollback",
				"token_id", tokenID,
				"current_status", ft.TokenStatus)
			continue
		}
		
		// Remove the pending token
		err = w.s.Delete(FTTokenStorage, &FTToken{}, "token_id=? AND transaction_id=?", tokenID, txID)
		if err != nil {
			w.log.Error("Failed to rollback FT token",
				"token_id", tokenID,
				"error", err)
			return err
		}
		rolledBackCount++
	}
	
	w.log.Info("Successfully rolled back FT tokens",
		"transaction_id", txID,
		"rolled_back_count", rolledBackCount)
	
	return nil
}

// CleanupExpiredPendingTokens removes pending tokens older than the specified duration
func (w *Wallet) CleanupExpiredPendingTokens(expiry time.Duration) error {
	w.l.Lock()
	defer w.l.Unlock()
	
	expiryTime := time.Now().Add(-expiry)
	w.log.Info("Cleaning up expired pending tokens",
		"expiry_time", expiryTime)
	
	// Clean up regular tokens
	var pendingTokens []Token
	err := w.s.Read(TokenStorage, &pendingTokens, 
		"token_status=? AND created_at<?", TokenIsPending, expiryTime)
	
	if err == nil {
		for _, t := range pendingTokens {
			err = w.s.Delete(TokenStorage, &Token{}, "token_id=?", t.TokenID)
			if err != nil {
				w.log.Error("Failed to cleanup pending token",
					"token_id", t.TokenID,
					"error", err)
			}
		}
		w.log.Info("Cleaned up expired pending tokens",
			"count", len(pendingTokens))
	}
	
	// Clean up FT tokens
	var pendingFTTokens []FTToken
	err = w.s.Read(FTTokenStorage, &pendingFTTokens,
		"token_status=? AND created_at<?", TokenIsPending, expiryTime)
	
	if err == nil {
		for _, ft := range pendingFTTokens {
			err = w.s.Delete(FTTokenStorage, &FTToken{}, "token_id=?", ft.TokenID)
			if err != nil {
				w.log.Error("Failed to cleanup pending FT token",
					"token_id", ft.TokenID,
					"error", err)
			}
		}
		w.log.Info("Cleaned up expired pending FT tokens",
			"count", len(pendingFTTokens))
	}
	
	return nil
}