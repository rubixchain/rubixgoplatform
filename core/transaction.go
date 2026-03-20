package core

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/constants"
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
	ctx := context.Background()
	initiatorDID := request.Initiator
	nextOwnerDID := request.Owner

	dc, err := c.SetupDID(reqID, initiatorDID)
	if err != nil {
		resp.Message = "InitiateTransaction:Failed to setup DID: " + err.Error()
		return resp
	}
	networkMode, err := util.GetNetworkMode(c.testnet, c.mainnet, c.localnet)
	if err != nil {
		resp.Message = "InitiateTransaction:Failed to determine network mode: " + err.Error()
		return resp
	}
	// Build transaction info
	transactionInfo, transactionValue, err := BuildTransactionInfoFromRequest(ctx, c.w, request, dc, networkMode, c.log, c.ps)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to build transaction info", "err", err)
		resp.Message = err.Error()
		return resp
	}

	// Validate each selected RBT token: must exist, belong to initiator, and be free
	if transactionInfo.Tokens != nil {
		for _, ti := range transactionInfo.Tokens.RBT {
			tok, err := c.w.ReadToken(ti.TokenID)
			if err != nil {
				resp.Message = "InitiateTransaction: token not found: " + ti.TokenID
				return resp
			}
			if tok.DID != initiatorDID {
				resp.Message = "InitiateTransaction: token " + ti.TokenID + " does not belong to initiator"
				return resp
			}
			if tok.TokenStatus != int16(constants.TokenStatus_Free) {
				resp.Message = "InitiateTransaction: token " + ti.TokenID + " is not free (status=" + fmt.Sprint(tok.TokenStatus) + ")"
				return resp
			}
		}
	}

	// Fetch quorum addresses
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
	txID, err := wallet.ComputeTransactionID(transactionInfo)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to compute transaction ID", "err", err)
		resp.Message = "InitiateTransaction: Failed to compute transaction ID: " + err.Error()
		return resp
	}

	c.log.Info("Transaction ID created", "txID", txID)

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
	// TODO: support multiple quorums (currently single quorum only)
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
	// Sending token information to the receiver
	// We need to send the genesis information, the previous transaction information and the latest transaction information.
	//sync api :
	// 1. Send information to receiver
	receiverPeer, err := c.getPeer(nextOwnerDID)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to get peer for receiver", "err", err)
	}
	defer receiverPeer.Close()

	var sendTokensRequest models.SendTokensRequest
	var sendTokensResponse model.BasicResponse
	sendTokensRequest.Tokens = transactionInfo.Tokens
	err = receiverPeer.SendJSONRequest(
		"POST",
		APISendTokens,
		nil,
		&sendTokensRequest,
		&sendTokensResponse,
		true,
	)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to send transaction info to receiver", "err", err)
	}
	resp.Status = true
	resp.Message = "Transfer initiated successfully"

	return resp
}

// This has been added here since this is part of the transaction flow.
// Can be refactored in the future
func (c *Core) TransactionSetup() {
	c.l.AddRoute(APISendTokens, "POST", c.SendTokens)
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
		c.log.Error("InitiateTransaction: Failed to get peer for receiver", "err", err)
	}
	defer peer.Close()
	// This will be the base logic, further changes can be added on top of this.
	// These will be added
	var syncTokenRequest models.SyncTransactionChainRequest
	var syncTokenResponse model.BasicResponse
	syncTokenRequest.Did = did
	// Here we need to manage this according to the new APISyncTransactionChain api
	err = peer.SendJSONRequest(
		"POST",
		APISyncTransactionChain,
		nil,
		&syncTokenRequest,
		&syncTokenResponse,
		true,
	)

	return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
}
