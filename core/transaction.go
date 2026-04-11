package core

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	rubixsync "github.com/rubixchain/rubixgoplatform/core/sync"
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
	c.log.Info("InitiateTransaction: Starting transaction",
		"reqID", reqID,
		"initiator", request.Initiator,
		"owner", request.Owner,
		"hasNFT", request.HasNFT(),
		"hasSC", request.HasSmartContract(),
	)

	resp := &model.BasicResponse{
		Status: false,
	}
	ctx := c.Ctx
	initiatorDID := request.Initiator
	nextOwnerDID := request.Owner

	dc, err := c.SetupDID(reqID, initiatorDID)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to setup DID", "err", err, "did", initiatorDID)
		resp.Message = "InitiateTransaction:Failed to setup DID: " + err.Error()
		return resp
	}
	c.log.Debug("InitiateTransaction: DID setup complete", "did", initiatorDID)

	networkMode := c.networkMode
	c.log.Debug("InitiateTransaction: Network mode", "mode", networkMode)
	// Build transaction info
	//Here the c.publishTxn must be verified because the input type is *model.PubSubTxnInfo which need to be updated
	// here the tokens which are being fetched as committed tokens in case of smartContract deployment: There we need to add the commitment block?

	// Ensure locked RBT tokens are released if the transaction fails at any step.
	// This defer must be registered BEFORE BuildTransactionInfoFromRequest because
	// LockTokensForSplit (called inside BuildTransactionInfoFromRequest) commits the
	// lock to DB immediately. If BuildTransactionInfoFromRequest itself fails (e.g.
	// insufficient balance), the lock must still be released.
	c.log.Debug("InitiateTransaction: Registering deferred token cleanup")
	txSucceeded := false
	defer func() {
		if !txSucceeded {
			c.log.Warn("InitiateTransaction: Transaction failed, releasing locked tokens", "did", initiatorDID)
			if releaseErr := c.w.ReleaseAllLockedRBTTokensForDID(ctx, initiatorDID, reqID); releaseErr != nil {
				c.log.Error("InitiateTransaction: failed to release locked tokens after failure", "err", releaseErr, "did", initiatorDID)
			} else {
				c.log.Info("InitiateTransaction: released locked tokens after failed transaction", "did", initiatorDID)
			}
		} else {
			c.log.Debug("InitiateTransaction: Transaction succeeded, tokens will be transferred", "did", initiatorDID)
		}
	}()

	c.log.Info("InitiateTransaction: Building transaction info", "maxRetries", 3)
	maxRetries := 3
	var transactionInfo *models.TransactionInfo
	var transactionValue float64

	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.log.Debug("InitiateTransaction: Attempting to build transaction info", "attempt", attempt, "maxRetries", maxRetries)

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
			c.log.Info("InitiateTransaction: Successfully built transaction info",
				"attempt", attempt,
				"transactionValue", transactionValue,
				"hasRBT", len(transactionInfo.Tokens.RBT) > 0,
				"hasNFT", len(transactionInfo.Tokens.NFT) > 0,
				"hasSC", len(transactionInfo.Tokens.SmartContract) > 0,
				"hasCommittedTokens", len(transactionInfo.CommittedTokens) > 0,
			)
			break
		}

		// ONLY retry TOCTOU
		if !isTOCTOUConflict(err) {
			c.log.Error("InitiateTransaction: Failed to build transaction info (non-retryable)", "err", err, "attempt", attempt)
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

		backoff := retryWithRandomBackoff(attempt)
		c.log.Debug("InitiateTransaction: Backing off before retry", "attempt", attempt, "backoffMs", backoff.Milliseconds())
		time.Sleep(backoff)
	}

	// Fetch the list of dids from quorum_manager table
	//  We then loop over that list and queried from did table and pfetch the peerid
	c.log.Debug("InitiateTransaction: Fetching quorum addresses")
	quorumAddresses, err := c.GetAllQuorum()
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to get quorum address", "err", err)
		resp.Message = "InitiateTransaction: Failed to get quorum address: " + err.Error()
		return resp
	}
	if len(quorumAddresses) == 0 {
		c.log.Error("InitiateTransaction: No quorums available")
		resp.Message = "InitiateTransaction: No quorums available for transaction"
		return resp
	}
	c.log.Info("InitiateTransaction: Quorums found", "count", len(quorumAddresses), "primaryQuorum", quorumAddresses[0])

	// this will be a list of *ipfsport.Peer since we can have multiple quorums, we need to loop over them
	c.log.Debug("InitiateTransaction: Opening peer connection to quorum", "quorumDID", quorumAddresses[0])
	p, err := c.getPeer(quorumAddresses[0])
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to open peer connection", "err", err, "quorumDID", quorumAddresses[0])
		resp.Message = err.Error()
		return resp
	}
	defer p.Close()
	c.log.Debug("InitiateTransaction: Peer connection established", "quorumDID", quorumAddresses[0])

	// Pledge request
	c.log.Info("InitiateTransaction: Requesting pledge tokens from quorum", "quorumDID", quorumAddresses[0], "transactionValue", transactionValue)
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
		c.log.Error("InitiateTransaction: Failed to get pledge tokens from quorum", "err", err, "quorumDID", quorumAddresses[0])
		resp.Message = err.Error()
		return resp
	}
	c.log.Info("InitiateTransaction: Received pledge tokens", "tokenCount", len(pledgeTokenResponse.PledgeTokens), "quorumDID", quorumAddresses[0])

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
	c.log.Debug("InitiateTransaction: Calculating transaction ID")
	transactionId, err := util.GetTransactionID(transactionInfo)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to get transaction ID", "err", err)
		resp.Message = "InitiateTransaction: Failed to get transaction ID: " + err.Error()
		return resp
	}
	c.log.Info("InitiateTransaction: Transaction ID created", "transactionID", transactionId)

	// Signature done by the initiator on the  transactionInfo
	c.log.Debug("InitiateTransaction: Signing transaction with initiator DID", "did", initiatorDID)
	initiatorSignature, err := util.SignTransaction(dc, transactionInfo)
	if err != nil {
		c.log.Error("InitiateTransaction: Failed to sign transaction", "err", err, "did", initiatorDID)
		resp.Message = "InitiateTransaction: Failed to sign transaction: " + err.Error()
		return resp
	}
	c.log.Debug("InitiateTransaction: Transaction signed", "signatureLength", len(initiatorSignature))

	// Consensus request
	c.log.Info("InitiateTransaction: Initiating consensus with quorum", "quorumDID", quorumAddresses[0], "transactionID", transactionId)
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
		c.log.Error("InitiateTransaction: Consensus request failed", "err", err, "quorumDID", quorumAddresses[0])
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
		c.log.Error("InitiateTransaction: Consensus rejected", "message", consensusResponse.Message, "quorumDID", quorumAddresses[0])
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

	c.log.Info("InitiateTransaction: Consensus successful", "quorumDID", quorumAddresses[0], "quorumSignatureLength", len(consensusResponse.QuorumSignature))

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
	c.log.Info("InitiateTransaction: Persisting post-consensus state", "transactionID", transactionId, "did", initiatorDID, "role", wallet.ExecutionRoleInitiator)
	persistErr := c.w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo: transactionInfo,
		Signature:       signatureTobePublished,
		DID:             initiatorDID,
		ExecutionRole:   wallet.ExecutionRoleInitiator,
	})
	if persistErr != nil {
		c.log.Error("InitiateTransaction: Failed to persist post-consensus state", "err", persistErr, "transactionID", transactionId)
		resp.Message = "Failed to persist transaction: " + persistErr.Error()
		// txSucceeded stays false → deferred cleanup will release all locked tokens
		return resp
	} else {
		c.log.Info("InitiateTransaction: Post-consensus state persisted successfully", "transactionID", transactionId)
		// Mark success to prevent the deferred full-release from firing,
		// then release only the non-selected locked tokens (candidates not used in the transfer).
		// The selected tokens are now status=Transferred so they won't be touched.
		txSucceeded = true
		c.log.Debug("InitiateTransaction: Releasing non-selected locked tokens", "did", initiatorDID)
		if err := c.w.ReleaseAllLockedRBTTokensForDID(ctx, initiatorDID, reqID); err != nil {
			c.log.Error("InitiateTransaction: failed to release non-selected locked tokens", "err", err, "did", initiatorDID)
		} else {
			c.log.Debug("InitiateTransaction: Non-selected locked tokens released", "did", initiatorDID)
		}
		if err := c.w.ReleaseReferenceID(reqID); err != nil {
			c.log.Error("InitiateTransaction: failed to release reference ID", "err", err)
		}
	}

	if request.HasSmartContract() {
		c.log.Info("InitiateTransaction: Publishing SmartContract events", "transactionID", transactionId)
		c.publishSmartContractEvents(request, transactionId, initiatorDID, initiatorSignature, transactionInfo.Epoch)
	}

	if request.HasNFT() {
		c.log.Info("InitiateTransaction: Publishing NFT events", "transactionID", transactionId)
		c.publishNFTEvents(request, transactionId, initiatorDID, initiatorSignature, transactionInfo.Epoch)
	}

	//Publish transaction to the network
	c.log.Info("InitiateTransaction: Publishing transaction to network", "transactionID", transactionId)
	if _, err := util.PublishTransaction(c.ps, transactionInfo, signatureTobePublished, true, ""); err != nil {
		c.log.Error("InitiateTransaction: Failed to publish transaction", "err", err)
	}

	// Skip receiver sync for:
	// 1. SmartContract transactions (no owner transfer concept)
	// 2. NFT deployment (TransferNFTOwnership = false, Owner == Initiator)
	// 3. Self-transfers (Initiator == Owner)
	skipReceiverSync := false
	if request.HasSmartContract() {
		c.log.Info("InitiateTransaction: Skipping receiver sync for SmartContract transaction", "did", initiatorDID, "transactionID", transactionId)
		skipReceiverSync = true
	} else if request.HasNFT() && !request.Tokens.TransferNFTOwnership {
		c.log.Info("InitiateTransaction: Skipping receiver sync for NFT deployment", "did", initiatorDID, "transactionID", transactionId)
		skipReceiverSync = true
	} else if initiatorDID == nextOwnerDID {
		c.log.Info("InitiateTransaction: Skipping receiver sync (self-transfer)", "did", initiatorDID, "transactionID", transactionId)
		skipReceiverSync = true
	}

	if !skipReceiverSync {
		c.log.Info("InitiateTransaction: Starting async receiver sync", "receiver", nextOwnerDID, "transactionID", transactionId)
		go c.sendTokensToReceiver(nextOwnerDID, transactionId, transactionInfo, signatureTobePublished, request)
	} else {
		// For SmartContract/NFT deployments/self-transfers, persist receiver role immediately
		// This avoids JSON marshaling/unmarshaling mismatch issues
		persistErr := c.w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
			TransactionInfo: transactionInfo,
			Signature:       signatureTobePublished,
			DID:             nextOwnerDID,
			ExecutionRole:   wallet.ExecutionRoleReceiver,
		})
		if persistErr != nil {
			c.log.Error("InitiateTransaction: Failed to persist receiver state", "err", persistErr, "transactionID", transactionId, "did", nextOwnerDID)
		} else {
			c.log.Info("InitiateTransaction: Receiver state persisted", "transactionID", transactionId, "did", nextOwnerDID)
		}
	}

	// Return immediately - receiver sync happens in background (if needed)
	c.log.Info("InitiateTransaction: Transaction completed successfully", "transactionID", transactionId, "initiator", initiatorDID, "receiver", nextOwnerDID)
	resp.Status = true
	resp.Message = "Transfer initiated successfully"
	resp.Result = map[string]interface{}{
		"transactionID": transactionId,
	}
	return resp
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

