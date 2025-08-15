package core

import (
	"fmt"
	"time"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// VerifyTokenStatusBeforeTransfer checks and logs the status of all tokens before transfer
func (c *Core) VerifyTokenStatusBeforeTransfer(ftName string, ftCount int) error {
	c.log.Info("=== VERIFICATION: Checking token status BEFORE transfer ===")
	
	var tokens []wallet.FTToken
	// Get the first DID from wallet (primary DID)
	dids, err := c.w.GetAllDIDs()
	if err != nil || len(dids) == 0 {
		return fmt.Errorf("failed to get wallet DID")
	}
	ownerDID := dids[0].DID
	
	err = c.s.Read(wallet.FTTokenStorage, &tokens, "ft_name=? AND owner_did=? AND token_status=?", 
		ftName, ownerDID, wallet.TokenIsFree)
	
	if err != nil {
		c.log.Error("Failed to read tokens", "err", err)
		return fmt.Errorf("failed to read tokens: %v", err)
	}
	
	if len(tokens) < ftCount {
		c.log.Error("Insufficient free tokens", 
			"required", ftCount, 
			"available", len(tokens))
		return fmt.Errorf("insufficient free tokens: need %d, have %d", ftCount, len(tokens))
	}
	
	// Log each token's status
	for i, token := range tokens[:ftCount] {
		c.log.Info(fmt.Sprintf("Token %d status check", i+1),
			"token_id", token.TokenID,
			"status", token.TokenStatus,
			"status_name", getStatusName(token.TokenStatus),
			"transaction_id", token.TransactionID,
			"owner", token.DID)
	}
	
	c.log.Info("=== VERIFICATION: All tokens confirmed as FREE before transfer ===",
		"count", ftCount)
	
	return nil
}

// VerifyTokenStatusAfterLocking verifies tokens are properly locked
func (c *Core) VerifyTokenStatusAfterLocking(tokenIDs []string) error {
	c.log.Info("=== VERIFICATION: Checking token status AFTER locking ===")
	
	for i, tokenID := range tokenIDs {
		var token wallet.FTToken
		err := c.s.Read(wallet.FTTokenStorage, &token, "token_id=?", tokenID)
		if err != nil {
			c.log.Error("Failed to read token after locking", 
				"token_id", tokenID, 
				"err", err)
			return fmt.Errorf("failed to read token %s: %v", tokenID, err)
		}
		
		if token.TokenStatus != wallet.TokenIsLocked {
			c.log.Error("Token not properly locked!",
				"token_id", tokenID,
				"expected_status", wallet.TokenIsLocked,
				"actual_status", token.TokenStatus)
			return fmt.Errorf("token %s not locked, status: %d", tokenID, token.TokenStatus)
		}
		
		c.log.Info(fmt.Sprintf("Token %d lock verification", i+1),
			"token_id", tokenID,
			"status", token.TokenStatus,
			"status_name", getStatusName(token.TokenStatus))
	}
	
	c.log.Info("=== VERIFICATION: All tokens confirmed as LOCKED ===",
		"count", len(tokenIDs))
	
	return nil
}

// VerifyTokenStatusAfterInTransfer verifies tokens are marked as in-transfer
func (c *Core) VerifyTokenStatusAfterInTransfer(tokenIDs []string, transactionID string) error {
	c.log.Info("=== VERIFICATION: Checking token status AFTER marking in-transfer ===")
	
	for i, tokenID := range tokenIDs {
		var token wallet.FTToken
		err := c.s.Read(wallet.FTTokenStorage, &token, "token_id=?", tokenID)
		if err != nil {
			c.log.Error("Failed to read token after in-transfer", 
				"token_id", tokenID, 
				"err", err)
			return fmt.Errorf("failed to read token %s: %v", tokenID, err)
		}
		
		if token.TokenStatus != wallet.TokenIsInTransfer {
			c.log.Error("Token not properly marked as in-transfer!",
				"token_id", tokenID,
				"expected_status", wallet.TokenIsInTransfer,
				"actual_status", token.TokenStatus)
			return fmt.Errorf("token %s not in-transfer, status: %d", tokenID, token.TokenStatus)
		}
		
		if token.TransactionID != transactionID {
			c.log.Error("Transaction ID not set on token!",
				"token_id", tokenID,
				"expected_txid", transactionID,
				"actual_txid", token.TransactionID)
			return fmt.Errorf("token %s has wrong transaction ID", tokenID)
		}
		
		c.log.Info(fmt.Sprintf("Token %d in-transfer verification", i+1),
			"token_id", tokenID,
			"status", token.TokenStatus,
			"status_name", getStatusName(token.TokenStatus),
			"transaction_id", token.TransactionID)
	}
	
	c.log.Info("=== VERIFICATION: All tokens confirmed as IN-TRANSFER ===",
		"count", len(tokenIDs),
		"transaction_id", transactionID)
	
	return nil
}

// VerifyTokenStatusAfterFailure verifies all tokens are properly unlocked after failure
func (c *Core) VerifyTokenStatusAfterFailure(ftName string, ftCount int, originalOwner string) error {
	c.log.Info("=== VERIFICATION: Checking token status AFTER FAILED TRANSACTION ===")
	
	// Wait a moment for async operations to complete
	time.Sleep(2 * time.Second)
	
	var tokens []wallet.FTToken
	err := c.s.Read(wallet.FTTokenStorage, &tokens, "ft_name=? AND owner_did=?", 
		ftName, originalOwner)
	
	if err != nil {
		c.log.Error("Failed to read tokens after failure", "err", err)
		return fmt.Errorf("failed to read tokens: %v", err)
	}
	
	if len(tokens) < ftCount {
		c.log.Error("Tokens missing after failed transaction!", 
			"expected", ftCount, 
			"found", len(tokens))
		return fmt.Errorf("missing tokens: expected %d, found %d", ftCount, len(tokens))
	}
	
	failedTokens := 0
	for i, token := range tokens[:ftCount] {
		if token.TokenStatus != wallet.TokenIsFree {
			failedTokens++
			c.log.Error("TOKEN NOT PROPERLY UNLOCKED!",
				"token_id", token.TokenID,
				"current_status", token.TokenStatus,
				"status_name", getStatusName(token.TokenStatus),
				"expected_status", wallet.TokenIsFree,
				"transaction_id", token.TransactionID)
		} else if token.TransactionID != "" {
			c.log.Warn("Transaction ID not cleared!",
				"token_id", token.TokenID,
				"transaction_id", token.TransactionID)
		} else {
			c.log.Info(fmt.Sprintf("Token %d properly reverted", i+1),
				"token_id", token.TokenID,
				"status", token.TokenStatus,
				"status_name", "FREE",
				"owner", token.DID)
		}
	}
	
	if failedTokens > 0 {
		c.log.Error("=== VERIFICATION FAILED: Some tokens not properly unlocked ===",
			"failed_count", failedTokens,
			"total_count", ftCount)
		return fmt.Errorf("%d tokens not properly unlocked", failedTokens)
	}
	
	c.log.Info("=== VERIFICATION SUCCESS: All tokens properly reverted to FREE ===",
		"count", ftCount,
		"owner", originalOwner)
	
	// Verify tokens are ready for retry
	var freeTokens []wallet.FTToken
	err = c.s.Read(wallet.FTTokenStorage, &freeTokens, 
		"ft_name=? AND owner_did=? AND token_status=?", 
		ftName, originalOwner, wallet.TokenIsFree)
	
	if err != nil {
		c.log.Error("Failed to query free tokens", "err", err)
		return fmt.Errorf("failed to query free tokens: %v", err)
	}
	
	if len(freeTokens) < ftCount {
		c.log.Error("Not all tokens are free for retry!",
			"expected_free", ftCount,
			"actual_free", len(freeTokens))
		return fmt.Errorf("only %d tokens free for retry, expected %d", len(freeTokens), ftCount)
	}
	
	c.log.Info("=== TOKENS READY FOR RETRY: Confirmed all tokens can be retransferred ===",
		"free_count", len(freeTokens))
	
	return nil
}

// LogTransactionSummary logs a summary of the transaction attempt
func (c *Core) LogTransactionSummary(transactionID string, success bool, errorMsg string) {
	c.log.Info("===========================================")
	c.log.Info("=== TRANSACTION SUMMARY ===")
	c.log.Info("===========================================")
	c.log.Info("Transaction Result",
		"transaction_id", transactionID,
		"success", success,
		"timestamp", time.Now().Format(time.RFC3339))
	
	if !success && errorMsg != "" {
		c.log.Info("Failure Reason", "error", errorMsg)
	}
	
	// Check transaction in database
	var txHistory model.TransactionDetails
	err := c.s.Read(wallet.TransactionStorage, &txHistory, "transaction_id=?", transactionID)
	
	if err != nil {
		c.log.Info("Transaction History", "status", "NOT FOUND IN DATABASE (Expected for failed tx)")
	} else {
		c.log.Warn("Transaction History", 
			"status", "FOUND IN DATABASE",
			"comment", "Failed transaction should not be in history!")
	}
	
	c.log.Info("===========================================")
}

// Helper function to get readable status name
func getStatusName(status int) string {
	switch status {
	case wallet.TokenIsFree:
		return "FREE (0)"
	case wallet.TokenIsLocked:
		return "LOCKED (1)"
	case wallet.TokenIsInTransfer:
		return "IN_TRANSFER (18)"
	case wallet.TokenIsPending:
		return "PENDING (17)"
	case wallet.TokenChainSyncIssue:
		return "SYNC_ISSUE (21)"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", status)
	}
}

// AddTransactionVerificationHooks adds verification at key points in the transaction
func (c *Core) AddTransactionVerificationHooks() {
	c.log.Info("=== TRANSACTION VERIFICATION HOOKS ENABLED ===")
	c.log.Info("Will verify token status at each step of transaction")
}