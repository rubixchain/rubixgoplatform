package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// APIConfirmTokenTransfer handles token transfer confirmation from receiver
// This was MISSING and causing the entire confirmation flow to fail!
func (c *Core) APIConfirmTokenTransfer(req *ConfirmTokenRequest) error {
	c.log.Info("Received token transfer confirmation",
		"transaction_id", req.TransactionID,
		"token_count", len(req.Tokens),
		"token_type", req.TokenType)

	// Step 1: Verify that we actually have these tokens
	hasTokens := true
	for _, tokenID := range req.Tokens {
		var exists bool
		if req.TokenType == c.TokenType(FTString) {
			var ftToken wallet.FTToken
			err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
			exists = (err == nil && ftToken.TransactionID == req.TransactionID)
		} else {
			var token wallet.Token
			err := c.w.GetStorage().Read(wallet.TokenStorage, &token, "token_id=?", tokenID)
			exists = (err == nil && token.TransactionID == req.TransactionID)
		}
		
		if !exists {
			c.log.Error("Token not found for confirmation",
				"token_id", tokenID,
				"transaction_id", req.TransactionID)
			hasTokens = false
			break
		}
	}

	if !hasTokens {
		return fmt.Errorf("receiver does not have all tokens for transaction %s", req.TransactionID)
	}

	// Step 2: Update token status to confirmed
	for _, tokenID := range req.Tokens {
		if req.TokenType == c.TokenType(FTString) {
			// Update FT token status from pending (17) to free (0)
			var ftToken wallet.FTToken
			err := c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
			if err == nil && ftToken.TokenStatus == wallet.TokenIsPending {
				ftToken.TokenStatus = wallet.TokenIsFree
				ftToken.TransactionID = req.TransactionID // Preserve transaction ID
				err = c.w.GetStorage().Update(wallet.FTTokenStorage, &ftToken, "token_id=?", tokenID)
				if err != nil {
					c.log.Error("Failed to update FT token status",
						"token_id", tokenID,
						"error", err)
				}
			}
		} else {
			// Update regular token status
			var token wallet.Token
			err := c.w.GetStorage().Read(wallet.TokenStorage, &token, "token_id=?", tokenID)
			if err == nil && token.TokenStatus == wallet.TokenIsPending {
				token.TokenStatus = wallet.TokenIsFree
				token.TransactionID = req.TransactionID
				err = c.w.GetStorage().Update(wallet.TokenStorage, &token, "token_id=?", tokenID)
				if err != nil {
					c.log.Error("Failed to update token status",
						"token_id", tokenID,
						"error", err)
				}
			}
		}
	}

	// Step 3: Signal confirmation to sender (if waiting)
	err := c.SignalConfirmation(req.TransactionID)
	if err != nil {
		c.log.Warn("Failed to signal confirmation (sender might not be waiting)",
			"transaction_id", req.TransactionID,
			"error", err)
	}

	c.log.Info("Token transfer confirmation completed successfully",
		"transaction_id", req.TransactionID,
		"token_count", len(req.Tokens))

	return nil
}

// HandleSyncFailure handles token sync failures specifically for "invalid block number" errors
func (c *Core) HandleSyncFailure(tokenID string, transactionID string, err error) error {
	c.log.Error("Handling sync failure for token",
		"token_id", tokenID,
		"transaction_id", transactionID,
		"error", err)

	// Check if this is a sequence missing error
	if err.Error() == "invalid block number, sequence missing" {
		c.log.Info("Attempting to recover from sequence missing error",
			"token_id", tokenID)

		// Try to fetch missing blocks
		if err := c.recoverMissingBlocks(tokenID, transactionID); err != nil {
			c.log.Error("Failed to recover missing blocks",
				"token_id", tokenID,
				"error", err)
			return err
		}
	}

	return nil
}

