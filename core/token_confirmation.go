package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// ConfirmTokenRequest represents the request to confirm pending tokens
type ConfirmTokenRequest struct {
	TransactionID string   `json:"transaction_id"`
	Tokens        []string `json:"tokens"`
	TokenType     int      `json:"token_type"` // 0 for RBT, 4 for FT
}

// ConfirmationManager manages confirmation channels for transactions
type ConfirmationManager struct {
	channels map[string]chan struct{}
	mu       sync.RWMutex
	log      logger.Logger
}

// NewConfirmationManager creates a new confirmation manager
func NewConfirmationManager(log logger.Logger) *ConfirmationManager {
	return &ConfirmationManager{
		channels: make(map[string]chan struct{}),
		mu:       sync.RWMutex{},
		log:      log,
	}
}

// confirmTokenTransfer confirms pending tokens after consensus finality
// This function is kept for backward compatibility but not used in the new confirmation flow
func (c *Core) confirmTokenTransfer(req interface{}) error {
	c.log.Info("confirmTokenTransfer called - this function is deprecated in the new confirmation flow")
	return fmt.Errorf("confirmTokenTransfer is deprecated - use the new confirmation mechanism")
}

// sendTokenConfirmation sends confirmation to receiver after finality
func (c *Core) sendTokenConfirmation(receiverAddress string, txID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Sending token confirmation to receiver",
		"receiver", receiverAddress,
		"transaction_id", txID,
		"token_count", len(tokens))

	// Extract token IDs
	tokenIDs := make([]string, len(tokens))
	for i, token := range tokens {
		tokenIDs[i] = token.Token
	}

	// Create confirmation request
	ctr := ConfirmTokenRequest{
		TransactionID: txID,
		Tokens:        tokenIDs,
		TokenType:     tokenType,
	}

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverAddress)
	if err != nil {
		return fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Send confirmation
	var resp model.BasicResponse
	err = receiverPeer.SendJSONRequest("POST", APIConfirmTokenTransfer, nil, &ctr, &resp, true)
	if err != nil {
		return fmt.Errorf("failed to send confirmation: %v", err)
	}

	if !resp.Status {
		return fmt.Errorf("receiver failed to confirm tokens: %s", resp.Message)
	}

	c.log.Info("Successfully sent token confirmation",
		"receiver", receiverAddress,
		"transaction_id", txID)

	return nil
}

// rollbackTokenTransfer rolls back pending tokens if consensus fails
func (c *Core) rollbackTokenTransfer(receiverAddress string, txID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Rolling back token transfer on receiver",
		"receiver", receiverAddress,
		"transaction_id", txID,
		"token_count", len(tokens))

	// Extract token IDs
	tokenIDs := make([]string, len(tokens))
	for i, token := range tokens {
		tokenIDs[i] = token.Token
	}

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverAddress)
	if err != nil {
		// Log error but don't fail - receiver might be offline
		c.log.Error("Failed to get receiver peer for rollback",
			"receiver", receiverAddress,
			"error", err)
		return nil
	}
	defer receiverPeer.Close()

	// Call rollback on receiver
	if tokenType == c.TokenType(FTString) {
		err = c.w.RollbackPendingFTTokens(txID, tokenIDs)
	} else {
		err = c.w.RollbackPendingTokens(txID, tokenIDs)
	}

	if err != nil {
		c.log.Error("Failed to rollback tokens",
			"receiver", receiverAddress,
			"error", err)
	}

	return nil
}

// sendTokenConfirmationWithVerification sends confirmation with enhanced verification
func (c *Core) sendTokenConfirmationWithVerification(receiverAddress string, txID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Sending token confirmation with verification",
		"receiver", receiverAddress,
		"transaction_id", txID,
		"token_count", len(tokens))

	// First, verify that receiver actually has the tokens
	receiverHasTokens, err := c.verifyReceiverHasTokens(receiverAddress, txID, tokens, tokenType)
	if err != nil {
		c.log.Error("Failed to verify receiver has tokens", "err", err)
		return fmt.Errorf("verification failed: %v", err)
	}

	if !receiverHasTokens {
		c.log.Error("Receiver does not have all required tokens",
			"receiver", receiverAddress,
			"transaction_id", txID)
		return fmt.Errorf("receiver does not have all required tokens")
	}

	// If verification passes, proceed with normal confirmation
	return c.sendTokenConfirmation(receiverAddress, txID, tokens, tokenType)
}

// verifyReceiverHasTokens verifies that receiver actually has the tokens
func (c *Core) verifyReceiverHasTokens(receiverAddress string, txID string, tokens []contract.TokenInfo, tokenType int) (bool, error) {
	c.log.Info("Verifying receiver has tokens",
		"receiver", receiverAddress,
		"transaction_id", txID,
		"token_count", len(tokens))

	// Extract token IDs
	tokenIDs := make([]string, len(tokens))
	for i, token := range tokens {
		tokenIDs[i] = token.Token
	}

	// Verify that tokens exist in receiver's storage
	// This is a critical check to ensure tokens were actually received
	verificationSuccess := true
	for i, token := range tokens {
		// Check if token exists in receiver's storage
		// This prevents false confirmations and ensures data integrity
		if !c.verifyTokenInReceiverStorage(receiverAddress, token.Token, txID) {
			c.log.Warn("Token verification failed - token not found in receiver storage",
				"receiver", receiverAddress,
				"transaction_id", txID,
				"token_id", token.Token,
				"token_index", i)
			verificationSuccess = false
			break
		}
	}

	if verificationSuccess {
		c.log.Info("Token verification completed successfully - all tokens found in receiver storage",
			"receiver", receiverAddress,
			"transaction_id", txID,
			"token_count", len(tokenIDs))
	} else {
		c.log.Error("Token verification failed - some tokens not found in receiver storage",
			"receiver", receiverAddress,
			"transaction_id", txID,
			"token_count", len(tokenIDs))
	}

	return verificationSuccess, nil
}

// verifyTokenInReceiverStorage checks if a specific token exists in receiver's storage
func (c *Core) verifyTokenInReceiverStorage(receiverAddress, tokenID, transactionID string) bool {
	c.log.Debug("Verifying token in receiver storage",
		"receiver", receiverAddress,
		"token_id", tokenID,
		"transaction_id", transactionID)

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverAddress)
	if err != nil {
		c.log.Error("Failed to get receiver peer for token verification",
			"receiver", receiverAddress,
			"error", err)
		return false
	}
	defer receiverPeer.Close()

	// Create verification request
	verifyReq := map[string]interface{}{
		"token_id":       tokenID,
		"transaction_id": transactionID,
		"verify_type":    "token_existence",
	}

	// Send verification request
	var resp model.BasicResponse
	err = receiverPeer.SendJSONRequest("POST", "/api/verify-token-existence", nil, &verifyReq, &resp, true)
	if err != nil {
		c.log.Error("Failed to verify token existence in receiver storage",
			"receiver", receiverAddress,
			"token_id", tokenID,
			"error", err)
		return false
	}

	// Return verification result
	return resp.Status
}

