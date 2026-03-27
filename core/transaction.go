package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (c *Core) InitiateTransaction(reqID string, req *models.TransactionRequest) {
	br := c.initiateTransaction(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateTransaction(reqID string, request *models.TransactionRequest) *model.BasicResponse {

	resp := &model.BasicResponse{
		Status: false,
	}
	ctx := c.Ctx
	initiatorDID := request.Initiator
	nextOwnerDID := request.Owner

	dc, err := c.SetupDID(reqID, initiatorDID)
	if err != nil {
		resp.Message = "InitiateTransaction:Failed to setup DID: " + err.Error()
		return resp
	}
	// This needs to be passed to the BuildTransactionInfoFromRequest to be given as an input to the function CollectRBTTokens
	// Can't call this inside BuildTransactionInfoFromRequest since it is a standalone function not a methiod of Core.
	networkMode, err := util.GetNetworkMode(c.testnet, c.mainnet, c.localnet)
	if err != nil {
		resp.Message = "InitiateTransaction:Failed to determine network mode: " + err.Error()
		return resp
	}
	// Build transaction info
	//Here the c.publishTxn must be verified because the input type is *model.PubSubTxnInfo which need to be updated
	transactionInfo, transactionValue, err := BuildTransactionInfoFromRequest(ctx, c.w, request, dc, networkMode, c.log, c.ps)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to build transaction info", "err", err)
		resp.Message = err.Error()
		return resp
	}

	// Fetch the list of dids from quorum_manager table
	//	We then loop over that list and queried from did table and pfetch the peerid
	quorumAddresses, err := c.GetAllQuorum()
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to get quorum address", "err", err)
		resp.Message = "InitiateTransaction: Failed to get quorum address: " + err.Error()
		return resp
	}
	if len(quorumAddresses) == 0 {
		resp.Message = "InitiateTransaction: No quorums available for transaction"
		return resp
	}

	// this will be a list of *ipfsport.Peer since we can have multiple quorums, we need to loop over them
	p, err := c.getPeer(quorumAddresses[0])
	if err != nil {
		resp.Message = err.Error()
		return resp
	}
	defer p.Close()

	// Pledge request
	pledgeTokenRequest := models.PledgeTokenRequest{
		ReferenceId:      reqID,
		TransactionValue: transactionValue,
	}

	var pledgeTokenResponse models.PledgeTokenResponse

	err = p.SendJSONRequest(
		"POST",
		APIReqPledgeToken,
		nil,
		&pledgeTokenRequest,
		&pledgeTokenResponse,
		true,
	)

	if err != nil {
		resp.Message = err.Error()
		return resp
	}

	// Attach quorum tokens
	pledgeTokenPtrs := make([]*models.TokenInfo, len(pledgeTokenResponse.PledgeTokens))
	for i := range pledgeTokenResponse.PledgeTokens {
		pledgeTokenPtrs[i] = pledgeTokenResponse.PledgeTokens[i]
	}
	pledegTokenInfo := &models.QuorumInfo{
		Did:    quorumAddresses[0],
		Tokens: pledgeTokenPtrs,
	}
	transactionInfo.Quorums = []*models.QuorumInfo{pledegTokenInfo}
	transactionId, err := util.GetTransactionID(transactionInfo)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to get transaction ID", "err", err)
		resp.Message = "InitiateTransaction: Failed to get transaction ID: " + err.Error()
		return resp
	}

	c.log.Info("Transaction ID created", "hash", transactionId)

	// Signature done by the initiator on the  transactionInfo
	initiatorSignature, err := util.SignTransaction(dc, transactionInfo)
	if err != nil {
		c.log.Error("InitiateTransaction:Failed to sign transaction", "err", err)
		resp.Message = "InitiateTransaction: Failed to sign transaction: " + err.Error()
		return resp
	}

	// Consensus request
	consensusRequest := models.ConsensusRequest{
		ReferenceId:        reqID,
		TransactionInfo:    transactionInfo,
		InitiatorSignature: initiatorSignature,
	}

	var consensusResponse models.ConsensusResponse

	err = p.SendJSONRequest(
		"POST",
		APIInitiateConsensus,
		nil,
		&consensusRequest,
		&consensusResponse,
		true,
	)

	if err != nil {
		resp.Message = err.Error()
		return resp
	}

	if !consensusResponse.Status {
		resp.Message = consensusResponse.Message
		return resp
	}
	// When multiple transaction situation comes into picture this quorum signature part will change.
	// We have kept this as an array to accomodate multiple quorums in future.
	// Right now we are only accomodating a single quorum.
	err = util.VerifySignature(dc, transactionInfo, consensusResponse.QuorumSignature)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to verify quorum signature", "err", err)
		resp.Message = "InitiateTransaction: Failed to verify quorum signature"
		return resp
	}
	quorumSignature := []models.QuorumSignature{{
		Did:       quorumAddresses[0],
		Signature: consensusResponse.QuorumSignature,
	}}
	signatureTobePublished := &models.Signature{
		InitiatorSignature: initiatorSignature,
		Quorums:            quorumSignature,
	}
	// Persist post-consensus state to PostgreSQL (soft-fail: log error, do not block transaction)
	if err := c.w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo: transactionInfo,
		Signature:       signatureTobePublished,
		DID:             initiatorDID,
		ExecutionRole:   wallet.ExecutionRoleInitiator,
	}); err != nil {
		c.log.Error("InitiateTransaction: failed to persist post-consensus state", "err", err)
	}
	//Publish transaction to the network
	util.PublishTransaction(c.ps, transactionInfo, signatureTobePublished)

	// Send tokens to receiver asynchronously in background
	go c.sendTokensToReceiver(nextOwnerDID, transactionId, transactionInfo, request)

	// Return immediately - receiver sync happens in background
	resp.Status = true
	resp.Message = "Transfer initiated successfully"
	return resp
}