// sendTokensToReceiver sends transaction tokens to the receiver synchronously.
// This function is designed avoid running as a goroutine and handles all errors externally.
// It will block or fail the main transaction flow.
func (c *Core) sendTokensToReceiverSync(
	receiverDID string,
	transactionID string,
	txInfo *models.TransactionInfo,
	signature *models.Signature,
	request *models.TransactionRequest,
) error {
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
		return fmt.Errorf("sendTokensToReceiver: Receiver offline, will sync later ",
			" receiver: ", receiverDID,
			" transactionID: ", transactionID,
			" err: ", err)
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
		return fmt.Errorf("sendTokensToReceiver: Failed to sync tokens to receiver (will retry later)",
			" receiver: ", receiverDID,
			" transactionID: ", transactionID,
			" err: ", err)
	}
	if !sendTokensResponse.Status {
		return fmt.Errorf("receiver rejected: %s", sendTokensResponse.Message)
	}

	c.log.Info("sendTokensToReceiver: Receiver sync completed successfully",
		"receiver", receiverDID,
		"transactionID", transactionID)

	return nil
}

// This has been added here since this is part of the transaction flow.
// Can be refactored in the future
func (c *Core) TransactionSetup() {
	c.l.AddRoute(APISendTokens, "POST", c.SendTokens)
	c.l.AddRoute(APISyncTransactionChain, "POST", c.SyncTransactionChain)
}