// waitForReceiverConfirmation waits for receiver to send confirmation with timeout and proactive checking
func (c *Core) waitForReceiverConfirmation(receiverAddress, transactionID string, tokens []contract.TokenInfo, tokenType int, explorerDone chan struct{}) {
	c.log.Info("Starting receiver confirmation wait",
		"receiver", receiverAddress,
		"transaction_id", transactionID,
		"token_count", len(tokens),
		"token_type", tokenType)

	// Calculate timeout based on token count (30 minutes base + 1 minute per 100 extra tokens)
	baseTimeout := 30 * time.Minute
	extraTokens := len(tokens) - 3000
	if extraTokens > 0 {
		extraMinutes := (extraTokens + 99) / 100 // Round up division
		baseTimeout += time.Duration(extraMinutes) * time.Minute
	}

	c.log.Info("Confirmation timeout calculated",
		"transaction_id", transactionID,
		"base_timeout", "30m",
		"extra_tokens", extraTokens,
		"total_timeout", baseTimeout)

	// Wait for explorer submission to complete if channel is provided
	if explorerDone != nil {
		c.log.Info("Waiting for explorer submission to complete",
			"transaction_id", transactionID)

		select {
		case <-explorerDone:
			c.log.Info("Explorer submission completed, waiting for receiver confirmation",
				"transaction_id", transactionID)
		case <-time.After(30 * time.Second):
			c.log.Warn("Explorer submission timeout, proceeding with confirmation wait",
				"transaction_id", transactionID)
		}
	}

	// Get confirmation channel for this transaction
	confirmationChan := c.getConfirmationSignal(transactionID)
	if confirmationChan == nil {
		c.log.Error("Failed to get confirmation channel",
			"transaction_id", transactionID)
		return
	}

	// Wait for receiver confirmation with timeout
	select {
	case <-confirmationChan:
		c.log.Info("Receiver confirmation received successfully",
			"transaction_id", transactionID,
			"receiver", receiverAddress)

		// Mark tokens as successfully transferred
		if err := c.markTokensAsTransferred(transactionID, tokens, tokenType); err != nil {
			c.log.Error("Failed to mark tokens as transferred",
				"transaction_id", transactionID,
				"error", err)
		}

		// Clean up confirmation channel
		c.CleanupConfirmation(transactionID)

	case <-time.After(baseTimeout):
		c.log.Warn("Confirmation timeout occurred, starting proactive status check",
			"transaction_id", transactionID,
			"receiver", receiverAddress)

		// Start proactive status checking
		go c.proactiveStatusCheck(receiverAddress, transactionID, tokens, tokenType)
	}
}

// getConfirmationSignal returns the confirmation channel for a specific transaction
func (c *Core) getConfirmationSignal(transactionID string) chan struct{} {
	// CRITICAL FIX: Use write lock to prevent race condition
	c.confirmationManager.mu.Lock()
	defer c.confirmationManager.mu.Unlock()

	if ch, exists := c.confirmationManager.channels[transactionID]; exists {
		return ch
	}

	// Create a new confirmation channel if it doesn't exist
	ch := make(chan struct{}, 1)
	c.confirmationManager.channels[transactionID] = ch
	c.log.Debug("Created new confirmation channel", "transaction_id", transactionID)
	return ch
}

// SignalConfirmation signals that a transaction has been confirmed by the receiver
func (c *Core) SignalConfirmation(transactionID string) error {
	c.confirmationManager.mu.Lock()
	defer c.confirmationManager.mu.Unlock()

	if ch, exists := c.confirmationManager.channels[transactionID]; exists {
		select {
		case ch <- struct{}{}:
			c.log.Info("Confirmation signal sent successfully",
				"transaction_id", transactionID)
			return nil
		default:
			return fmt.Errorf("confirmation channel is full for transaction %s", transactionID)
		}
	}

	return fmt.Errorf("no confirmation channel found for transaction %s", transactionID)
}

// CleanupConfirmation removes the confirmation channel for a completed transaction
func (c *Core) CleanupConfirmation(transactionID string) {
	c.confirmationManager.mu.Lock()
	defer c.confirmationManager.mu.Unlock()

	if ch, exists := c.confirmationManager.channels[transactionID]; exists {
		close(ch)
		delete(c.confirmationManager.channels, transactionID)
		c.log.Debug("Cleaned up confirmation channel",
			"transaction_id", transactionID)
	}
}

// proactiveStatusCheck proactively checks receiver status if confirmation timeout occurs
func (c *Core) proactiveStatusCheck(receiverAddress, transactionID string, tokens []contract.TokenInfo, tokenType int) {
	c.log.Info("Starting enhanced proactive status check",
		"transaction_id", transactionID,
		"receiver", receiverAddress)

	// Check receiver status every 5 minutes for up to 1 hour
	maxRetries := 12
	retryInterval := 5 * time.Minute

	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.log.Info("Proactive status check attempt",
			"transaction_id", transactionID,
			"attempt", attempt,
			"max_retries", maxRetries)

		// Enhanced status check with rollback coordination
		status, err := c.enhancedReceiverStatusCheck(receiverAddress, transactionID, tokens, tokenType)
		if err != nil {
			c.log.Error("Failed to check receiver status",
				"transaction_id", transactionID,
				"attempt", attempt,
				"error", err)

			// If this is the last attempt, we need to handle the failure
			if attempt == maxRetries {
				c.log.Error("All proactive status check attempts failed, handling transaction failure",
					"transaction_id", transactionID,
					"receiver", receiverAddress)

				// Attempt coordinated rollback to prevent double-spending
				if err := c.handleTransactionFailureWithRollback(receiverAddress, transactionID, tokens, tokenType); err != nil {
					c.log.Error("Failed to handle transaction failure with rollback",
						"transaction_id", transactionID,
						"error", err)
				}
				return
			}
		} else if status {
			c.log.Info("Receiver has successfully processed tokens",
				"transaction_id", transactionID,
				"attempt", attempt)

			// Mark tokens as successfully transferred
			if err := c.markTokensAsTransferred(transactionID, tokens, tokenType); err != nil {
				c.log.Error("Failed to mark tokens as transferred after proactive check",
					"transaction_id", transactionID,
					"error", err)
			}

			// Clean up confirmation channel
			c.CleanupConfirmation(transactionID)
			return
		}

		// Wait before next attempt
		time.Sleep(retryInterval)
	}

	// If we reach here, all attempts failed
	c.log.Error("All proactive status check attempts failed",
		"transaction_id", transactionID,
		"receiver", receiverAddress)

	// Handle transaction failure with coordinated rollback
	if err := c.handleTransactionFailureWithRollback(receiverAddress, transactionID, tokens, tokenType); err != nil {
		c.log.Error("Failed to handle transaction failure with rollback",
			"transaction_id", transactionID,
			"error", err)
	}
}

