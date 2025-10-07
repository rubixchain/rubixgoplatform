package core

import (
	"fmt"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/contract"
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