// This function has been added here since the other corresponding sync functions has not been added yet.
// Once the other sync functions and all are added, we can move this along with that.
func (c *Core) syncTransactionTokens(
	peer *ipfsport.Peer,
	tokens *models.TransactionTokens,
	NFTOwnershipTransfer bool,
) error {

	tokenGroups := map[string][]*models.TokenInfo{
		constants.TokenType_RBT: tokens.RBT,
		constants.TokenType_FT:  tokens.FT,
	}

	// Add NFT only if flag is true
	if NFTOwnershipTransfer {
		tokenGroups[constants.TokenType_NFT] = tokens.NFT
	}

	for tokenTypeStr, group := range tokenGroups {
		tokenType := models.GetTokenTypeID(tokenTypeStr)
		for _, token := range group {
			if token == nil {
				continue
			}
			//Handling the response in the future.
			err, _ := rubixsync.SyncTransactionChainFrom(peer, token.TokenID, tokenType, c.w, c.log)
			if err != nil {
				return err
			}
		}
	}

	return nil
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
	currentTxID, err := util.GetTransactionID(sendTokensRequest.TransactionInfo)
	if err != nil {
		c.log.Error("SendTokens: unable to get trnx id from sendTokensRequest.TransactionInfo, err: ", err)
	}

	var syncTokenIDs []string
	//var excludedTrnxIDs []string
	prevTxIDs := make(map[string]string)
	if txns := sendTokensRequest.TransactionInfo.Tokens; txns != nil {
		for _, t := range txns.RBT {
			if t != nil {
				syncTokenIDs = append(syncTokenIDs, t.TokenID)
				//excludedTrnxIDs = append(excludedTrnxIDs, currentTxID)
				if t.PreviousTransactionID != "" {
					prevTxIDs[t.TokenID] = t.PreviousTransactionID
				}
			}
		}
		for _, t := range txns.FT {
			if t != nil {
				syncTokenIDs = append(syncTokenIDs, t.TokenID)
				//excludedTrnxIDs = append(excludedTrnxIDs, currentTxID)
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
					//excludedTrnxIDs = append(excludedTrnxIDs, currentTxID)
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
		if err := c.SyncTransactionChainsFromPeer(initiatorDID, syncTokenIDs, prevTxIDs, []string{currentTxID}); err != nil {
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

func retryWithRandomBackoff(attempt int) time.Duration {
	return time.Duration(attempt*50+rand.Intn(1000)) * time.Millisecond
}