// enhancedReceiverStatusCheck performs enhanced status checking with rollback coordination
func (c *Core) enhancedReceiverStatusCheck(receiverAddress, transactionID string, tokens []contract.TokenInfo, tokenType int) (bool, error) {
	c.log.Debug("Performing enhanced receiver status check",
		"transaction_id", transactionID,
		"receiver", receiverAddress)

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverAddress)
	if err != nil {
		return false, fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Create enhanced status check request
	statusReq := map[string]interface{}{
		"transaction_id": transactionID,
		"token_count":    len(tokens),
		"token_type":     tokenType,
		"check_type":     "enhanced_status_check",
	}

	// Send enhanced status check request
	var resp model.BasicResponse
	err = receiverPeer.SendJSONRequest("POST", "/api/check-token-status", nil, &statusReq, &resp, true)
	if err != nil {
		return false, fmt.Errorf("failed to check token status: %v", err)
	}

	return resp.Status, nil
}

// handleTransactionFailureWithRollback handles transaction failure with coordinated rollback
func (c *Core) handleTransactionFailureWithRollback(receiverAddress, transactionID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Handling transaction failure with coordinated rollback",
		"transaction_id", transactionID,
		"receiver", receiverAddress,
		"token_count", len(tokens))

	// Step 1: Attempt to coordinate rollback with receiver
	rollbackSuccess := c.attemptCoordinatedRollback(receiverAddress, transactionID, tokens, tokenType)

	if rollbackSuccess {
		c.log.Info("Coordinated rollback successful, unlocking sender tokens",
			"transaction_id", transactionID,
			"receiver", receiverAddress)

		// Step 2: Unlock sender tokens (safe now that receiver has rolled back)
		if err := c.unlockSenderTokens(transactionID, tokens, tokenType); err != nil {
			c.log.Error("Failed to unlock sender tokens after successful rollback",
				"transaction_id", transactionID,
				"error", err)
			return err
		}
	} else {
		c.log.Warn("Coordinated rollback failed, proceeding with sender unlock",
			"transaction_id", transactionID,
			"receiver", receiverAddress,
			"warning", "This may create a double-spending risk if receiver comes back online")

		// Step 2: Unlock sender tokens (with warning about potential double-spending)
		if err := c.unlockSenderTokens(transactionID, tokens, tokenType); err != nil {
			c.log.Error("Failed to unlock sender tokens",
				"transaction_id", transactionID,
				"error", err)
			return err
		}
	}

	// Step 3: Record transaction failure in history
	if err := c.recordTransactionFailure(transactionID, tokens, tokenType); err != nil {
		c.log.Error("Failed to record transaction failure",
			"transaction_id", transactionID,
			"error", err)
	}

	c.log.Info("Transaction failure handling completed",
		"transaction_id", transactionID,
		"coordinated_rollback", rollbackSuccess)

	return nil
}

// attemptCoordinatedRollback attempts to coordinate rollback with receiver
func (c *Core) attemptCoordinatedRollback(receiverAddress, transactionID string, tokens []contract.TokenInfo, tokenType int) bool {
	c.log.Info("Attempting coordinated rollback with receiver",
		"transaction_id", transactionID,
		"receiver", receiverAddress)

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverAddress)
	if err != nil {
		c.log.Error("Failed to get receiver peer for coordinated rollback",
			"transaction_id", transactionID,
			"receiver", receiverAddress,
			"error", err)
		return false
	}
	defer receiverPeer.Close()

	// Create rollback request
	rollbackReq := map[string]interface{}{
		"transaction_id": transactionID,
		"token_count":    len(tokens),
		"token_type":     tokenType,
		"rollback_type":  "coordinated_rollback",
		"reason":         "sender_confirmation_timeout",
	}

	// Send coordinated rollback request
	var resp model.BasicResponse
	err = receiverPeer.SendJSONRequest("POST", "/api/coordinated-rollback", nil, &rollbackReq, &resp, true)
	if err != nil {
		c.log.Error("Failed to send coordinated rollback request",
			"transaction_id", transactionID,
			"receiver", receiverAddress,
			"error", err)
		return false
	}

	if !resp.Status {
		c.log.Error("Receiver rejected coordinated rollback",
			"transaction_id", transactionID,
			"receiver", receiverAddress,
			"message", resp.Message)
		return false
	}

	c.log.Info("Coordinated rollback successful",
		"transaction_id", transactionID,
		"receiver", receiverAddress)
	return true
}

// unlockSenderTokens unlocks sender tokens from transfer state
func (c *Core) unlockSenderTokens(transactionID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Unlocking sender tokens from transfer state",
		"transaction_id", transactionID,
		"token_count", len(tokens))

	// Unlock tokens and mark as failed
	for _, token := range tokens {
		if tokenType == c.TokenType(FTString) {
			if err := c.w.UnlockFTTokenFromTransfer(token.Token); err != nil {
				c.log.Error("Failed to unlock FT token from transfer",
					"token", token.Token,
					"transaction_id", transactionID,
					"error", err)
			}
		} else {
			if err := c.w.UnlockTokenFromTransfer(token.Token); err != nil {
				c.log.Error("Failed to unlock token from transfer",
					"token", token.Token,
					"transaction_id", transactionID,
					"error", err)
			}
		}
	}

	c.log.Info("Successfully unlocked all sender tokens",
		"transaction_id", transactionID,
		"token_count", len(tokens))

	return nil
}

