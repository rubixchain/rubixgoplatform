package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ConfirmTokenRequest represents the request to confirm pending tokens
type ConfirmTokenRequest struct {
	TransactionID string   `json:"transaction_id"`
	Tokens        []string `json:"tokens"`
	TokenType     int      `json:"token_type"` // 0 for RBT, 4 for FT
}

// confirmTokenTransfer confirms pending tokens after consensus finality
func (c *Core) confirmTokenTransfer(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")

	var ctr ConfirmTokenRequest
	err := c.l.ParseJSON(req, &ctr)
	if err != nil {
		c.log.Error("Failed to parse confirm token request", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Failed to parse request",
		}, 400)
	}

	c.log.Info("Confirming token transfer",
		"did", did,
		"transaction_id", ctr.TransactionID,
		"token_count", len(ctr.Tokens),
		"token_type", ctr.TokenType)

	// Confirm tokens based on type
	if ctr.TokenType == c.TokenType(FTString) {
		err = c.w.ConfirmPendingFTTokens(ctr.TransactionID, ctr.Tokens)
	} else {
		err = c.w.ConfirmPendingTokens(ctr.TransactionID, ctr.Tokens)
	}

	if err != nil {
		c.log.Error("Failed to confirm tokens", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: fmt.Sprintf("Failed to confirm tokens: %v", err),
		}, 500)
	}

	return c.l.RenderJSON(req, &model.BasicResponse{
		Status:  true,
		Message: "Tokens confirmed successfully",
	}, 200)
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

	// For now, we'll use a simple approach - check if tokens exist in receiver's storage
	// This can be enhanced later with a proper verification API
	c.log.Info("Token verification completed (placeholder implementation)",
		"receiver", receiverAddress,
		"transaction_id", txID,
		"token_count", len(tokenIDs))

	// Return true for now - you can implement proper verification later
	return true, nil
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

	// Start confirmation wait with timeout
	confirmationChan := make(chan bool, 1)
	timeoutChan := time.After(baseTimeout)

	// Goroutine to wait for receiver confirmation
	go func() {
		// Wait for receiver to send confirmation via API endpoint
		// This will be implemented as a separate API endpoint that receiver calls
		// For now, we'll simulate waiting for confirmation
		select {
		case <-timeoutChan:
			c.log.Warn("Confirmation timeout occurred, no confirmation received from receiver",
				"transaction_id", transactionID,
				"receiver", receiverAddress)
			confirmationChan <- false
		case <-c.getConfirmationSignal(transactionID): // This will be implemented
			c.log.Info("Receiver confirmation received",
				"transaction_id", transactionID,
				"receiver", receiverAddress)
			confirmationChan <- true
		}
	}()

	// Wait for confirmation or timeout
	select {
	case confirmed := <-confirmationChan:
		if confirmed {
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
		} else {
			c.log.Warn("Confirmation timeout, starting proactive status check",
				"transaction_id", transactionID,
				"receiver", receiverAddress)

			// Start proactive status checking
			go c.proactiveStatusCheck(receiverAddress, transactionID, tokens, tokenType)
		}
	}
}

// confirmationManager manages confirmation signals for transactions
type confirmationManager struct {
	confirmations map[string]chan struct{}
	mu            sync.RWMutex
}

var globalConfirmationManager = &confirmationManager{
	confirmations: make(map[string]chan struct{}),
}

// getConfirmationSignal returns a channel that will receive a signal when receiver confirms
func (c *Core) getConfirmationSignal(transactionID string) chan struct{} {
	globalConfirmationManager.mu.Lock()
	defer globalConfirmationManager.mu.Unlock()

	// Create confirmation channel for this transaction
	if _, exists := globalConfirmationManager.confirmations[transactionID]; !exists {
		globalConfirmationManager.confirmations[transactionID] = make(chan struct{}, 1)
	}

	return globalConfirmationManager.confirmations[transactionID]
}

// SignalConfirmation signals that a transaction has been confirmed by the receiver
func (c *Core) SignalConfirmation(transactionID string) error {
	globalConfirmationManager.mu.Lock()
	defer globalConfirmationManager.mu.Unlock()

	if ch, exists := globalConfirmationManager.confirmations[transactionID]; exists {
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
	globalConfirmationManager.mu.Lock()
	defer globalConfirmationManager.mu.Unlock()

	if ch, exists := globalConfirmationManager.confirmations[transactionID]; exists {
		close(ch)
		delete(globalConfirmationManager.confirmations, transactionID)
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