// recoverMissingBlocks attempts to recover missing token chain blocks
func (c *Core) recoverMissingBlocks(tokenID string, transactionID string) error {
	c.log.Info("Recovering missing blocks for token",
		"token_id", tokenID,
		"transaction_id", transactionID)

	// Step 1: Get current block height
	currentHeight, err := c.w.GetTokenChainHeight(tokenID)
	if err != nil {
		return fmt.Errorf("failed to get current block height: %v", err)
	}

	// Step 2: Request missing blocks from peers
	missingBlocks, err := c.requestMissingBlocks(tokenID, currentHeight)
	if err != nil {
		return fmt.Errorf("failed to request missing blocks: %v", err)
	}

	// Step 3: Apply missing blocks in sequence
	for _, block := range missingBlocks {
		if err := c.w.AddTokenChainBlock(block); err != nil {
			return fmt.Errorf("failed to add missing block: %v", err)
		}
	}

	c.log.Info("Successfully recovered missing blocks",
		"token_id", tokenID,
		"blocks_recovered", len(missingBlocks))

	return nil
}

// requestMissingBlocks requests missing blocks from peers
func (c *Core) requestMissingBlocks(tokenID string, fromHeight int) ([]interface{}, error) {
	c.log.Debug("Requesting missing blocks from peers",
		"token_id", tokenID,
		"from_height", fromHeight)

	// Get quorum list for recovery - use the proper method via QuorumManager
	ql := c.qm.GetQuorum(QuorumTypeTwo, "", c.peerID)
	if ql == nil || len(ql) == 0 {
		return nil, fmt.Errorf("failed to get quorum list: no quorums available")
	}

	var missingBlocks []interface{}
	
	// Try each quorum member until we get the blocks
	for _, quorumDID := range ql {
		peer, err := c.getPeer(quorumDID)
		if err != nil {
			c.log.Debug("Failed to connect to quorum peer",
				"peer", quorumDID,
				"error", err)
			continue
		}
		defer peer.Close()

		// Request blocks starting from the missing height
		req := map[string]interface{}{
			"token_id":    tokenID,
			"from_height": fromHeight,
		}

		var resp struct {
			Status bool          `json:"status"`
			Blocks []interface{} `json:"blocks"`
		}

		err = peer.SendJSONRequest("POST", "/api/get-token-blocks", nil, &req, &resp, true)
		if err == nil && resp.Status && len(resp.Blocks) > 0 {
			missingBlocks = resp.Blocks
			c.log.Info("Successfully retrieved missing blocks",
				"token_id", tokenID,
				"block_count", len(missingBlocks),
				"from_peer", quorumDID)
			break
		}
	}

	if len(missingBlocks) == 0 {
		return nil, fmt.Errorf("no blocks retrieved from any quorum peer")
	}

	return missingBlocks, nil
}

// RecoverFailedTransactions recovers all failed transactions from logs
// This specifically handles the scenario seen in your logs
func (c *Core) RecoverFailedTransactions(senderDID string) ([]*TokenRecoveryResult, error) {
	c.log.Info("Starting recovery of failed transactions",
		"sender_did", senderDID)

	// Get all transactions that show as successful but might have failed at receiver
	var ftHistory []model.FTTransactionHistory
	err := c.w.GetStorage().Read("ft_transaction_history", &ftHistory, 
		"sender_did=? AND status=? AND date_time > ?", 
		senderDID, "success", time.Now().Add(-7*24*time.Hour))
	
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction history: %v", err)
	}

	var recoveryResults []*TokenRecoveryResult
	
	for _, tx := range ftHistory {
		c.log.Info("Checking transaction for recovery",
			"transaction_id", tx.TransactionID,
			"receiver_did", tx.ReceiverDID)

		// Check if receiver actually has the tokens
		receiverHasTokens, err := c.checkReceiverTokenOwnership(tx.ReceiverDID, tx.TransactionID, int(tx.Amount))
		if err != nil || !receiverHasTokens {
			c.log.Info("Receiver doesn't have tokens, initiating recovery",
				"transaction_id", tx.TransactionID)

			// Recover the tokens
			result, err := c.RecoverLostTokens(senderDID, tx.TransactionID)
			if err != nil {
				c.log.Error("Failed to recover tokens",
					"transaction_id", tx.TransactionID,
					"error", err)
				continue
			}

			recoveryResults = append(recoveryResults, result)
		}
	}

	c.log.Info("Failed transaction recovery completed",
		"sender_did", senderDID,
		"transactions_recovered", len(recoveryResults))

	return recoveryResults, nil
}