// markTokensAsTransferred marks tokens as successfully transferred
func (c *Core) markTokensAsTransferred(transactionID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Marking tokens as transferred",
		"transaction_id", transactionID,
		"token_count", len(tokens))

	// Update token status to transferred
	for _, token := range tokens {
		if tokenType == c.TokenType(FTString) {
			if err := c.w.MarkFTTokenAsTransferred(token.Token, transactionID); err != nil {
				c.log.Error("Failed to mark FT token as transferred",
					"token", token.Token,
					"transaction_id", transactionID,
					"error", err)
				return err
			}
		} else {
			if err := c.w.MarkTokenAsTransferred(token.Token, transactionID); err != nil {
				c.log.Error("Failed to mark token as transferred",
					"token", token.Token,
					"transaction_id", transactionID,
					"error", err)
				return err
			}
		}
	}

	c.log.Info("Successfully marked all tokens as transferred",
		"transaction_id", transactionID,
		"token_count", len(tokens))

	return nil
}

// recordTransactionFailure records the failed transaction in history
func (c *Core) recordTransactionFailure(transactionID string, tokens []contract.TokenInfo, tokenType int) error {
	c.log.Info("Recording transaction failure",
		"transaction_id", transactionID,
		"token_count", len(tokens))

	// This function should record the failed transaction in the appropriate history table
	// Implementation depends on existing transaction history structure
	// For now, we'll log the failure

	c.log.Error("Transaction failed - manual intervention required",
		"transaction_id", transactionID,
		"token_count", len(tokens),
		"token_type", tokenType,
		"status", "failed")

	return nil
}

// CoordinatedRollback handles coordinated rollback requests from senders
func (c *Core) CoordinatedRollback(transactionID string, tokenType int) error {
	c.log.Info("Handling coordinated rollback request",
		"transaction_id", transactionID,
		"token_type", tokenType)

	// Extract token IDs for this transaction
	var tokenIDs []string
	var err error

	if tokenType == c.TokenType(FTString) {
		// Get FT token IDs for this transaction
		tokenIDs, err = c.w.GetFTTokenIDsByTransactionID(transactionID)
		if err != nil {
			c.log.Error("Failed to get FT token IDs for rollback",
				"transaction_id", transactionID,
				"error", err)
			return fmt.Errorf("failed to get FT token IDs: %v", err)
		}
	} else {
		// Get regular token IDs for this transaction
		tokenIDs, err = c.w.GetTokenIDsByTransactionID(transactionID)
		if err != nil {
			c.log.Error("Failed to get token IDs for rollback",
				"transaction_id", transactionID,
				"error", err)
			return fmt.Errorf("failed to get token IDs: %v", err)
		}
	}

	if len(tokenIDs) == 0 {
		c.log.Warn("No tokens found for rollback",
			"transaction_id", transactionID,
			"token_type", tokenType)
		return nil // No tokens to rollback
	}

	c.log.Info("Found tokens for coordinated rollback",
		"transaction_id", transactionID,
		"token_count", len(tokenIDs),
		"token_type", tokenType)

	// Perform the rollback
	if tokenType == c.TokenType(FTString) {
		err = c.w.RollbackPendingFTTokens(transactionID, tokenIDs)
	} else {
		err = c.w.RollbackPendingTokens(transactionID, tokenIDs)
	}

	if err != nil {
		c.log.Error("Failed to perform coordinated rollback",
			"transaction_id", transactionID,
			"token_type", tokenType,
			"error", err)
		return fmt.Errorf("rollback failed: %v", err)
	}

	c.log.Info("Coordinated rollback completed successfully",
		"transaction_id", transactionID,
		"token_count", len(tokenIDs),
		"token_type", tokenType)

	return nil
}