// sendTokensToReceiver sends transaction tokens to the receiver asynchronously.
// This function is designed to run as a goroutine and handles all errors internally.
// It will not block or fail the main transaction flow.
func (c *Core) sendTokensToReceiver(
	receiverDID string,
	transactionID string,
	txInfo *models.TransactionInfo,
	request *models.TransactionRequest,
) {
	c.log.Debug("sendTokensToReceiver: Starting async receiver sync",
		"receiver", receiverDID,
		"transactionID", transactionID)

	// Get receiver peer connection
	receiverPeer, err := c.getPeer(receiverDID)
	if err != nil {
		c.log.Warn("sendTokensToReceiver: Receiver offline, will sync later",
			"receiver", receiverDID,
			"transactionID", transactionID,
			"err", err)
		return
	}
	defer receiverPeer.Close()

	// Prepare sync request
	// For smartcontracts we need not send the token information to the receiver, we just need to publish it.
	// For NFTs we need to check the particular boolean in the request for sending.
	// For RBTs we need to send the entire token information.
	var sendTokensRequest models.SendTokensRequest
	var sendTokensResponse model.BasicResponse
	sendTokensRequest.Tokens = txInfo.Tokens
	if request.HasNFT() {
		sendTokensRequest.NFTOwnershipTransfer = request.Tokens.TransferNFTOwnership
	}

	// Send tokens to receiver with 2-minute timeout
	// SendJSONRequest has built-in retry logic (3 attempts with exponential backoff)
	err = receiverPeer.SendJSONRequest(
		"POST",
		APISendTokens,
		nil,
		&sendTokensRequest,
		&sendTokensResponse,
		true,
		2*time.Minute, // Timeout for the entire operation
	)

	if err != nil {
		c.log.Warn("sendTokensToReceiver: Failed to sync tokens to receiver (will retry later)",
			"receiver", receiverDID,
			"transactionID", transactionID,
			"err", err)
		return
	}

	c.log.Info("sendTokensToReceiver: Receiver sync completed successfully",
		"receiver", receiverDID,
		"transactionID", transactionID)
}

// This has been added here since this is part of the transaction flow.
// Can be refactored in the future
func (c *Core) TransactionSetup() {
	c.l.AddRoute(APISendTokens, "POST", c.SendTokens)
}

// This function has been added here since the other corresponding sync functions has not been added yet.
// Once the other sync functions and all are added, we can move this along with that.
func (c *Core) syncTransactionTokens(
	peer *ipfsport.Peer,
	tokens *models.TransactionTokens,
	NFTOwnershipTransfer bool,
) error {

	tokenGroups := [][]*models.TokenInfo{
		tokens.RBT,
		tokens.FT,
	}

	// Add NFT only if flag is true
	if NFTOwnershipTransfer {
		tokenGroups = append(tokenGroups, tokens.NFT)
	}

	for _, group := range tokenGroups {
		for _, token := range group {
			if token == nil {
				continue
			}
			//Handling the response in the future.
			err, _ := c.syncTransactionChainFrom(peer, token.PreviousTransactionID, token.TokenID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Core) SendTokens(request *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuery(request, "did")
	crep := model.BasicResponse{Status: false}

	var sendTokensRequest models.SendTokensRequest
	err := c.l.ParseJSON(request, &sendTokensRequest)
	if err != nil {
		c.log.Error("SendTokens: Failed to parse json request", "err", err)
		crep.Message = "SendTokens: Failed to parse json request"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}
	peer, err := c.getPeer(did)
	if err != nil {
		c.log.Error("SendTokens: Failed to get peer for receiver", "err", err)
		crep.Message = "SendTokens: Failed to get peer for receiver"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}
	defer peer.Close()
	err = c.syncTransactionTokens(peer, sendTokensRequest.Tokens, sendTokensRequest.NFTOwnershipTransfer)
	if err != nil {
		c.log.Error("SendTokens: Failed to sync transaction tokens", "err", err)
		crep.Message = "SendTokens: Failed to sync transaction tokens"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}

	crep.Status = true
	crep.Message = "Tokens synced successfully"
	return c.l.RenderJSON(request, &crep, http.StatusOK)
}

func (c *Core) GetTransactionByID(txId string) (*models.TransactionInfo, error) {
	transactionDetail, err := c.w.GetTransactionByID(txId)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions details for tx: %v, err: %v", txId, err)
	}

	var txInfo *models.TransactionInfo
	if err := json.Unmarshal(transactionDetail.Info, txInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction info, err: %v", err)
	}

	return txInfo, nil
}

func (c *Core) GetAllTransactions() ([]models.Transactions, error) {
	return c.w.GetAllTransactions()
}
