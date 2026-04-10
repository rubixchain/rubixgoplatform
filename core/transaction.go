package core

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/consensus"
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
	networkMode := c.networkMode
	// Build transaction info
	//Here the c.publishTxn must be verified because the input type is *model.PubSubTxnInfo which need to be updated
	// here the tokens which are being fetched as committed tokens in case of smartContract deployment: There we need to add the commitment block?

	// Ensure locked RBT tokens are released if the transaction fails at any step.
	// This defer must be registered BEFORE BuildTransactionInfoFromRequest because
	// LockTokensForSplit (called inside BuildTransactionInfoFromRequest) commits the
	// lock to DB immediately. If BuildTransactionInfoFromRequest itself fails (e.g.
	// insufficient balance), the lock must still be released.
	txSucceeded := false
	defer func() {
		if !txSucceeded {
			if releaseErr := c.w.ReleaseAllLockedRBTTokensForDID(ctx, initiatorDID, reqID); releaseErr != nil {
				c.log.Error("InitiateTransaction: failed to release locked tokens after failure", "err", releaseErr)
			} else {
				c.log.Info("InitiateTransaction: released locked tokens after failed transaction", "did", initiatorDID)
			}
		}
	}()
	maxRetries := 3
	var transactionInfo *models.TransactionInfo
	var transactionValue float64

	for attempt := 1; attempt <= maxRetries; attempt++ {

		transactionInfo, transactionValue, err = BuildTransactionInfoFromRequest(
			ctx,
			c.w,
			request,
			dc,
			networkMode,
			c.log,
			c.ps,
			reqID,
		)

		if err == nil {
			// success
			break
		}

		// ONLY retry TOCTOU
		if !isTOCTOUConflict(err) {
			c.log.Error("InitiateTransaction: Failed to build transaction info (non-retryable)", "err", err)
			resp.Message = err.Error()
			return resp
		}

		c.log.Warn("InitiateTransaction: TOCTOU conflict, retrying",
			"attempt", attempt,
			"maxRetries", maxRetries,
			"err", err,
		)

		if attempt == maxRetries {
			c.log.Error("InitiateTransaction: TOCTOU conflict — retries exhausted", "err", err)
			resp.Message = "Transaction failed due to high contention, please retry"
			return resp
		}

		/// In a high-contention scenario, we expect some transactions to hit TOCTOU conflicts due to the optimistic locking mechanism on tokens.
		time.Sleep(retryWithRandomBackoff(attempt))
		//time.Sleep(retryBackoff(attempt))
	}

	// Fetch the list of dids from quorum_manager table
	//  We then loop over that list and queried from did table and pfetch the peerid
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
		APIRequestPledgeToken,
		nil,
		&pledgeTokenRequest,
		&pledgeTokenResponse,
		true,
	)

	if err != nil {
		resp.Message = err.Error()
		return resp
	}

	pledgeTokens := pledgeTokenResponse.PledgeTokens

	// --- PRE-CONSENSUS GUARD ------------------------------------

	if len(pledgeTokens) == 0 {
		c.log.Debug("InitiateTransaction: no pledge tokens received",
			"referenceID", reqID,
			"requiredAmount", transactionValue,
		)

		resp.Message = "insufficient quorum liquidity"
		return resp
	}

	// compute total pledged value
	var totalPledged float64
	/* 	for _, t := range pledgeTokens {
		totalPledged += t.TokenValue
	} */

	// compute total pledged value (TRUSTLESS — derive from TokenID)
	for _, t := range pledgeTokens {
		val, err := util.GetTokenValueFromTokenID(t.TokenID)
		if err != nil {
			c.log.Error("InitiateTransaction: invalid token value",
				"tokenID", t.TokenID,
				"err", err,
			)
			resp.Message = "invalid quorum token"
			return resp
		}
		totalPledged += val
	}

	if totalPledged < transactionValue {
		c.log.Debug("InitiateTransaction: insufficient pledged value",
			"referenceID", reqID,
			"requiredAmount", transactionValue,
			"pledgedAmount", totalPledged,
		)

		resp.Message = "insufficient quorum liquidity"
		return resp
	}
	// ------------------------------------------------------------

	for _, token := range pledgeTokens {
		err = consensus.ValidateNewTokenContent(token.TokenID, true, c.testnet, c.mainnet, c.localnet, c.log)
		if err != nil {
			c.log.Error("InitiateTransaction: Failed to validate token content", "err", err)
			resp.Message = "InitiateTransaction: Failed to validate token content"
			return resp
		}
	}
	//Harden quorum assignment
	if len(pledgeTokens) > 0 {
		pledegTokenInfo := &models.QuorumInfo{
			Did:    quorumAddresses[0],
			Tokens: pledgeTokens,
		}
		transactionInfo.Quorums = []*models.QuorumInfo{pledegTokenInfo}
	} else {
		resp.Message = "insufficient quorum liquidity"
		return resp
	}
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
		if _, err := util.PublishTransaction(
			c.ps,
			transactionInfo,
			&models.Signature{
				InitiatorSignature: initiatorSignature,
			},
			false,
			err.Error(),
		); err != nil {
			c.log.Error("InitiateTransaction: Failed to publish transaction", "err", err)
		}

		resp.Message = err.Error()
		return resp
	}

	if !consensusResponse.Status {
		if _, err := util.PublishTransaction(
			c.ps,
			transactionInfo,
			&models.Signature{
				InitiatorSignature: initiatorSignature,
			},
			false,
			consensusResponse.Message,
		); err != nil {
			c.log.Error("InitiateTransaction: Failed to publish transaction", "err", err)
		}

		resp.Message = consensusResponse.Message
		return resp
	}

	c.log.Info("InitiateTransaction: Consensus response received", "quorumDID", quorumAddresses[0], "quorumSignatureLength", len(consensusResponse.QuorumSignature))

	quorumSignature := []models.QuorumSignature{{
		Did:       quorumAddresses[0],
		Signature: consensusResponse.QuorumSignature,
	}}
	signatureTobePublished := &models.Signature{
		InitiatorSignature: initiatorSignature,
		Quorums:            quorumSignature,
	}

	// When multiple transaction situation comes into picture this quorum signature part will change.
	// We have kept this as an array to accomodate multiple quorums in future.
	// Right now we are only accomodating a single quorum.

	// Set up quorum DIDCrypto for signature verification
	// We need the quorum's public key to verify its signature, not the initiator's
	// Note: selfDID parameter is not used by SetupForienDID, so we pass empty string
	c.log.Debug("InitiateTransaction: Setting up quorum DID for verification", "quorumDID", quorumAddresses[0])
	quorumDC, err := c.SetupForienDID(quorumAddresses[0], "")
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to setup quorum DID for verification", "quorumDID", quorumAddresses[0], "err", err)
		resp.Message = "InitiateTransaction: Failed to setup quorum DID: " + err.Error()
		return resp
	}

	c.log.Debug("InitiateTransaction: Verifying quorum signature", "quorumDID", quorumAddresses[0])
	err = util.VerifySignature(quorumDC, transactionInfo, consensusResponse.QuorumSignature)
	if err != nil {
		if _, err := util.PublishTransaction(c.ps, transactionInfo, signatureTobePublished, false, err.Error()); err != nil {
			c.log.Error("InitiateTransaction: Failed to publish transaction", "err", err)
		}

		c.log.Error("InitiateTransaction: Failed to verify quorum signature", "quorumDID", quorumAddresses[0], "err", err)
		resp.Message = "InitiateTransaction: Failed to verify quorum signature: " + err.Error()
		return resp
	}
	c.log.Info("InitiateTransaction: Quorum signature verified successfully", "quorumDID", quorumAddresses[0])

	// Persist post-consensus state to PostgreSQL.
	// On success the selected tokens are marked Transferred; remaining locked tokens
	// (candidates not chosen by CollectRBTTokens) are then released back to Free.
	// On failure we log and continue — the deferred ReleaseAllLockedRBTTokensForDID
	// will fire because txSucceeded is still false at this point.
	persistErr := c.w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo: transactionInfo,
		Signature:       signatureTobePublished,
		DID:             initiatorDID,
		ExecutionRole:   wallet.ExecutionRoleInitiator,
	})
	if persistErr != nil {
		c.log.Error("InitiateTransaction: failed to persist post-consensus state", "err", persistErr)
		// txSucceeded stays false → deferred cleanup will release all locked tokens
	} else {
		// Mark success to prevent the deferred full-release from firing,
		// then release only the non-selected locked tokens (candidates not used in the transfer).
		// The selected tokens are now status=Transferred so they won't be touched.
		txSucceeded = true
		if err := c.w.ReleaseAllLockedRBTTokensForDID(ctx, initiatorDID, reqID); err != nil {
			c.log.Error("InitiateTransaction: failed to release non-selected locked tokens", "err", err)
		}
		if err := c.w.ReleaseReferenceID(reqID); err != nil {
			c.log.Error("InitiateTransaction: failed to release reference ID", "err", err)
		}
	}
	/*
		if request.HasSmartContract() {
			c.publishSmartContractEvents(request, transactionId, initiatorDID, initiatorSignature, transactionInfo.Epoch)
		}

		if request.HasNFT() {
			c.publishNFTEvents(request, transactionId, initiatorDID, initiatorSignature, transactionInfo.Epoch)
		}
	*/
	//Publish transaction to the network
	if _, err := util.PublishTransaction(c.ps, transactionInfo, signatureTobePublished, true, ""); err != nil {
		c.log.Error("InitiateTransaction: Failed to publish transaction", "err", err)
	}

	// Send tokens to receiver asynchronously in background
	go c.sendTokensToReceiver(nextOwnerDID, transactionId, transactionInfo, signatureTobePublished, request)

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
	signature *models.Signature,
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
	sendTokensRequest.TransactionInfo = txInfo
	sendTokensRequest.Signature = signature
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
	c.l.AddRoute(APISyncTransactionChain, "POST", c.SyncTransactionChain)
}