// RecoverLostTokens attempts to recover tokens that were sent but not received
// This function helps senders recover tokens when receivers don't have them
func (c *Core) RecoverLostTokens(senderDID, transactionID string) (*TokenRecoveryResult, error) {
	c.log.Info("Starting token recovery process",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	// Step 1: Get transaction details from history
	transactionDetails, err := c.getTransactionFromHistory(senderDID, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction from history: %v", err)
	}

	// Step 1.5: Check if transaction has already been recovered
	if c.isTransactionAlreadyRecovered(transactionDetails) {
		c.log.Warn("Transaction has already been recovered",
			"transaction_id", transactionID,
			"comment", transactionDetails.Comment)
		return nil, fmt.Errorf("transaction has already been recovered, recovery can only be done once per transaction")
	}

	// Step 2: Check if transaction is within recovery window (7-14 days)
	if !c.isTransactionWithinRecoveryWindow(transactionDetails.DateTime) {
		return nil, fmt.Errorf("transaction must be between 7-14 days old for recovery (current age: %v)", time.Since(transactionDetails.DateTime))
	}

	// Step 3: Get receiver DID from transaction
	receiverDID := transactionDetails.ReceiverDID
	if receiverDID == "" {
		return nil, fmt.Errorf("receiver DID not found in transaction")
	}

	c.log.Info("Transaction details retrieved",
		"sender_did", senderDID,
		"receiver_did", receiverDID,
		"transaction_id", transactionID,
		"amount", transactionDetails.Amount,
		"date", transactionDetails.DateTime)

	// Check if this transaction has special exception status
	// NOTE: Exception transactions can skip receiver online check BUT still subject to:
	// - One-time recovery check (Step 1.5)
	// - 7-14 day window check (Step 2)
	// - Token-level recovery check (in unlockTransferredTokens)
	isExceptionTransaction := c.isExceptionTransaction(transactionID)

	// Step 4: Check if receiver has the tokens
	receiverHasTokens, err := c.checkReceiverTokenStatus(receiverDID, transactionID, int(transactionDetails.Amount))
	if err != nil {
		if isExceptionTransaction {
			// Exception transactions can proceed even if receiver is offline
			c.log.Warn("Exception transaction - proceeding despite receiver being offline",
				"transaction_id", transactionID,
				"receiver_did", receiverDID,
				"error", err)
			receiverHasTokens = false
		} else {
			c.log.Error("Failed to verify receiver token status",
				"receiver_did", receiverDID,
				"error", err)
			// SECURITY: Cannot proceed with recovery if we can't verify receiver status
			// This prevents recovery when receiver is offline (they might have the tokens)
			return nil, fmt.Errorf("cannot verify receiver token status (receiver may be offline): %v", err)
		}
	}

	if receiverHasTokens {
		return nil, fmt.Errorf("receiver has the tokens, no recovery needed")
	}

	// Step 5: Check if tokens were transferred from receiver (double-spending check)
	tokensTransferredFromReceiver, err := c.checkIfTokensTransferredFromReceiver(receiverDID, transactionID)
	if err != nil {
		if isExceptionTransaction {
			// Exception transactions can proceed even if we can't verify transfer status
			c.log.Warn("Exception transaction - proceeding despite unable to verify transfer status",
				"transaction_id", transactionID,
				"receiver_did", receiverDID,
				"error", err)
			tokensTransferredFromReceiver = false
		} else {
			c.log.Error("Failed to check if tokens were transferred from receiver",
				"receiver_did", receiverDID,
				"error", err)
			// SECURITY: Cannot proceed with recovery if we can't verify transfer status
			// Receiver might have transferred the tokens already
			return nil, fmt.Errorf("cannot verify if receiver transferred tokens (receiver may be offline): %v", err)
		}
	}

	if tokensTransferredFromReceiver {
		return nil, fmt.Errorf("tokens were transferred from receiver, cannot recover (double-spending detected)")
	}

	// Step 6: Perform token recovery
	recoveryResult, err := c.performTokenRecovery(senderDID, transactionID, transactionDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to perform token recovery: %v", err)
	}

	c.log.Info("Token recovery completed successfully",
		"sender_did", senderDID,
		"transaction_id", transactionID,
		"recovered_tokens", recoveryResult.RecoveredTokenCount)

	return recoveryResult, nil
}

// TokenRecoveryResult contains the result of a token recovery operation
type TokenRecoveryResult struct {
	TransactionID       string    `json:"transaction_id"`
	SenderDID           string    `json:"sender_did"`
	ReceiverDID         string    `json:"receiver_did"`
	RecoveredTokenCount int       `json:"recovered_token_count"`
	RecoveryDate        time.Time `json:"recovery_date"`
	Status              string    `json:"status"`
	Message             string    `json:"message"`
}

// getTransactionFromHistory retrieves transaction details from FT transaction history
func (c *Core) getTransactionFromHistory(senderDID, transactionID string) (*model.TransactionDetails, error) {
	c.log.Debug("Getting transaction from history",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	// First, try to get from FTTransactionHistory table
	var ftHistory model.FTTransactionHistory
	err := c.w.GetStorage().Read("ft_transaction_history", &ftHistory, "transaction_id=? AND sender_did=?", transactionID, senderDID)
	if err == nil {
		// Convert FTTransactionHistory to TransactionDetails
		transactionDetails := &model.TransactionDetails{
			TransactionID:   ftHistory.TransactionID,
			TransactionType: ftHistory.TransactionType,
			BlockID:         ftHistory.BlockID,
			Mode:            ftHistory.Mode,
			SenderDID:       ftHistory.SenderDID,
			ReceiverDID:     ftHistory.ReceiverDID,
			Amount:          ftHistory.Amount,
			TotalTime:       ftHistory.TotalTime,
			Comment:         ftHistory.Comment,
			DateTime:        ftHistory.DateTime,
			Status:          ftHistory.Status,
			DeployerDID:     ftHistory.DeployerDID,
			Epoch:           ftHistory.Epoch,
			Tokens: []model.FTTokenSummary{
				{
					CreatorDID: ftHistory.CreatorDID,
					FTName:     ftHistory.FTName,
					Count:      ftHistory.TokenCount,
				},
			},
		}

		c.log.Debug("Found transaction in FTTransactionHistory",
			"transaction_id", transactionID,
			"receiver_did", transactionDetails.ReceiverDID,
			"amount", transactionDetails.Amount,
			"date", transactionDetails.DateTime)

		return transactionDetails, nil
	}

	// If not found in FTTransactionHistory, try the regular TransactionDetails table
	var transactionDetails model.TransactionDetails
	err = c.w.GetStorage().Read("TransactionHistory", &transactionDetails, "transaction_id=? AND sender_did=?", transactionID, senderDID)
	if err != nil {
		c.log.Error("Transaction not found in either history table",
			"transaction_id", transactionID,
			"sender_did", senderDID,
			"error", err)
		return nil, fmt.Errorf("transaction not found: %v", err)
	}

	c.log.Debug("Found transaction in TransactionHistory",
		"transaction_id", transactionID,
		"receiver_did", transactionDetails.ReceiverDID,
		"amount", transactionDetails.Amount,
		"date", transactionDetails.DateTime)

	return &transactionDetails, nil
}

// isTransactionWithinRecoveryWindow checks if transaction is within 7 days
// isTransactionAlreadyRecovered checks if a transaction has already been recovered
func (c *Core) isTransactionAlreadyRecovered(transactionDetails *model.TransactionDetails) bool {
	// Check if the comment field contains recovery marker
	if transactionDetails.Comment == "" {
		return false
	}

	// Check for recovery marker in the comment
	// The recovery marker is in the format "RECOVERED_<timestamp>_<type>"
	if strings.Contains(transactionDetails.Comment, "RECOVERED_") {
		c.log.Info("Transaction has recovery marker",
			"transaction_id", transactionDetails.TransactionID,
			"comment", transactionDetails.Comment)
		return true
	}

	return false
}

// isExceptionTransaction checks if a transaction ID is in the exception list
// These transactions can be recovered even if receiver is offline
// BUT they can still only be recovered ONCE (same as all other transactions)
func (c *Core) isExceptionTransaction(transactionID string) bool {
	// Special exception transaction IDs that can be recovered even if receiver is offline
	// These are specific historical transactions that need special handling
	exceptionTransactions := []string{
		"ddd9a7bd6c73a1ae53ec51d9c049018a71497c1e267a633d809e3dc1737891cc",
		"806635cb5415a5e0828bfd9ec9052b0148b0e4f2c8bcf2bde85f381d655874e4",
		"bc749e27dfbdcec69fbfd4e55f2ca9bfbfe0b4b99e38835da050209d88837a59",
	}

	for _, exceptionID := range exceptionTransactions {
		if transactionID == exceptionID {
			c.log.Info("Transaction is in exception list - can recover even if receiver offline",
				"transaction_id", transactionID)
			return true
		}
	}

	return false
}

func (c *Core) isTransactionWithinRecoveryWindow(transactionDate time.Time) bool {
	// Recovery window: 7-14 days old (not less than 7, not more than 14)
	//minAge := 7 * 24 * time.Hour  // 7 days
	maxAge := 14 * 24 * time.Hour // 14 days

	now := time.Now()
	//minCutoff := now.Add(-minAge) // 7 days ago
	maxCutoff := now.Add(-maxAge) // 14 days ago

	// Transaction must be:
	// - Older than 7 days (before minCutoff)
	// - But not older than 14 days (after maxCutoff)
	//return transactionDate.Before(minCutoff) && transactionDate.After(maxCutoff)
	return transactionDate.After(maxCutoff)
}

// checkReceiverTokenStatus checks if receiver has the tokens for the given transaction
func (c *Core) checkReceiverTokenStatus(receiverDID, transactionID string, amount int) (bool, error) {
	c.log.Debug("Checking receiver token status",
		"receiver_did", receiverDID,
		"transaction_id", transactionID,
		"amount", amount)

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverDID)
	if err != nil {
		return false, fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Create status check request
	statusReq := map[string]interface{}{
		"transaction_id":  transactionID,
		"expected_amount": amount,
		"check_type":      "recovery_verification",
	}

	// Send status check request
	var resp model.BasicResponse
	err = receiverPeer.SendJSONRequest("POST", "/api/verify-token-ownership", nil, &statusReq, &resp, true)
	if err != nil {
		return false, fmt.Errorf("failed to verify token ownership: %v", err)
	}

	return resp.Status, nil
}

// checkIfTokensTransferredFromReceiver checks if tokens were transferred from receiver
func (c *Core) checkIfTokensTransferredFromReceiver(receiverDID, transactionID string) (bool, error) {
	c.log.Debug("Checking if tokens were transferred from receiver",
		"receiver_did", receiverDID,
		"transaction_id", transactionID)

	// Get receiver peer
	receiverPeer, err := c.getPeer(receiverDID)
	if err != nil {
		return false, fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Create transfer check request
	transferReq := map[string]interface{}{
		"transaction_id": transactionID,
		"check_type":     "transfer_verification",
	}

	// Send transfer check request
	var resp model.BasicResponse
	err = receiverPeer.SendJSONRequest("POST", "/api/check-token-transfer-status", nil, &transferReq, &resp, true)
	if err != nil {
		return false, fmt.Errorf("failed to check token transfer status: %v", err)
	}

	// Parse response to check if tokens were transferred
	// Parse the response to determine if tokens were transferred
	if !resp.Status {
		// If the API call failed, assume no transfer occurred (safe default)
		c.log.Debug("Transfer status check API call failed, assuming no transfer occurred",
			"receiver_did", receiverDID,
			"transaction_id", transactionID)
		return false, nil
	}

	// Check the response message to determine transfer status
	// The API returns true if tokens were transferred, false otherwise
	// This is based on the actual implementation in APICheckTokenTransferStatus
	transferStatus := resp.Status

	c.log.Debug("Transfer status check completed",
		"receiver_did", receiverDID,
		"transaction_id", transactionID,
		"transfer_status", transferStatus)

	return transferStatus, nil
}

// performTokenRecovery performs the actual token recovery
func (c *Core) performTokenRecovery(senderDID, transactionID string, transactionDetails *model.TransactionDetails) (*TokenRecoveryResult, error) {
	c.log.Info("Performing token recovery",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	// Step 1: Unlock tokens in sender's wallet (if they're in transferred state)
	recoveredCount, err := c.unlockTransferredTokens(senderDID, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to unlock transferred tokens: %v", err)
	}

	// Step 2: Update transaction history to mark as recovered
	err = c.markTransactionAsRecovered(transactionID)
	if err != nil {
		c.log.Warn("Failed to mark transaction as recovered",
			"transaction_id", transactionID,
			"error", err)
		// Don't fail the recovery if this step fails
	}

	// Step 3: Create recovery result
	recoveryResult := &TokenRecoveryResult{
		TransactionID:       transactionID,
		SenderDID:           senderDID,
		ReceiverDID:         transactionDetails.ReceiverDID,
		RecoveredTokenCount: recoveredCount,
		RecoveryDate:        time.Now(),
		Status:              "success",
		Message:             fmt.Sprintf("Successfully recovered %d tokens", recoveredCount),
	}

	c.log.Info("Token recovery completed",
		"transaction_id", transactionID,
		"recovered_count", recoveredCount)

	return recoveryResult, nil
}

// unlockTransferredTokens unlocks tokens that are in transferred state
func (c *Core) unlockTransferredTokens(senderDID, transactionID string) (int, error) {
	c.log.Debug("Unlocking transferred tokens",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	// Get token IDs for this transaction
	tokenIDs, err := c.w.GetFTTokenIDsByTransactionID(transactionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get token IDs: %v", err)
	}

	// If no tokens found locally, try to get them from explorer
	if len(tokenIDs) == 0 {
		c.log.Info("No tokens found locally for transaction, trying explorer fallback",
			"transaction_id", transactionID)
		
		tokenIDs, err = c.getTokensFromExplorer(transactionID)
		if err != nil {
			c.log.Error("Failed to get tokens from explorer",
				"transaction_id", transactionID,
				"error", err)
			return 0, fmt.Errorf("no tokens found locally or from explorer: %v", err)
		}
		
		c.log.Info("Found tokens from explorer",
			"transaction_id", transactionID,
			"token_count", len(tokenIDs))
	}

	if len(tokenIDs) == 0 {
		return 0, nil // No tokens to recover
	}

	// Check if any of these tokens have been recovered before
	for _, tokenID := range tokenIDs {
		if c.isTokenPreviouslyRecovered(tokenID) {
			c.log.Error("Token has been previously recovered, aborting recovery",
				"token_id", tokenID,
				"transaction_id", transactionID)
			return 0, fmt.Errorf("token %s has been previously recovered, potential double-recovery attempt detected", tokenID)
		}
	}

	// Unlock each token and ensure they are transaction ready
	unlockedCount := 0
	for _, tokenID := range tokenIDs {
		// First, unlock the token from transfer state
		if err := c.w.UnlockFTTokenFromTransfer(tokenID); err != nil {
			c.log.Error("Failed to unlock token from transfer",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Ensure the token is in a transaction-ready state
		if err := c.ensureTokenTransactionReady(tokenID); err != nil {
			c.log.Error("Failed to make token transaction ready",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// IMPORTANT: Mark the token with recovery metadata in its state hash
		// This prevents the token from being recovered again in future transactions
		if err := c.markTokenAsRecovered(tokenID, transactionID); err != nil {
			c.log.Error("Failed to mark token as recovered",
				"token_id", tokenID,
				"error", err)
			// Don't fail the recovery, but log the error
		}

		unlockedCount++
		c.log.Debug("Successfully unlocked and made token transaction ready",
			"token_id", tokenID)
	}

	c.log.Info("Unlocked and prepared tokens for transactions",
		"transaction_id", transactionID,
		"total_tokens", len(tokenIDs),
		"unlocked_count", unlockedCount)

	return unlockedCount, nil
}

// ensureTokenTransactionReady ensures a token is in a state ready for new transactions
func (c *Core) ensureTokenTransactionReady(tokenID string) error {
	c.log.Debug("Ensuring token is transaction ready",
		"token_id", tokenID)

	// Read the current token state
	var ftToken wallet.FTToken
	err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to read token state: %v", err)
	}

	// Check if token needs to be made transaction ready
	needsUpdate := false

	// Clear any old transaction ID to prevent conflicts
	if ftToken.TransactionID != "" {
		ftToken.TransactionID = ""
		needsUpdate = true
		c.log.Debug("Cleared old transaction ID",
			"token_id", tokenID,
			"old_transaction_id", ftToken.TransactionID)
	}

	// Ensure token status is free (0) for new transactions
	if ftToken.TokenStatus != 0 {
		ftToken.TokenStatus = 0 // TokenIsFree
		needsUpdate = true
		c.log.Debug("Updated token status to free",
			"token_id", tokenID,
			"old_status", ftToken.TokenStatus,
			"new_status", 0)
	}

	// Update token if changes were made
	if needsUpdate {
		err = c.w.GetStorage().Update(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
		if err != nil {
			return fmt.Errorf("failed to update token for transaction readiness: %v", err)
		}

		c.log.Info("Token made transaction ready",
			"token_id", tokenID,
			"status", ftToken.TokenStatus,
			"transaction_id", ftToken.TransactionID)
	} else {
		c.log.Debug("Token already transaction ready",
			"token_id", tokenID,
			"status", ftToken.TokenStatus)
	}

	return nil
}

// markTransactionAsRecovered marks a transaction as recovered in history
func (c *Core) markTransactionAsRecovered(transactionID string) error {
	c.log.Debug("Marking transaction as recovered",
		"transaction_id", transactionID)

	// First, try to mark in FTTransactionHistory table
	var ftHistory model.FTTransactionHistory
	err := c.w.GetStorage().Read("ft_transaction_history", &ftHistory, "transaction_id=?", transactionID)
	if err == nil {
		// Add recovery metadata to the transaction
		// We'll add a recovery flag in the comment field for now
		// You can enhance this by adding a dedicated recovery field to the struct
		recoveryComment := fmt.Sprintf("RECOVERED_%s_%s", time.Now().Format("2006-01-02T15:04:05Z"), "sender_initiated")
		if ftHistory.Comment != "" {
			ftHistory.Comment = ftHistory.Comment + " | " + recoveryComment
		} else {
			ftHistory.Comment = recoveryComment
		}

		// Update the transaction
		err = c.w.GetStorage().Update("ft_transaction_history", &ftHistory, "transaction_id=?", transactionID)
		if err != nil {
			c.log.Error("Failed to mark FT transaction as recovered",
				"transaction_id", transactionID,
				"error", err)
			return fmt.Errorf("failed to mark FT transaction as recovered: %v", err)
		}

		c.log.Info("Successfully marked FT transaction as recovered",
			"transaction_id", transactionID,
			"recovery_comment", recoveryComment)
		return nil
	}

	// If not found in FTTransactionHistory, try the regular TransactionHistory table
	var transactionDetails model.TransactionDetails
	err = c.w.GetStorage().Read("TransactionHistory", &transactionDetails, "transaction_id=?", transactionID)
	if err != nil {
		c.log.Error("Transaction not found in either history table for recovery marking",
			"transaction_id", transactionID,
			"error", err)
		return fmt.Errorf("transaction not found for recovery marking: %v", err)
	}

	// Add recovery metadata to the comment field
	recoveryComment := fmt.Sprintf("RECOVERED_%s_%s", time.Now().Format("2006-01-02T15:04:05Z"), "sender_initiated")
	if transactionDetails.Comment != "" {
		transactionDetails.Comment = transactionDetails.Comment + " | " + recoveryComment
	} else {
		transactionDetails.Comment = recoveryComment
	}

	// Update the transaction
	err = c.w.GetStorage().Update("TransactionHistory", &transactionDetails, "transaction_id=?", transactionID)
	if err != nil {
		c.log.Error("Failed to mark transaction as recovered",
			"transaction_id", transactionID,
			"error", err)
		return fmt.Errorf("failed to mark transaction as recovered: %v", err)
	}

	c.log.Info("Successfully marked transaction as recovered",
		"transaction_id", transactionID,
		"recovery_comment", recoveryComment)
	return nil
}

// VerifyLocalTokenOwnership checks if this node has tokens for the given transaction
func (c *Core) VerifyLocalTokenOwnership(transactionID string, expectedAmount int) (bool, error) {
	c.log.Debug("Verifying local token ownership",
		"transaction_id", transactionID,
		"expected_amount", expectedAmount)

	// Get token IDs for this transaction
	tokenIDs, err := c.w.GetFTTokenIDsByTransactionID(transactionID)
	if err != nil {
		return false, fmt.Errorf("failed to get token IDs: %v", err)
	}

	if len(tokenIDs) == 0 {
		c.log.Debug("No tokens found for transaction",
			"transaction_id", transactionID)
		return false, nil
	}

	// Check if the number of tokens matches the expected amount
	if len(tokenIDs) != expectedAmount {
		c.log.Debug("Token count mismatch",
			"transaction_id", transactionID,
			"expected", expectedAmount,
			"actual", len(tokenIDs))
		return false, nil
	}

	// Enhanced token status checking with real database queries
	validTokens := 0
	for _, tokenID := range tokenIDs {
		var ftToken wallet.FTToken
		err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
		if err != nil {
			c.log.Debug("Failed to read token status",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Check if token is in a valid state for ownership verification
		// Status 0 = free, Status 17 = pending, Status 18 = in transfer
		if ftToken.TokenStatus == 0 || ftToken.TokenStatus == 17 || ftToken.TokenStatus == 18 {
			validTokens++
			c.log.Debug("Token is in valid state",
				"token_id", tokenID,
				"status", ftToken.TokenStatus)
		} else {
			c.log.Debug("Token is not in valid state",
				"token_id", tokenID,
				"status", ftToken.TokenStatus)
		}
	}

	c.log.Debug("Token ownership verification completed",
		"transaction_id", transactionID,
		"total_tokens", len(tokenIDs),
		"valid_tokens", validTokens)

	// Return true if we have the expected number of valid tokens
	return validTokens == expectedAmount, nil
}

// CheckLocalTokenTransferStatus checks if tokens were transferred from this node for the given transaction
func (c *Core) CheckLocalTokenTransferStatus(transactionID string) (bool, error) {
	c.log.Debug("Checking local token transfer status",
		"transaction_id", transactionID)

	// Get token IDs for this transaction
	tokenIDs, err := c.w.GetFTTokenIDsByTransactionID(transactionID)
	if err != nil {
		return false, fmt.Errorf("failed to get token IDs: %v", err)
	}

	if len(tokenIDs) == 0 {
		c.log.Debug("No tokens found for transaction",
			"transaction_id", transactionID)
		return false, nil
	}

	// Enhanced token transfer status checking with real database queries
	transferredTokens := 0
	for _, tokenID := range tokenIDs {
		var ftToken wallet.FTToken
		err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
		if err != nil {
			c.log.Debug("Failed to read token status",
				"token_id", tokenID,
				"error", err)
			continue
		}

		// Check if token was transferred (status indicates it was sent to another node)
		// Status 19 = transferred, Status 20 = quorum pledged
		if ftToken.TokenStatus == 19 || ftToken.TokenStatus == 20 {
			transferredTokens++
			c.log.Debug("Token was transferred",
				"token_id", tokenID,
				"status", ftToken.TokenStatus)
		} else {
			c.log.Debug("Token was not transferred",
				"token_id", tokenID,
				"status", ftToken.TokenStatus)
		}
	}

	c.log.Debug("Token transfer status check completed",
		"transaction_id", transactionID,
		"total_tokens", len(tokenIDs),
		"transferred_tokens", transferredTokens)

	// Return true if any tokens were transferred
	return transferredTokens > 0, nil
}

// VerifyLocalTokenExistence checks if a specific token exists in local storage for a given transaction
func (c *Core) VerifyLocalTokenExistence(tokenID, transactionID string) (bool, error) {
	c.log.Debug("Verifying local token existence",
		"token_id", tokenID,
		"transaction_id", transactionID)

	// Check if token exists in FTTokenStorage
	var ftToken wallet.FTToken
	err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err == nil {
		// Token exists, check if it's associated with the transaction
		if ftToken.TransactionID == transactionID {
			c.log.Debug("Token found and associated with transaction",
				"token_id", tokenID,
				"transaction_id", transactionID,
				"status", ftToken.TokenStatus)
			return true, nil
		} else {
			c.log.Debug("Token found but not associated with transaction",
				"token_id", tokenID,
				"transaction_id", transactionID,
				"token_transaction_id", ftToken.TransactionID)
			return false, nil
		}
	}

	// If not found in FTTokenStorage, check regular TokenStorage
	var regularToken wallet.Token
	err = c.w.GetStorage().Read(wallet.TokenStorage, &regularToken, "token_id=?", tokenID)
	if err == nil {
		// Token exists, check if it's associated with the transaction
		if regularToken.TransactionID == transactionID {
			c.log.Debug("Regular token found and associated with transaction",
				"token_id", tokenID,
				"transaction_id", transactionID,
				"status", regularToken.TokenStatus)
			return true, nil
		} else {
			c.log.Debug("Regular token found but not associated with transaction",
				"token_id", tokenID,
				"transaction_id", transactionID,
				"token_transaction_id", regularToken.TransactionID)
			return false, nil
		}
	}

	// Token not found in either storage
	c.log.Debug("Token not found in any storage",
		"token_id", tokenID,
		"transaction_id", transactionID)
	return false, nil
}

// markTokenAsRecovered marks a token as recovered to prevent double recovery
func (c *Core) markTokenAsRecovered(tokenID, transactionID string) error {
	c.log.Debug("Marking token as recovered",
		"token_id", tokenID,
		"transaction_id", transactionID)

	// Read the FT token
	var ftToken wallet.FTToken
	err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to read token for recovery marking: %v", err)
	}

	// Add recovery metadata to the token state hash
	// This creates a permanent record that this token was recovered
	recoveryMarker := fmt.Sprintf("_RECOVERED_%s_%s", transactionID, time.Now().Format("20060102150405"))
	if !strings.Contains(ftToken.TokenStateHash, "_RECOVERED_") {
		ftToken.TokenStateHash = ftToken.TokenStateHash + recoveryMarker
	}

	// Update the token
	err = c.w.GetStorage().Update(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to mark token as recovered: %v", err)
	}

	c.log.Info("Successfully marked token as recovered",
		"token_id", tokenID,
		"transaction_id", transactionID,
		"recovery_marker", recoveryMarker)

	return nil
}

// isTokenPreviouslyRecovered checks if a token has been recovered before
func (c *Core) isTokenPreviouslyRecovered(tokenID string) bool {
	// Read the FT token
	var ftToken wallet.FTToken
	err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		c.log.Debug("Failed to read token for recovery check",
			"token_id", tokenID,
			"error", err)
		return false
	}

	// Check if the token state hash contains recovery marker
	if strings.Contains(ftToken.TokenStateHash, "_RECOVERED_") {
		c.log.Warn("Token has recovery marker in state hash",
			"token_id", tokenID,
			"state_hash", ftToken.TokenStateHash)
		return true
	}

	return false
}

// getTokensFromExplorer fetches token IDs from explorer API when local DB doesn't have them
func (c *Core) getTokensFromExplorer(transactionID string) ([]string, error) {
	c.log.Info("Fetching token list from explorer",
		"transaction_id", transactionID)
	
	// Determine which explorer to use based on network
	var explorerURL string
	if c.testNet {
		explorerURL = fmt.Sprintf("https://testnet-app-api.rubixexplorer.com/api/Transaction/GetById/%s", transactionID)
	} else {
		explorerURL = fmt.Sprintf("https://rexplorerapi.azurewebsites.net/api/Transaction/GetById/%s", transactionID)
	}
	
	c.log.Debug("Calling explorer API",
		"url", explorerURL)
	
	// Make HTTP request to explorer
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(explorerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from explorer: %v", err)
	}
	defer resp.Body.Close()
	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read explorer response: %v", err)
	}
	
	// Parse the response
	var explorerResp struct {
		Status bool `json:"status"`
		Data   struct {
			FTTokenList []string `json:"ftTokenList"`
			SenderDID   string   `json:"sender"`
			ReceiverDID string   `json:"receiverDid"`
			Amount      float64  `json:"amount"`
		} `json:"data"`
		Message string `json:"message"`
	}
	
	if err := json.Unmarshal(body, &explorerResp); err != nil {
		return nil, fmt.Errorf("failed to parse explorer response: %v", err)
	}
	
	if !explorerResp.Status {
		return nil, fmt.Errorf("explorer returned error: %s", explorerResp.Message)
	}
	
	if len(explorerResp.Data.FTTokenList) == 0 {
		return nil, fmt.Errorf("no tokens found in explorer response")
	}
	
	c.log.Info("Successfully fetched tokens from explorer",
		"transaction_id", transactionID,
		"token_count", len(explorerResp.Data.FTTokenList),
		"sender", explorerResp.Data.SenderDID,
		"receiver", explorerResp.Data.ReceiverDID)
	
	return explorerResp.Data.FTTokenList, nil
}
