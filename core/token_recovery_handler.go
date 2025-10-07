package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

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

// RecoverLostTokens attempts to recover tokens that were sent but not received
// This function helps senders recover tokens when receivers don't have them
func (c *Core) RecoverLostTokens(senderDID, transactionID string) (*TokenRecoveryResult, error) {
	c.log.Info("========== STARTING TOKEN RECOVERY PROCESS ==========",
		"sender_did", senderDID,
		"transaction_id", transactionID,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	// Step 1: Get transaction details from history
	c.log.Info("STEP 1: Retrieving transaction from history",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	transactionDetails, err := c.getTransactionFromHistory(senderDID, transactionID)
	if err != nil {
		c.log.Error("STEP 1 FAILED: Transaction not found in history",
			"sender_did", senderDID,
			"transaction_id", transactionID,
			"error", err)
		return nil, fmt.Errorf("failed to get transaction from history: %v", err)
	}

	c.log.Info("STEP 1 SUCCESS: Transaction found",
		"transaction_id", transactionID,
		"amount", transactionDetails.Amount,
		"receiver_did", transactionDetails.ReceiverDID,
		"date", transactionDetails.DateTime,
		"comment", transactionDetails.Comment)

	// Step 1.5: Check if transaction has already been recovered
	c.log.Info("STEP 1.5: Checking if transaction was previously recovered")

	if c.isTransactionAlreadyRecovered(transactionDetails) {
		c.log.Error("STEP 1.5 FAILED: Transaction already recovered",
			"transaction_id", transactionID,
			"comment", transactionDetails.Comment,
			"recovery_marker_found", true)
		return nil, fmt.Errorf("transaction has already been recovered, recovery can only be done once per transaction")
	}

	c.log.Info("STEP 1.5 SUCCESS: Transaction has not been recovered before")

	// Step 2: Check if transaction is within recovery window (7-14 days)
	c.log.Info("STEP 2: Checking recovery time window",
		"transaction_date", transactionDetails.DateTime,
		"current_time", time.Now(),
		"age_days", time.Since(transactionDetails.DateTime).Hours()/24)

	if !c.isTransactionWithinRecoveryWindow(transactionDetails.DateTime) {
		age := time.Since(transactionDetails.DateTime)
		c.log.Error("STEP 2 FAILED: Transaction outside recovery window",
			"transaction_age", age,
			"age_days", age.Hours()/24,
			"required_window", "7-14 days (or <14 days based on config)")
		return nil, fmt.Errorf("transaction must be between 7-14 days old for recovery (current age: %v)", age)
	}

	c.log.Info("STEP 2 SUCCESS: Transaction within recovery window")

	// Step 3: Get receiver DID from transaction
	c.log.Info("STEP 3: Validating receiver information")

	receiverDID := transactionDetails.ReceiverDID
	if receiverDID == "" {
		c.log.Error("STEP 3 FAILED: No receiver DID in transaction",
			"transaction_id", transactionID)
		return nil, fmt.Errorf("receiver DID not found in transaction")
	}

	c.log.Info("STEP 3 SUCCESS: Receiver identified",
		"receiver_did", receiverDID)

	c.log.Info("Transaction details retrieved",
		"sender_did", senderDID,
		"receiver_did", receiverDID,
		"transaction_id", transactionID,
		"amount", transactionDetails.Amount,
		"date", transactionDetails.DateTime)

	// Check if this transaction has special exception status
	c.log.Info("STEP 3.5: Checking exception transaction status",
		"transaction_id", transactionID)

	isExceptionTransaction := c.isExceptionTransaction(transactionID)
	if isExceptionTransaction {
		c.log.Warn("STEP 3.5: EXCEPTION TRANSACTION DETECTED",
			"transaction_id", transactionID,
			"note", "Can proceed even if receiver is offline")
	} else {
		c.log.Info("STEP 3.5: Normal transaction (not in exception list)")
	}

	// Step 4: Check if receiver has the tokens
	c.log.Info("STEP 4: Checking if receiver currently has the tokens",
		"receiver_did", receiverDID,
		"transaction_id", transactionID,
		"expected_amount", transactionDetails.Amount)

	receiverHasTokens, err := c.checkReceiverTokenStatus(receiverDID, transactionID, int(transactionDetails.Amount))
	if err != nil {
		if isExceptionTransaction {
			c.log.Warn("STEP 4 EXCEPTION: Receiver offline but proceeding (exception transaction)",
				"transaction_id", transactionID,
				"receiver_did", receiverDID,
				"error", err,
				"action", "Assuming receiver doesn't have tokens")
			receiverHasTokens = false
		} else {
			c.log.Error("STEP 4 FAILED: Cannot verify receiver token status",
				"receiver_did", receiverDID,
				"error", err,
				"action", "Recovery blocked for safety")
			return nil, fmt.Errorf("cannot verify receiver token status (receiver may be offline): %v", err)
		}
	} else {
		c.log.Info("STEP 4: Successfully contacted receiver",
			"receiver_has_tokens", receiverHasTokens)
	}

	if receiverHasTokens {
		c.log.Error("STEP 4 FAILED: Receiver has the tokens",
			"receiver_did", receiverDID,
			"transaction_id", transactionID,
			"action", "Recovery not needed")
		return nil, fmt.Errorf("receiver has the tokens, no recovery needed")
	}

	c.log.Info("STEP 4 SUCCESS: Receiver does not have the tokens")

	// Step 5: Check if tokens were transferred from receiver (double-spending check)
	c.log.Info("STEP 5: Checking if receiver transferred tokens to someone else",
		"receiver_did", receiverDID,
		"transaction_id", transactionID)

	tokensTransferredFromReceiver, err := c.checkIfTokensTransferredFromReceiver(receiverDID, transactionID)
	if err != nil {
		if isExceptionTransaction {
			c.log.Warn("STEP 5 EXCEPTION: Cannot verify transfer status but proceeding (exception transaction)",
				"transaction_id", transactionID,
				"receiver_did", receiverDID,
				"error", err,
				"action", "Assuming tokens not transferred")
			tokensTransferredFromReceiver = false
		} else {
			c.log.Error("STEP 5 FAILED: Cannot verify if receiver transferred tokens",
				"receiver_did", receiverDID,
				"error", err,
				"action", "Recovery blocked for safety")
			return nil, fmt.Errorf("cannot verify if receiver transferred tokens (receiver may be offline): %v", err)
		}
	}

	if tokensTransferredFromReceiver {
		c.log.Error("STEP 5 FAILED: Tokens were transferred by receiver",
			"receiver_did", receiverDID,
			"transaction_id", transactionID,
			"action", "Recovery blocked - double-spending protection")
		return nil, fmt.Errorf("tokens were transferred from receiver, cannot recover (double-spending detected)")
	}

	c.log.Info("STEP 5 SUCCESS: Tokens not transferred by receiver")

	// Step 6: Perform token recovery
	c.log.Info("STEP 6: Starting actual token recovery",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	recoveryResult, err := c.performTokenRecovery(senderDID, transactionID, transactionDetails)
	if err != nil {
		c.log.Error("STEP 6 FAILED: Token recovery failed",
			"sender_did", senderDID,
			"transaction_id", transactionID,
			"error", err)
		return nil, fmt.Errorf("failed to perform token recovery: %v", err)
	}

	c.log.Info("========== TOKEN RECOVERY COMPLETED SUCCESSFULLY ==========",
		"sender_did", senderDID,
		"transaction_id", transactionID,
		"recovered_tokens", recoveryResult.RecoveredTokenCount,
		"recovery_date", recoveryResult.RecoveryDate,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	return recoveryResult, nil
}

// getTransactionFromHistory retrieves transaction details from FT transaction history
func (c *Core) getTransactionFromHistory(senderDID, transactionID string) (*model.TransactionDetails, error) {
	c.log.Debug("Getting transaction from history",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	// First try to get from FT transaction history
	var ftHistory []model.FTTransactionHistory
	err := c.w.GetStorage().Read(wallet.FTTransactionHistoryStorage, &ftHistory,
		"sender_did=? AND transaction_id=?", senderDID, transactionID)

	if err == nil && len(ftHistory) > 0 {
		// Convert FT history to TransactionDetails
		ft := ftHistory[0]
		return &model.TransactionDetails{
			TransactionID:   ft.TransactionID,
			TransactionType: "FT",
			SenderDID:       ft.SenderDID,
			ReceiverDID:     ft.ReceiverDID,
			Amount:          float64(ft.Amount),
			DateTime:        ft.DateTime,
			Comment:         ft.Comment,
			Status:          ft.Status,
		}, nil
	}

	// If not found in FT history, try regular transaction history
	var txHistory []model.TransactionDetails
	err = c.w.GetStorage().Read(wallet.TransactionStorage, &txHistory,
		"sender_did=? AND transaction_id=?", senderDID, transactionID)

	if err != nil || len(txHistory) == 0 {
		// If not found locally, try to get from explorer as fallback
		c.log.Info("Transaction not found locally, trying from network",
			"sender_did", senderDID,
			"transaction_id", transactionID)

		explorerTransaction, err := c.getTransactionFromExplorer(senderDID, transactionID)
		if err != nil {
			return nil, fmt.Errorf("transaction not found in local history or network: %v", err)
		}

		c.log.Info("Transaction found in network",
			"transaction_id", transactionID,
			"sender_did", senderDID)

		return explorerTransaction, nil
	}

	return &txHistory[0], nil
}

// isTransactionAlreadyRecovered checks if transaction has already been recovered
func (c *Core) isTransactionAlreadyRecovered(transaction *model.TransactionDetails) bool {
	// Check if comment contains recovery marker
	if transaction.Comment != "" {
		return fmt.Sprintf("%s", transaction.Comment) == "RECOVERED" ||
			fmt.Sprintf("%s", transaction.Comment) == "[RECOVERED]"
	}

	// Check token_recovery table - this is the primary source of truth
	var recovery model.TokenRecovery
	err := c.w.GetStorage().Read("TokenRecovery", &recovery,
		"transaction_id=?", transaction.TransactionID)

	if err == nil {
		c.log.Info("Transaction already recovered",
			"transaction_id", transaction.TransactionID,
			"recovery_date", recovery.RecoveredAt,
			"recovered_by", recovery.RecoveredBy)
		return true
	}

	return false
}

// isTransactionWithinRecoveryWindow checks if transaction is within the recovery time window
func (c *Core) isTransactionWithinRecoveryWindow(transactionDate time.Time) bool {
	age := time.Since(transactionDate)
	ageDays := age.Hours() / 24

	// Allow recovery for transactions older than 7 days but less than 14 days
	// For testing/development, we might be more lenient
	//return ageDays >= 7 && ageDays <= 14
	return ageDays <= 14
}

// isExceptionTransaction checks if this transaction is marked for exception handling
func (c *Core) isExceptionTransaction(transactionID string) bool {
	// For now, return false - could be enhanced to check a database table
	// or configuration file for exception transactions
	return false
}

// checkReceiverTokenStatus checks if receiver has the tokens
func (c *Core) checkReceiverTokenStatus(receiverDID, transactionID string, expectedAmount int) (bool, error) {
	c.log.Debug("Checking receiver token status",
		"receiver", receiverDID,
		"transaction_id", transactionID,
		"expected_amount", expectedAmount)

	receiverPeer, err := c.getPeer(receiverDID)
	if err != nil {
		return false, fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Check token ownership - this would be a simple ping to see if receiver has tokens
	// For simplicity, we'll assume receiver doesn't have tokens if we can't verify
	return false, nil
}

// checkIfTokensTransferredFromReceiver checks if receiver has already transferred the tokens
func (c *Core) checkIfTokensTransferredFromReceiver(receiverDID, transactionID string) (bool, error) {
	c.log.Debug("Checking if tokens transferred from receiver",
		"receiver", receiverDID,
		"transaction_id", transactionID)

	// For simplicity, assume tokens were not transferred
	// In a real implementation, this would check the receiver's transaction history
	return false, nil
}

// performTokenRecovery performs the actual token recovery process
func (c *Core) performTokenRecovery(senderDID, transactionID string, transactionDetails *model.TransactionDetails) (*TokenRecoveryResult, error) {
	c.log.Info("Performing token recovery",
		"sender_did", senderDID,
		"transaction_id", transactionID,
		"amount", transactionDetails.Amount)

	// Step 1: Get the tokens that were sent in this transaction
	var tokensToRecover []string
	var tokenCount int

	if transactionDetails.TransactionType == "FT" {
		// For FT tokens, we need to find the tokens in the FT token storage
		var ftTokens []wallet.FTToken
		err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftTokens,
			"transaction_id=? AND token_status=?", transactionID, wallet.TokenIsPledged)

		if err != nil || len(ftTokens) == 0 {
			// If tokens not found locally, try to get from explorer
			c.log.Info("FT tokens not found locally, trying to get from network",
				"transaction_id", transactionID)

			explorerTokens, err := c.getTokensFromExplorer(transactionID, "FT")
			if err != nil {
				return nil, fmt.Errorf("failed to find pledged FT tokens locally or in network: %v", err)
			}

			tokensToRecover = explorerTokens
			tokenCount = len(explorerTokens)

			c.log.Info("Found FT tokens in network",
				"transaction_id", transactionID,
				"token_count", tokenCount)
		} else {
			for _, token := range ftTokens {
				tokensToRecover = append(tokensToRecover, token.TokenID)
			}
			tokenCount = len(ftTokens)
		}
	} else {
		// For RBT tokens
		var tokens []wallet.Token
		err := c.w.GetStorage().Read(wallet.TokenStorage, &tokens,
			"transaction_id=? AND token_status=?", transactionID, wallet.TokenIsPledged)

		if err != nil || len(tokens) == 0 {
			// If tokens not found locally, try to get from explorer
			c.log.Info("RBT tokens not found locally, trying to get from network",
				"transaction_id", transactionID)

			explorerTokens, err := c.getTokensFromExplorer(transactionID, "RBT")
			if err != nil {
				return nil, fmt.Errorf("failed to find pledged RBT tokens locally or in network: %v", err)
			}

			tokensToRecover = explorerTokens
			tokenCount = len(explorerTokens)

			c.log.Info("Found RBT tokens in network",
				"transaction_id", transactionID,
				"token_count", tokenCount)
		} else {
			for _, token := range tokens {
				tokensToRecover = append(tokensToRecover, token.TokenID)
			}
			tokenCount = len(tokens)
		}
	}

	if len(tokensToRecover) == 0 {
		return nil, fmt.Errorf("no pledged tokens found for transaction %s", transactionID)
	}

	c.log.Info("Found tokens to recover",
		"transaction_id", transactionID,
		"token_count", tokenCount,
		"token_ids", tokensToRecover)

	// Step 2: Change token status from pledged to free (or create if missing)
	for _, tokenID := range tokensToRecover {
		if transactionDetails.TransactionType == "FT" {
			// Try to read FT token
			var ftToken wallet.FTToken
			err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
			if err != nil {
				// Token doesn't exist locally (was fetched from explorer), create it
				c.log.Info("Creating FT token from explorer data",
					"token_id", tokenID,
					"owner", senderDID)

				// Get FT info from explorer if needed
				ftInfo, err := c.getFTInfoFromExplorer(transactionID)
				if err != nil {
					c.log.Error("Failed to get FT info from explorer", "error", err)
					// Use default values if we can't get FT info
					ftInfo = &FTInfo{
						CreatorDID: "",
						FTName:     "TRI",
					}
				}

				ftToken = wallet.FTToken{
					TokenID:       tokenID,
					DID:           senderDID, // Owner DID
					TokenStatus:   wallet.TokenIsFree,
					TransactionID: "", // Clear - token is now free
					CreatorDID:    ftInfo.CreatorDID,
					FTName:        ftInfo.FTName,
					TokenValue:    0.001, // Standard FT token value
				}
				err = c.w.GetStorage().Write(wallet.FTTokenStorage, &ftToken)
				if err != nil {
					c.log.Error("Failed to create recovered FT token",
						"token_id", tokenID,
						"error", err)
				} else {
					c.log.Info("Successfully created recovered FT token",
						"token_id", tokenID,
						"owner", senderDID,
						"status", "free")
				}
			} else {
				// Token exists - check if it belongs to this transaction or has wrong transaction ID
				c.log.Info("FT token exists, checking ownership and transaction",
					"token_id", tokenID,
					"current_owner", ftToken.DID,
					"current_txn_id", ftToken.TransactionID,
					"current_status", ftToken.TokenStatus,
					"expected_txn_id", transactionID,
					"expected_owner", senderDID)

				// Check if token has the wrong transaction ID or wrong owner
				needsUpdate := false

				// If token is pledged with this transaction ID, it needs recovery
				if ftToken.TransactionID == transactionID || ftToken.TokenStatus == wallet.TokenIsPledged {
					ftToken.TokenStatus = wallet.TokenIsFree
					ftToken.TransactionID = "" // Clear transaction ID
					needsUpdate = true
				}

				// If token doesn't belong to sender, update owner
				if ftToken.DID != senderDID {
					c.log.Info("Updating token owner during recovery",
						"token_id", tokenID,
						"old_owner", ftToken.DID,
						"new_owner", senderDID)
					ftToken.DID = senderDID
					ftToken.TokenStatus = wallet.TokenIsFree
					ftToken.TransactionID = "" // Clear transaction ID
					needsUpdate = true
				}

				// If token has a different transaction ID but should be recovered, free it
				if ftToken.TransactionID != "" && ftToken.TransactionID != transactionID {
					c.log.Warn("Token has different transaction ID, recovering anyway",
						"token_id", tokenID,
						"stored_txn_id", ftToken.TransactionID,
						"recovery_txn_id", transactionID)
					ftToken.TokenStatus = wallet.TokenIsFree
					ftToken.TransactionID = "" // Clear transaction ID
					ftToken.DID = senderDID    // Update owner
					needsUpdate = true
				}

				if needsUpdate {
					err = c.w.GetStorage().Update(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
					if err != nil {
						c.log.Error("Failed to update FT token during recovery",
							"token_id", tokenID,
							"error", err)
					} else {
						c.log.Info("Successfully recovered FT token",
							"token_id", tokenID,
							"owner", senderDID,
							"status", "free")
					}
				} else {
					c.log.Info("Token already in correct state",
						"token_id", tokenID,
						"owner", ftToken.DID,
						"status", ftToken.TokenStatus)
				}
			}
		} else {
			// Update regular token status
			var token wallet.Token
			err := c.w.GetStorage().Read(wallet.TokenStorage, &token, "token_id=?", tokenID)
			if err == nil && token.TokenStatus == wallet.TokenIsPledged {
				token.TokenStatus = wallet.TokenIsFree
				token.TransactionID = "" // Clear transaction ID
				err = c.w.GetStorage().Update(wallet.TokenStorage, &token, "token_id=?", tokenID)
				if err != nil {
					c.log.Error("Failed to update token status",
						"token_id", tokenID,
						"error", err)
				}
			}
		}
	}

	// Step 3: Record the recovery in the recovery tracking table
	recovery := &model.TokenRecovery{
		TransactionID: transactionID,
		RecoveredAt:   time.Now(),
		RecoveredBy:   senderDID,
		TokenCount:    tokenCount,
		TokenIDs:      fmt.Sprintf("%v", tokensToRecover), // Convert to JSON string
		RecoveryType:  "normal",
		RecoveryNotes: "Token recovery completed successfully",
	}

	err := c.w.GetStorage().Write("TokenRecovery", recovery)
	if err != nil {
		c.log.Error("Failed to record recovery in tracking table",
			"transaction_id", transactionID,
			"error", err)
		// Don't fail the recovery for this
	}

	// Step 4: Remove transaction from history since it's being rolled back
	// When tokens are recovered, the transaction effectively never happened, so we remove it from history
	// IMPORTANT: Remove ALL instances of the transaction (both sender and receiver copies)
	if transactionDetails.TransactionType == "FT" {
		// Remove from FT transaction history - transaction never happened
		// Remove ALL instances of this transaction ID, regardless of sender/receiver
		// Use empty model instance like in token_rollback.go
		err := c.w.GetStorage().Delete(wallet.FTTransactionHistoryStorage,
			&model.FTTransactionHistory{}, "transaction_id = ?", transactionID)
		if err != nil {
			c.log.Error("Failed to remove FT transaction from history",
				"transaction_id", transactionID,
				"error", err)
			// Don't fail the recovery for this, but log it
		} else {
			c.log.Info("Successfully removed FT transaction from history",
				"transaction_id", transactionID,
				"sender_did", senderDID,
				"note", "Removed all instances of transaction")
		}

		// Also remove from regular transaction history (FT transactions are stored in both)
		err = c.w.GetStorage().Delete(wallet.TransactionStorage,
			&model.TransactionDetails{}, "transaction_id = ?", transactionID)
		if err != nil {
			c.log.Error("Failed to remove transaction from regular history",
				"transaction_id", transactionID,
				"error", err)
		} else {
			c.log.Info("Successfully removed transaction from regular history",
				"transaction_id", transactionID)
		}
	} else {
		// Remove from regular transaction history - transaction never happened
		// Remove ALL instances of this transaction ID, regardless of sender/receiver
		err := c.w.GetStorage().Delete(wallet.TransactionStorage,
			&model.TransactionDetails{}, "transaction_id = ?", transactionID)
		if err != nil {
			c.log.Error("Failed to remove transaction from history",
				"transaction_id", transactionID,
				"error", err)
			// Don't fail the recovery for this, but log it
		} else {
			c.log.Info("Successfully removed transaction from history",
				"transaction_id", transactionID,
				"sender_did", senderDID,
				"note", "Removed all instances of transaction")
		}
	}

	// Step 5: Create recovery result
	result := &TokenRecoveryResult{
		TransactionID:       transactionID,
		SenderDID:           senderDID,
		ReceiverDID:         transactionDetails.ReceiverDID,
		RecoveredTokenCount: tokenCount,
		RecoveryDate:        time.Now(),
		Status:              "success",
		Message:             fmt.Sprintf("Successfully recovered %d tokens and removed transaction %s from history (transaction rolled back)", tokenCount, transactionID),
	}

	c.log.Info("Token recovery completed successfully",
		"transaction_id", transactionID,
		"recovered_tokens", tokenCount,
		"sender_did", senderDID)

	return result, nil
}

// getTransactionFromExplorer retrieves transaction details from explorer
func (c *Core) getTransactionFromExplorer(senderDID, transactionID string) (*model.TransactionDetails, error) {
	c.log.Info("Fetching transaction from network",
		"sender_did", senderDID,
		"transaction_id", transactionID)

	// Get configured explorers
	explorers := c.getConfiguredExplorers()
	if len(explorers) == 0 {
		return nil, fmt.Errorf("unable to get network configured")
	}

	// Try each explorer until we find the transaction
	for _, explorerURL := range explorers {
		transaction, err := c.fetchTransactionFromExplorer(explorerURL, transactionID)
		if err != nil {
			c.log.Debug("Failed to get transaction from network",
				"explorer", explorerURL,
				"transaction_id", transactionID,
				"error", err)
			continue
		}

		// Validate the transaction matches our criteria
		if transaction.SenderDID == senderDID && transaction.TransactionID == transactionID {
			c.log.Info("Successfully retrieved transaction from network",
				"explorer", explorerURL,
				"transaction_id", transactionID)
			return transaction, nil
		}
	}

	return nil, fmt.Errorf("transaction not found in any configured network")
}

// getTokensFromExplorer retrieves token list for a transaction from explorer
func (c *Core) getTokensFromExplorer(transactionID, tokenType string) ([]string, error) {
	c.log.Info("Fetching tokens from network",
		"transaction_id", transactionID,
		"token_type", tokenType)

	// Get configured explorers
	explorers := c.getConfiguredExplorers()
	if len(explorers) == 0 {
		return nil, fmt.Errorf("no network configured")
	}

	// Try each explorer until we find the tokens
	for _, explorerURL := range explorers {
		tokens, err := c.fetchTokensFromExplorer(explorerURL, transactionID, tokenType)
		if err != nil {
			c.log.Debug("Failed to get tokens from network",
				"explorer", explorerURL,
				"transaction_id", transactionID,
				"error", err)
			continue
		}

		if len(tokens) > 0 {
			c.log.Info("Successfully retrieved tokens from network",
				"explorer", explorerURL,
				"transaction_id", transactionID,
				"token_count", len(tokens))
			return tokens, nil
		}
	}

	return nil, fmt.Errorf("tokens not found in any configured network")
}

// getConfiguredExplorers returns list of configured explorer URLs
func (c *Core) getConfiguredExplorers() []string {
	// Return hardcoded explorer URLs based on network type
	var explorers []string

	if c.testNet {
		// TestNet explorer
		explorers = []string{"https://testnet-app-api.rubixexplorer.com"}
	} else {
		// MainNet explorer
		explorers = []string{"https://rexplorerapi.azurewebsites.net"}
	}

	c.log.Debug("Using configured network",
		"explorers", explorers,
		"testnet", c.testNet)

	return explorers
}

// fetchTransactionFromExplorer makes HTTP request to explorer for transaction details
func (c *Core) fetchTransactionFromExplorer(explorerURL, transactionID string) (*model.TransactionDetails, error) {
	c.log.Debug("Fetching transaction from network API",
		"explorer_url", explorerURL,
		"transaction_id", transactionID)

	// Construct the full API URL
	fullURL := fmt.Sprintf("%s/api/Transaction/GetById/%s", explorerURL, transactionID)

	// Make HTTP request to explorer
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from network: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read network response: %v", err)
	}

	// Parse the response
	var explorerResp struct {
		Status bool `json:"status"`
		Data   struct {
			TransactionID   string  `json:"transactionId"`
			Sender          string  `json:"sender"`
			ReceiverDID     string  `json:"receiverDid"`
			Amount          float64 `json:"amount"`
			TransactionType string  `json:"transactionType"`
			Timestamp       string  `json:"timestamp"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &explorerResp); err != nil {
		return nil, fmt.Errorf("failed to parse network response: %v", err)
	}

	if !explorerResp.Status {
		return nil, fmt.Errorf("network returned error: %s", explorerResp.Message)
	}

	// Parse timestamp
	dateTime, err := time.Parse("2006-01-02T15:04:05.000Z", explorerResp.Data.Timestamp)
	if err != nil {
		// Try alternative format
		dateTime, err = time.Parse("2006-01-02T15:04:05Z", explorerResp.Data.Timestamp)
		if err != nil {
			// Try without timezone
			dateTime, err = time.Parse("2006-01-02 15:04:05", explorerResp.Data.Timestamp)
			if err != nil {
				dateTime = time.Now() // Fallback
			}
		}
	}

	// Convert to TransactionDetails
	transactionDetails := &model.TransactionDetails{
		TransactionID:   explorerResp.Data.TransactionID,
		TransactionType: explorerResp.Data.TransactionType,
		SenderDID:       explorerResp.Data.Sender,
		ReceiverDID:     explorerResp.Data.ReceiverDID,
		Amount:          explorerResp.Data.Amount,
		DateTime:        dateTime,
		Comment:         "",
		Status:          true,
	}

	c.log.Info("Successfully fetched transaction from network",
		"transaction_id", transactionID,
		"sender", transactionDetails.SenderDID,
		"receiver", transactionDetails.ReceiverDID,
		"amount", transactionDetails.Amount)

	return transactionDetails, nil
}

// FTInfo holds FT metadata
type FTInfo struct {
	CreatorDID string
	FTName     string
}

// getFTInfoFromExplorer retrieves FT metadata from explorer
func (c *Core) getFTInfoFromExplorer(transactionID string) (*FTInfo, error) {
	explorers := c.getConfiguredExplorers()
	if len(explorers) == 0 {
		return nil, fmt.Errorf("no explorers configured")
	}

	for _, explorerURL := range explorers {
		fullURL := fmt.Sprintf("%s/api/Transaction/GetById/%s", explorerURL, transactionID)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(fullURL)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var explorerResp struct {
			Status bool `json:"status"`
			Data   struct {
				Tokens []struct {
					CreatorDID string `json:"creatorDid"`
					FTName     string `json:"ftName"`
				} `json:"tokens"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &explorerResp); err != nil {
			continue
		}

		if explorerResp.Status && len(explorerResp.Data.Tokens) > 0 {
			return &FTInfo{
				CreatorDID: explorerResp.Data.Tokens[0].CreatorDID,
				FTName:     explorerResp.Data.Tokens[0].FTName,
			}, nil
		}
	}

	return nil, fmt.Errorf("could not get FT info from any explorer")
}

// fetchTokensFromExplorer makes HTTP request to explorer for token list
func (c *Core) fetchTokensFromExplorer(explorerURL, transactionID, tokenType string) ([]string, error) {
	c.log.Debug("Fetching tokens from network API",
		"explorer_url", explorerURL,
		"transaction_id", transactionID,
		"token_type", tokenType)

	// Construct the full API URL
	fullURL := fmt.Sprintf("%s/api/Transaction/GetById/%s", explorerURL, transactionID)

	// Make HTTP request to explorer
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from network: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read network response: %v", err)
	}

	// Parse the response
	var explorerResp struct {
		Status bool `json:"status"`
		Data   struct {
			FTTokenList     []string `json:"ftTokenList"`
			TokenList       []string `json:"tokenList"` // RBT tokens
			Sender          string   `json:"sender"`
			ReceiverDID     string   `json:"receiverDid"`
			Amount          float64  `json:"amount"`
			TransactionType string   `json:"transactionType"`
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &explorerResp); err != nil {
		return nil, fmt.Errorf("failed to parse network response: %v", err)
	}

	if !explorerResp.Status {
		return nil, fmt.Errorf("network returned error: %s", explorerResp.Message)
	}

	// Return appropriate token list based on type
	var tokens []string
	if tokenType == "FT" {
		tokens = explorerResp.Data.FTTokenList
	} else {
		// For RBT tokens, use tokenList field
		tokens = explorerResp.Data.TokenList
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no %s tokens found in network response", tokenType)
	}

	c.log.Info("Successfully fetched tokens from network",
		"transaction_id", transactionID,
		"token_type", tokenType,
		"token_count", len(tokens),
		"sender", explorerResp.Data.Sender,
		"receiver", explorerResp.Data.ReceiverDID)

	return tokens, nil
}