func (c *Core) SendTokens(request *ensweb.Request) *ensweb.Result {
	crep := model.BasicResponse{Status: false}

	var sendTokensRequest models.SendTokensRequest
	err := c.l.ParseJSON(request, &sendTokensRequest)
	if err != nil {
		c.log.Error("SendTokens: Failed to parse json request", "err", err)
		crep.Message = "SendTokens: Failed to parse json request"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}

	if sendTokensRequest.TransactionInfo == nil || sendTokensRequest.Signature == nil {
		c.log.Error("SendTokens: Missing transaction info or signature in request")
		crep.Message = "SendTokens: Missing transaction info or signature"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}

	receiverDID := sendTokensRequest.TransactionInfo.Owner
	if receiverDID == "" {
		c.log.Error("SendTokens: Missing owner DID in transaction info")
		crep.Message = "SendTokens: Missing owner DID in transaction info"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}

	// --- Token chain sync: fill gaps from sender before persisting ---
	// Collect all token IDs and their PreviousTransactionIDs from the incoming transaction.
	var syncTokenIDs []string
	prevTxIDs := make(map[string]string)
	if txns := sendTokensRequest.TransactionInfo.Tokens; txns != nil {
		for _, t := range txns.RBT {
			if t != nil {
				syncTokenIDs = append(syncTokenIDs, t.TokenID)
				if t.PreviousTransactionID != "" {
					prevTxIDs[t.TokenID] = t.PreviousTransactionID
				}
			}
		}
		for _, t := range txns.FT {
			if t != nil {
				syncTokenIDs = append(syncTokenIDs, t.TokenID)
				if t.PreviousTransactionID != "" {
					prevTxIDs[t.TokenID] = t.PreviousTransactionID
				}
			}
		}
		// NFT: only sync if this is an ownership transfer
		if sendTokensRequest.NFTOwnershipTransfer {
			for _, t := range txns.NFT {
				if t != nil {
					syncTokenIDs = append(syncTokenIDs, t.TokenID)
					if t.PreviousTransactionID != "" {
						prevTxIDs[t.TokenID] = t.PreviousTransactionID
					}
				}
			}
		}
	}

	// Synchronous/blocking sync from sender (initiator) before we persist.
	// prevTxIDs allows applyTokenChainFromSync to skip tokens whose chain
	// tail already matches — avoiding redundant network + DB work.
	// Errors are logged but never break SendTokens — sync is best-effort.
	if len(syncTokenIDs) > 0 {
		initiatorDID := sendTokensRequest.TransactionInfo.Initiator
		if err := c.SyncTransactionChainsFromPeer(initiatorDID, syncTokenIDs, prevTxIDs, nil); err != nil {
			c.log.Warn("SendTokens: chain sync from sender failed (non-fatal)", "initiator", initiatorDID, "err", err)
		}
	}

	persistErr := c.w.PersistPostConsensus(c.Ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo:           sendTokensRequest.TransactionInfo,
		Signature:                 sendTokensRequest.Signature,
		DID:                       receiverDID,
		ExecutionRole:             wallet.ExecutionRoleReceiver,
		SkipSignatureVerification: true,
	})
	if persistErr != nil {
		c.log.Error("SendTokens: Failed to persist receiver token state", "err", persistErr)
		crep.Message = "SendTokens: Failed to persist receiver token state: " + persistErr.Error()
		return c.l.RenderJSON(request, &crep, http.StatusInternalServerError)
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

func isTOCTOUConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "TOCTOU conflict")
}

func retryBackoff(attempt int) time.Duration {
	// 50ms → 100ms → 150ms
	return time.Duration(attempt*50) * time.Millisecond
}

func retryWithRandomBackoff(attempt int) time.Duration {
	return time.Duration(attempt*50+rand.Intn(1000)) * time.Millisecond
}