// EnhancedProactiveCheck performs enhanced proactive checking with sync recovery
func (c *Core) EnhancedProactiveCheck(receiverAddress, transactionID string, tokens []contract.TokenInfo, tokenType int) {
	c.log.Info("Starting enhanced proactive check with sync recovery",
		"receiver", receiverAddress,
		"transaction_id", transactionID)

	// First, check if receiver is having sync issues
	syncStatus, err := c.checkReceiverSyncStatus(receiverAddress, transactionID)
	if err != nil {
		c.log.Error("Failed to check receiver sync status",
			"error", err)
	}

	if syncStatus == "sync_failed" {
		c.log.Info("Receiver has sync issues, attempting to help with sync",
			"receiver", receiverAddress)

		// Try to help receiver sync by providing token chain data
		if err := c.assistReceiverSync(receiverAddress, tokens); err != nil {
			c.log.Error("Failed to assist receiver sync",
				"error", err)
		}
	}

	// Continue with normal proactive check
	c.proactiveStatusCheck(receiverAddress, transactionID, tokens, tokenType)
}

// checkReceiverTokenOwnership verifies if receiver has tokens for a transaction
func (c *Core) checkReceiverTokenOwnership(receiverDID, transactionID string, tokenCount int) (bool, error) {
	c.log.Debug("Checking receiver token status",
		"receiver", receiverDID,
		"transaction_id", transactionID,
		"expected_tokens", tokenCount)

	receiverPeer, err := c.getPeer(receiverDID)
	if err != nil {
		return false, fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Check token ownership
	req := map[string]interface{}{
		"transaction_id": transactionID,
		"token_count":    tokenCount,
	}

	var resp struct {
		Status    bool `json:"status"`
		HasTokens bool `json:"has_tokens"`
		Message   string `json:"message"`
	}

	err = receiverPeer.SendJSONRequest("POST", "/api/verify-token-ownership", nil, &req, &resp, true)
	if err != nil {
		c.log.Error("Failed to verify receiver token ownership",
			"receiver", receiverDID,
			"error", err)
		return false, err
	}

	return resp.HasTokens, nil
}

// checkReceiverSyncStatus checks if receiver is having sync issues
func (c *Core) checkReceiverSyncStatus(receiverAddress, transactionID string) (string, error) {
	receiverPeer, err := c.getPeer(receiverAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get receiver peer: %v", err)
	}
	defer receiverPeer.Close()

	// Check sync status
	var resp struct {
		Status     bool   `json:"status"`
		SyncStatus string `json:"sync_status"`
	}

	err = receiverPeer.SendJSONRequest("GET", "/api/check-sync-status", 
		map[string]string{"transaction_id": transactionID}, nil, &resp, true)
	
	if err != nil {
		return "unknown", err
	}

	return resp.SyncStatus, nil
}

// assistReceiverSync helps receiver sync token chains
func (c *Core) assistReceiverSync(receiverAddress string, tokens []contract.TokenInfo) error {
	c.log.Info("Assisting receiver with token sync",
		"receiver", receiverAddress,
		"token_count", len(tokens))

	for _, token := range tokens {
		// Get complete token chain
		tokenChain, err := c.w.GetCompleteTokenChain(token.Token)
		if err != nil {
			c.log.Error("Failed to get token chain",
				"token", token.Token,
				"error", err)
			continue
		}

		// Send token chain to receiver
		receiverPeer, err := c.getPeer(receiverAddress)
		if err != nil {
			continue
		}

		syncReq := map[string]interface{}{
			"token_id":    token.Token,
			"token_chain": tokenChain,
			"assist_sync": true,
		}

		var resp model.BasicResponse
		err = receiverPeer.SendJSONRequest("POST", "/api/assist-token-sync", nil, &syncReq, &resp, true)
		receiverPeer.Close()

		if err != nil {
			c.log.Error("Failed to assist token sync",
				"token", token.Token,
				"error", err)
		}
	}

	return nil
}