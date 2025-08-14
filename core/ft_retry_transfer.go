package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
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

// ExplorerTransactionResponse represents the response from explorer API
type ExplorerTransactionResponse struct {
	Status bool `json:"status"`
	Data   struct {
		TransactionType  string   `json:"transactionType"`
		TransactionID    string   `json:"transactionId"`
		Creator          string   `json:"creator"`
		Sender           string   `json:"sender"`
		ReceiverDID      string   `json:"receiverDid"`
		Amount           float64  `json:"amount"`
		FTName           string   `json:"ftName"`
		FTTransferCount  int      `json:"ftTransferCount"`
		FTTokenList      []string `json:"ftTokenList"`
		Timestamp        string   `json:"timestamp"`
	} `json:"data"`
	Message string `json:"message"`
}

// RetryFTTransfer retries sending FT tokens to receiver using transaction details from explorer
func (c *Core) RetryFTTransfer(req *RetryFTTransferRequest) (*RetryFTTransferResponse, error) {
	c.log.Info("Starting FT transfer retry", 
		"transaction_id", req.TransactionID,
		"sender_did", req.SenderDID,
		"receiver_did", req.ReceiverDID)
	
	response := &RetryFTTransferResponse{
		Status: false,
	}
	
	// Step 1: Fetch transaction details from explorer API
	var explorerBaseURL string
	var apiPath string
	if c.testNet {
		explorerBaseURL = "testnet-app-api.rubixexplorer.com"
	} else {
		explorerBaseURL = "rexplorerapi.azurewebsites.net"
	}
	apiPath = fmt.Sprintf("/api/Transaction/GetById/%s", req.TransactionID)
	
	c.log.Info("Fetching transaction from explorer", "base_url", explorerBaseURL, "path", apiPath)
	
	// Create a temporary explorer client for this specific request
	// Using Production: "true" ensures TLS certificate verification is skipped
	cl, err := ensweb.NewClient(&config.Config{
		ServerAddress: explorerBaseURL,
		ServerPort:    "443",
		Production:    "true",
	}, c.log)
	if err != nil {
		c.log.Error("Failed to create explorer client", "err", err)
		response.Message = fmt.Sprintf("Failed to create explorer client: %v", err)
		return response, nil
	}
	
	// Create the request
	httpReq, err := cl.JSONRequestForExplorer("GET", apiPath, nil, fmt.Sprintf("https://%s", explorerBaseURL), "")
	if err != nil {
		c.log.Error("Failed to create explorer request", "err", err)
		response.Message = fmt.Sprintf("Failed to create explorer request: %v", err)
		return response, nil
	}
	
	// Execute the request
	resp, err := cl.Do(httpReq)
	if err != nil {
		c.log.Error("Failed to fetch transaction from explorer", "err", err)
		response.Message = fmt.Sprintf("Failed to fetch transaction from explorer: %v", err)
		return response, nil
	}
	defer resp.Body.Close()
	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.log.Error("Failed to read explorer response", "err", err)
		response.Message = fmt.Sprintf("Failed to read explorer response: %v", err)
		return response, nil
	}
	
	var explorerResp ExplorerTransactionResponse
	if err := json.Unmarshal(body, &explorerResp); err != nil {
		c.log.Error("Failed to parse explorer response", "err", err)
		response.Message = fmt.Sprintf("Failed to parse explorer response: %v", err)
		return response, nil
	}
	
	if !explorerResp.Status {
		c.log.Error("Explorer API returned error", "message", explorerResp.Message)
		response.Message = fmt.Sprintf("Explorer API error: %s", explorerResp.Message)
		return response, nil
	}
	
	// Step 2: Validate transaction type and participants
	if explorerResp.Data.TransactionType != "FT" {
		response.Message = fmt.Sprintf("Transaction %s is not an FT transaction (type: %s)", req.TransactionID, explorerResp.Data.TransactionType)
		return response, nil
	}
	
	if explorerResp.Data.Sender != req.SenderDID {
		response.Message = fmt.Sprintf("Sender DID mismatch. Expected: %s, Got: %s", req.SenderDID, explorerResp.Data.Sender)
		return response, nil
	}
	
	if explorerResp.Data.ReceiverDID != req.ReceiverDID {
		response.Message = fmt.Sprintf("Receiver DID mismatch. Expected: %s, Got: %s", req.ReceiverDID, explorerResp.Data.ReceiverDID)
		return response, nil
	}
	
	// Step 3: Extract FT details from explorer response
	ftTokenList := explorerResp.Data.FTTokenList
	ftName := explorerResp.Data.FTName
	creatorDID := explorerResp.Data.Creator
	ftCount := explorerResp.Data.FTTransferCount
	tokenValue := explorerResp.Data.Amount / float64(ftCount) // Calculate individual token value
	
	c.log.Info("Found FT transaction in explorer", 
		"ft_name", ftName,
		"token_count", ftCount,
		"creator_did", creatorDID,
		"token_value", tokenValue)
	
	// Step 4: Build TokenInfo array from the token list
	tokenInfo := make([]contract.TokenInfo, 0)
	tt := c.TokenType(FTString)
	
	for _, tokenID := range ftTokenList {
		// Get the latest block for this token from local storage
		blk := c.w.GetLatestTokenBlock(tokenID, tt)
		if blk == nil {
			c.log.Error("Failed to get latest block for token", "token_id", tokenID)
			continue
		}
		
		bid, err := blk.GetBlockID(tokenID)
		if err != nil {
			c.log.Error("Failed to get block ID", "token_id", tokenID, "err", err)
			continue
		}
		
		ti := contract.TokenInfo{
			Token:      tokenID,
			TokenType:  tt,
			TokenValue: tokenValue,
			OwnerDID:   req.ReceiverDID, // Tokens should be transferred to receiver
			BlockID:    bid,
		}
		tokenInfo = append(tokenInfo, ti)
	}
	
	if len(tokenInfo) == 0 {
		response.Message = "Failed to prepare token information for retry"
		return response, nil
	}
	
	// Step 5: Get the token chain block
	firstToken := tokenInfo[0].Token
	tokenChainBlock := c.w.GetLatestTokenBlock(firstToken, c.TokenType(FTString))
	if tokenChainBlock == nil {
		response.Message = "Failed to get token chain block"
		return response, nil
	}
	
	// Step 6: Use current time as transaction epoch for retry
	transactionEpoch := time.Now().Unix()
	
	// Step 7: Prepare SendFTRequest
	senderPeerID := c.peerID
	sr := SendFTRequest{
		Address:          senderPeerID + "." + req.SenderDID,
		TokenInfo:        tokenInfo,
		TokenChainBlock:  tokenChainBlock.GetBlock(),
		QuorumList:       []string{}, // Empty for retry as tokens are already pledged
		TransactionEpoch: int(transactionEpoch),
		FTInfo: model.FTInfo{
			FTName:     ftName,
			FTCount:    len(tokenInfo),
			CreatorDID: creatorDID,
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
		"ft_name", ftName)
	
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
		req.TransactionID, len(tokenInfo), ftName)
	response.TransactionID = req.TransactionID
	response.TokenCount = len(tokenInfo)
	
	c.log.Info("Successfully retried FT transfer", 
		"transaction_id", req.TransactionID,
		"tokens_sent", len(tokenInfo),
		"ft_name", ftName)
	
	// Step 13: Update explorer balances (optional, in background)
	go func() {
		c.UpdateUserInfo([]string{req.SenderDID})
		c.UpdateUserInfo([]string{req.ReceiverDID})
	}()
	
	return response, nil
}