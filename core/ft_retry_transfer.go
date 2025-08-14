package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
)

// RetryFTTransferRequest represents the request to retry an FT transfer
type RetryFTTransferRequest struct {
	TransactionID string `json:"transaction_id" binding:"required"`
	SenderDID     string `json:"sender_did" binding:"required"`
	ReceiverDID   string `json:"receiver_did" binding:"required"`
}

// RetryFTTransferResponse represents the response for retry FT transfer
type RetryFTTransferResponse struct {
	Status        bool   `json:"status"`
	Message       string `json:"message"`
	TransactionID string `json:"transaction_id,omitempty"`
	TokenCount    int    `json:"token_count,omitempty"`
}

// RetryFTTransfer retries sending FT tokens to receiver using transaction details
func (c *Core) RetryFTTransfer(req *RetryFTTransferRequest) (*RetryFTTransferResponse, error) {
	c.log.Info("Starting FT transfer retry", 
		"transaction_id", req.TransactionID,
		"sender_did", req.SenderDID,
		"receiver_did", req.ReceiverDID)
	
	response := &RetryFTTransferResponse{
		Status: false,
	}
	
	// Step 1: Verify this is a valid FT transaction from FTTransactionHistoryStorage
	var ftTxnHistory model.FTTransactionHistory
	err := c.s.Read(wallet.FTTransactionHistoryStorage, &ftTxnHistory, 
		"transaction_id=? AND sender_did=? AND receiver_did=?", 
		req.TransactionID, req.SenderDID, req.ReceiverDID)
	if err != nil {
		c.log.Error("FT transaction not found in FTTransactionHistoryStorage", 
			"transaction_id", req.TransactionID, "err", err)
		response.Message = fmt.Sprintf("FT transaction not found or sender/receiver mismatch: %v", err)
		return response, nil
	}
	
	c.log.Info("Found FT transaction history", 
		"ft_name", ftTxnHistory.FTName,
		"token_count", ftTxnHistory.TokenCount,
		"creator_did", ftTxnHistory.CreatorDID)
	
	// Step 2: Get all FT tokens for this transaction from FTTokenStorage
	var ftTokens []wallet.FTToken
	err = c.s.Read(wallet.FTTokenStorage, &ftTokens, "transaction_id=?", req.TransactionID)
	if err != nil || len(ftTokens) == 0 {
		c.log.Error("No FT tokens found in FTTokenStorage for transaction", 
			"transaction_id", req.TransactionID, "err", err)
		response.Message = fmt.Sprintf("No FT tokens found for transaction %s", req.TransactionID)
		return response, nil
	}
	
	// Verify token count matches FT transaction history
	if len(ftTokens) != ftTxnHistory.TokenCount {
		c.log.Warn("Token count mismatch", 
			"expected", ftTxnHistory.TokenCount, 
			"found", len(ftTokens))
	}
	
	c.log.Info("Found FT tokens for transaction", 
		"count", len(ftTokens), 
		"transaction_id", req.TransactionID)
	
	// Step 3: Build TokenInfo array from the FT tokens
	tokenInfo := make([]contract.TokenInfo, 0)
	
	for _, ft := range ftTokens {
		// Verify FT metadata matches
		if ft.FTName != ftTxnHistory.FTName {
			c.log.Warn("FT name mismatch for token", 
				"token_id", ft.TokenID,
				"expected", ftTxnHistory.FTName,
				"found", ft.FTName)
		}
		
		// Get the latest block for this token
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(ft.TokenID, tt)
		if blk == nil {
			c.log.Error("Failed to get latest block for token", "token_id", ft.TokenID)
			continue
		}
		
		bid, err := blk.GetBlockID(ft.TokenID)
		if err != nil {
			c.log.Error("Failed to get block ID", "token_id", ft.TokenID, "err", err)
			continue
		}
		
		ti := contract.TokenInfo{
			Token:      ft.TokenID,
			TokenType:  tt,
			TokenValue: ft.TokenValue,
			OwnerDID:   req.ReceiverDID, // Tokens should be transferred to receiver
			BlockID:    bid,
		}
		tokenInfo = append(tokenInfo, ti)
	}
	
	if len(tokenInfo) == 0 {
		response.Message = "Failed to prepare token information for retry"
		return response, nil
	}
	
	// Step 4: Get the token chain block
	firstToken := tokenInfo[0].Token
	tokenChainBlock := c.w.GetLatestTokenBlock(firstToken, c.TokenType(FTString))
	if tokenChainBlock == nil {
		response.Message = "Failed to get token chain block"
		return response, nil
	}
	
	// Step 5: Get transaction epoch from FT transaction history
	transactionEpoch := ftTxnHistory.Epoch
	if transactionEpoch == 0 {
		// Fallback: try to get from general transaction storage if needed
		var txHistory model.TransactionDetails
		err = c.s.Read(wallet.TransactionStorage, &txHistory, "transaction_id=?", req.TransactionID)
		if err == nil {
			transactionEpoch = txHistory.Epoch
		} else {
			// Use current time as epoch if not found
			transactionEpoch = time.Now().Unix()
		}
	}
	
	// Step 6: Check if we have FTTransactionToken metadata
	var ftTxnTokens []model.FTTransactionToken
	err = c.s.Read(wallet.FTTransactionTokenStorage, &ftTxnTokens, 
		"transaction_id=? AND direction=?", req.TransactionID, "sent")
	if err == nil && len(ftTxnTokens) > 0 {
		c.log.Info("Found FT transaction token metadata", 
			"count", len(ftTxnTokens),
			"ft_name", ftTxnTokens[0].FTName,
			"creator_did", ftTxnTokens[0].CreatorDID)
	}
	
	// Step 7: Prepare SendFTRequest
	senderPeerID := c.peerID
	sr := SendFTRequest{
		Address:          senderPeerID + "." + req.SenderDID,
		TokenInfo:        tokenInfo,
		TokenChainBlock:  tokenChainBlock.GetBlock(),
		QuorumList:       []string{}, // Empty for retry as tokens are already pledged
		TransactionEpoch: int(transactionEpoch),
		FTInfo: model.FTInfo{
			FTName:     ftTxnHistory.FTName,
			FTCount:    len(tokenInfo),
			CreatorDID: ftTxnHistory.CreatorDID,
		},
	}
	
	// Step 8: Get receiver peer connection
	receiverPeerID, receiverDID, ok := util.ParseAddress(req.ReceiverDID)
	if !ok || receiverPeerID == "" {
		// If receiver DID doesn't have peer ID, try to get it from peer info
		peerInfo, err := c.GetPeerDIDInfo(req.ReceiverDID)
		if err != nil || peerInfo.PeerID == "" {
			response.Message = fmt.Sprintf("Failed to get receiver peer information: %v", err)
			return response, nil
		}
		receiverPeerID = peerInfo.PeerID
		receiverDID = req.ReceiverDID
	}
	
	// Connect to receiver peer
	rp, err := c.getPeer(receiverPeerID + "." + receiverDID)
	if err != nil {
		response.Message = fmt.Sprintf("Failed to connect to receiver: %v", err)
		c.log.Error("Failed to connect to receiver", "err", err)
		return response, nil
	}
	defer rp.Close()
	
	// Step 9: Populate quorum info if available (empty for retry scenarios)
	sr.QuorumInfo = []QuorumDIDPeerMap{}
	
	// Step 10: Send the FT transfer request to receiver
	c.log.Info("Sending FT tokens to receiver", 
		"receiver", receiverPeerID+"."+receiverDID,
		"token_count", len(tokenInfo),
		"ft_name", ftTxnHistory.FTName)
	
	var br model.BasicResponse
	err = rp.SendJSONRequest("POST", APISendFTToken, nil, &sr, &br, true)
	if err != nil {
		response.Message = fmt.Sprintf("Failed to send tokens to receiver: %v", err)
		c.log.Error("Failed to send tokens to receiver", "err", err)
		return response, nil
	}
	
	// Step 11: Check response from receiver
	if !br.Status {
		if strings.Contains(br.Message, "failed to sync tokenchain") {
			response.Message = fmt.Sprintf("Receiver failed to sync token chain: %s", br.Message)
		} else {
			response.Message = fmt.Sprintf("Receiver rejected tokens: %s", br.Message)
		}
		c.log.Error("Receiver rejected FT tokens", "message", br.Message)
		return response, nil
	}
	
	// Step 12: Optionally update FT transaction history status if needed
	// This could be used to mark the retry as successful
	
	// Success
	response.Status = true
	response.Message = fmt.Sprintf("Successfully retried FT transfer for transaction %s. Sent %d %s tokens to receiver.",
		req.TransactionID, len(tokenInfo), ftTxnHistory.FTName)
	response.TransactionID = req.TransactionID
	response.TokenCount = len(tokenInfo)
	
	c.log.Info("Successfully retried FT transfer", 
		"transaction_id", req.TransactionID,
		"tokens_sent", len(tokenInfo),
		"ft_name", ftTxnHistory.FTName)
	
	// Step 13: Update explorer balances (optional, in background)
	go func() {
		c.UpdateUserInfo([]string{req.SenderDID})
		c.UpdateUserInfo([]string{req.ReceiverDID})
	}()
	
	return response, nil
}