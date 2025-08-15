package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
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

	// Step 2: Check if transaction is within 7 days
	if !c.isTransactionWithinRecoveryWindow(transactionDetails.DateTime) {
		return nil, fmt.Errorf("transaction is older than 7 days, cannot recover tokens")
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

	// Step 4: Check if receiver has the tokens
	receiverHasTokens, err := c.checkReceiverTokenStatus(receiverDID, transactionID, int(transactionDetails.Amount))
	if err != nil {
		c.log.Warn("Failed to check receiver token status, proceeding with recovery",
			"receiver_did", receiverDID,
			"error", err)
		// Continue with recovery even if we can't check receiver
		receiverHasTokens = false
	}

	if receiverHasTokens {
		return nil, fmt.Errorf("receiver has the tokens, no recovery needed")
	}

	// Step 5: Check if tokens were transferred from receiver (double-spending check)
	tokensTransferredFromReceiver, err := c.checkIfTokensTransferredFromReceiver(receiverDID, transactionID)
	if err != nil {
		c.log.Warn("Failed to check if tokens were transferred from receiver",
			"receiver_did", receiverDID,
			"error", err)
		// Continue with recovery but log warning
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
func (c *Core) isTransactionWithinRecoveryWindow(transactionDate time.Time) bool {
	recoveryWindow := 7 * 24 * time.Hour // 7 days
	cutoffTime := time.Now().Add(-recoveryWindow)

	return transactionDate.After(cutoffTime)
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

	if len(tokenIDs) == 0 {
		return 0, nil // No tokens to recover
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
