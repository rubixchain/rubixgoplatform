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
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
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

func (c *Core) initiateTransaction(reqID string, request *models.TransactionRequest) *models.BasicResponse {
	c.log.Info("InitiateTransaction: Starting transaction",
		"reqID", reqID,
		"initiator", request.Initiator,
		"owner", request.Owner,
		"hasNFT", request.HasNFT(),
		"hasSC", request.HasSmartContract(),
	)

	resp := &models.BasicResponse{
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
	//Here the c.publishTxn must be verified because the input type is *models.PubSubTxnInfo which need to be updated
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
				c.log.Error("InitiateTransaction: failed to release locked RBT tokens after failure", "err", releaseErr, "did", initiatorDID)
			} else {
				c.log.Info("InitiateTransaction: released locked RBT tokens after failed transaction", "did", initiatorDID)
			}

			// release all locked FTs
			if releaseFtErr := c.w.ReleaseAllLockedFTTokensForDID(ctx, initiatorDID, reqID); releaseFtErr != nil {
				c.log.Error("InitiateTransaction: failed to release locked FTs after failure", "err", releaseFtErr, "did", initiatorDID)
			} else {
				c.log.Info("InitiateTransaction: released locked FTs after failed transaction", "did", initiatorDID)
			}
			// Also release any locked NFT/SC tokens. These are locked by
			// BuildTransactionInfoFromRequest via QueryAndLockForExecution + batch
			// status UPDATE, but ReleaseAllLockedRBTTokensForDID only handles RBT.
			// Without this, NFT/SC tokens remain permanently stuck in Locked status
			// after a failed transaction (e.g. consensus rejection, insufficient
			// quorum liquidity).
			if released, releaseErr := c.w.ReleaseAllLockedNFTAndSCTokensForDID(ctx, initiatorDID); releaseErr != nil {
				c.log.Error("InitiateTransaction: failed to release locked NFT/SC tokens after failure", "err", releaseErr, "did", initiatorDID)
			} else if released > 0 {
				c.log.Info("InitiateTransaction: released locked NFT/SC tokens after failed transaction", "did", initiatorDID, "count", released)
			}
		} else {
			c.log.Debug("InitiateTransaction: Transaction succeeded, tokens will be transferred", "did", initiatorDID)
		}
	}()

	// Expand any child-mint entries (those with ParentNFTId set) into
	// parent-execute + child-deploy pairs before building the transaction info.
	// Server-generates the child NFT IDs and rewrites request.Tokens.NFT in place.
	mintedChildren, childToParent, err := c.expandChildMintEntries(request)
	if err != nil {
		c.log.Error("InitiateTransaction: child-mint expansion failed", "err", err)
		resp.Message = "InitiateTransaction: child-mint expansion failed: " + err.Error()
		return resp
	}
	if len(mintedChildren) > 0 {
		c.log.Info("InitiateTransaction: expanded child-mint entries",
			"childCount", len(mintedChildren))
	}

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

	// prepare did info to share with receiver
	algoID, err := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
	if err != nil {
		errMsg := fmt.Sprintf("InitiateTransaction: Failed to get algoID of Initiator, quorumDID: %s, err: %v", quorumAddresses[0], err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}
	pledgeTokenRequest.InitiatorPeerInfo = &models.DID{
		DID:    initiatorDID,
		PeerID: c.peerID,
		AlgoID: algoID,
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
		totalPledged = rubixmath.AddFloat(totalPledged, val)
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
		ReferenceId:          reqID,
		TransactionInfo:      transactionInfo,
		InitiatorSignature:   initiatorSignature,
		TransferNFTOwnership: request.Tokens.TransferNFTOwnership,
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
		TransactionInfo:      transactionInfo,
		Signature:            signatureTobePublished,
		DID:                  initiatorDID,
		ExecutionRole:        wallet.ExecutionRoleInitiator,
		TransferNFTOwnership: request.Tokens.TransferNFTOwnership,
		ChildToParent:        childToParent,
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
		// Auto-subscribe the deploying node to each SC topic so it receives
		// future execution notifications without a separate subscribe call.
		for _, sc := range request.GetAllSmartContracts() {
			if err := c.ps.SubscribeTopic(sc.SmartContractId, c.ContractCallBack); err != nil {
				if err.Error() == "topic already subscribed" {
					c.log.Debug("InitiateTransaction: already subscribed to SC topic", "topic", sc.SmartContractId)
				} else {
					c.log.Error("InitiateTransaction: failed to subscribe to SC topic", "topic", sc.SmartContractId, "err", err)
				}
			} else {
				c.log.Info("InitiateTransaction: subscribed to SC topic after deployment", "topic", sc.SmartContractId)
			}
		}
	}

	if request.HasNFT() {
		c.log.Info("InitiateTransaction: Publishing NFT events", "transactionID", transactionId)
		c.publishNFTEvents(request, transactionId, initiatorDID, initiatorSignature, transactionInfo.Epoch)
		// Auto-subscribe the deploying node to each NFT topic so it receives
		// future execution/transfer notifications without a separate subscribe call.
		for _, nft := range request.GetAllNFTs() {
			if err := c.ps.SubscribeTopic(nft.NFTId, c.NFTCallBack); err != nil {
				if err.Error() == "topic already subscribed" {
					c.log.Debug("InitiateTransaction: already subscribed to NFT topic", "topic", nft.NFTId)
				} else {
					c.log.Error("InitiateTransaction: failed to subscribe to NFT topic", "topic", nft.NFTId, "err", err)
				}
			} else {
				c.log.Info("InitiateTransaction: subscribed to NFT topic after deployment", "topic", nft.NFTId)
			}
		}
	}

	//Publish transaction to the network
	c.log.Info("InitiateTransaction: Publishing transaction to network", "transactionID", transactionId)
	if _, err := util.PublishTransaction(c.ps, transactionInfo, signatureTobePublished, true, ""); err != nil {
		c.log.Error("InitiateTransaction: Failed to publish transaction", "err", err)
	}

	// Skip receiver sync for:
	// 1. SmartContract-only transactions (no owner transfer concept)
	// 2. NFT-only execution without ownership transfer (no receiver to notify)
	// NOTE: Mixed transactions (e.g., RBT + NFT) must NOT skip — the RBT receiver needs sync.
	// NOTE: Self-transfer (initiator == owner) is naturally handled by the IsLocalDID
	// check below — IsLocalDID(initiatorDID) is true, so we take the local branch.
	skipReceiverSync := false
	hasRBT := request.Tokens.RBT > 0
	hasFT := len(request.Tokens.FT) > 0
	if request.HasSmartContract() && !hasRBT && !hasFT && !request.Tokens.TransferNFTOwnership {
		c.log.Info("InitiateTransaction: Skipping receiver sync for SmartContract transaction", "did", initiatorDID, "transactionID", transactionId)
		skipReceiverSync = true
	} else if request.HasNFT() && !request.Tokens.TransferNFTOwnership && !hasRBT && !hasFT {
		c.log.Info("InitiateTransaction: Skipping receiver sync for NFT execution (no ownership transfer)", "did", initiatorDID, "transactionID", transactionId)
		skipReceiverSync = true
	}

	if !skipReceiverSync {
		// Receiver sync needed — but skip network round-trip if receiver is on this node.
		// In that case, the initiator persistence (with isLocalTransfer=true) has already
		// recorded receiver-side token state via BuildPersistencePayload.
		var isOwnerDIDLocal bool
		isOwnerDIDLocal, err = c.w.IsLocalDID(nextOwnerDID)
		if err != nil {
			c.log.Error("InitiateTransaction: Failed to check if owner DID is local", "ownerDID", nextOwnerDID, "err", err)
			resp.Message = "InitiateTransaction: Failed to check if owner DID is local: " + err.Error()
			return resp
		}

		if !isOwnerDIDLocal {
			c.log.Info("InitiateTransaction: Sending tokens to receiver (synchronous)", "receiver", nextOwnerDID, "transactionID", transactionId)
			asyncMode := false
			if asyncMode {
				go c.sendTokensToReceiver(nextOwnerDID, transactionId, transactionInfo, signatureTobePublished, request)
			} else {
				if err := c.sendTokensToReceiverSync(nextOwnerDID, transactionId, transactionInfo, signatureTobePublished, request); err != nil {
					c.log.Error("InitiateTransaction: receiver sync failed", "err", err)
					resp.Status = false
					resp.Message = "Transaction failed: receiver sync failed: " + err.Error()
					return resp
				}
			}
		} else {
			c.log.Info("InitiateTransaction: Owner DID is local — receiver state handled by initiator persistence", "receiver", nextOwnerDID, "transactionID", transactionId)
		}
	} else {
		// SC/NFT skip cases — persist receiver role directly when receiver differs from initiator
		// (defensive fallback for SC/NFT edge cases where nextOwnerDID != initiator).
		// When initiator == receiver (self-deploy, self-execute) or when the receiver DID is empty
		// (SC deploy with no owner), the initiator persistence already set the correct final token
		// status (Deployed/Executed). Persisting a second time for the same DID would hit a
		// transaction_unit uniqueness conflict and leave NFT/SC tokens in a non-recoverable state.
		if nextOwnerDID != "" && nextOwnerDID != initiatorDID {
			persistErr := c.w.PersistPostConsensus(ctx, &wallet.PostConsensusPersistenceRequest{
				TransactionInfo:      transactionInfo,
				Signature:            signatureTobePublished,
				DID:                  nextOwnerDID,
				ExecutionRole:        wallet.ExecutionRoleReceiver,
				TransferNFTOwnership: request.Tokens.TransferNFTOwnership,
			})
			if persistErr != nil {
				c.log.Error("InitiateTransaction: Failed to persist receiver state", "err", persistErr, "transactionID", transactionId, "did", nextOwnerDID)
			} else {
				c.log.Info("InitiateTransaction: Receiver state persisted", "transactionID", transactionId, "did", nextOwnerDID)
			}
		} else {
			c.log.Info("InitiateTransaction: Skipping receiver persistence (initiator is receiver or no receiver DID)", "initiator", initiatorDID, "receiver", nextOwnerDID, "transactionID", transactionId)
		}
	}

	// Return immediately - receiver sync happens in background (if needed)
	c.log.Info("InitiateTransaction: Transaction completed successfully", "transactionID", transactionId, "initiator", initiatorDID, "receiver", nextOwnerDID)
	resp.Status = true
	resp.Message = fmt.Sprintf("Transaction %v completed successfully", transactionId)
	resp.Result = models.TransactionResult{
		TransactionID:     transactionId,
		MintedNFTChildren: mintedChildren,
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
	var sendTokensResponse models.BasicResponse
	sendTokensRequest.Tokens = txInfo.Tokens
	sendTokensRequest.TransactionInfo = txInfo
	sendTokensRequest.Signature = signature
	if request.HasNFT() {
		sendTokensRequest.NFTOwnershipTransfer = request.Tokens.TransferNFTOwnership
	}

	// prepare did info to share with receiver
	algoID, err := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
	if err != nil {
		c.log.Warn("sendTokensToReceiver: failed to get algo ID of Initiator (will sync later)",
			"receiver", receiverDID,
			"transactionID", transactionID,
			"err", err)
		return
	}
	sendTokensRequest.InitiatorPeerInfo = &models.DID{
		DID:    txInfo.Initiator,
		PeerID: c.peerID,
		AlgoID: algoID,
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
	var sendTokensResponse models.BasicResponse
	sendTokensRequest.Tokens = txInfo.Tokens
	sendTokensRequest.TransactionInfo = txInfo
	sendTokensRequest.Signature = signature
	if request.HasNFT() {
		sendTokensRequest.NFTOwnershipTransfer = request.Tokens.TransferNFTOwnership
	}

	// prepare did info to share with receiver
	algoID, err := c.w.GetDidAlgoIDByName(constants.DidAlgo_SECP256K1)
	if err != nil {
		errMsg := fmt.Sprintf("sendTokensToReceiver: failed to get algo ID of Initiator, will sync later; receiver: %s, transactionID: %s, err: %s", receiverDID, transactionID, err)
		c.log.Warn(errMsg)
		return fmt.Errorf(errMsg)
	}
	sendTokensRequest.InitiatorPeerInfo = &models.DID{
		DID:    txInfo.Initiator,
		PeerID: c.peerID,
		AlgoID: algoID,
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

func (c *Core) SendTokens(request *ensweb.Request) *ensweb.Result {
	crep := models.BasicResponse{Status: false}

	var sendTokensRequest models.SendTokensRequest
	err := c.l.ParseJSON(request, &sendTokensRequest)
	if err != nil {
		c.log.Error("SendTokens: Failed to parse json request", "err", err)
		crep.Message = "SendTokens: Failed to parse json request"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}

	// add initiator peer details to dids table, if not there
	if isExist := c.IsDIDExist(sendTokensRequest.InitiatorPeerInfo.DID); !isExist {
		if sendTokensRequest.InitiatorPeerInfo.PeerID == "" {
			errMsg := fmt.Sprintf("SendTokens: Failed to register initiator peer info, peerID is empty; err: %v", err)
			c.log.Error(errMsg)
			crep.Message = errMsg
			return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
		}
		if sendTokensRequest.InitiatorPeerInfo.PeerID != c.peerID {
			sendTokensRequest.InitiatorPeerInfo.Local = false
		} else {
			sendTokensRequest.InitiatorPeerInfo.Local = true
		}
		err = c.AddPeerDetails(*sendTokensRequest.InitiatorPeerInfo)
		if err != nil {
			errMsg := fmt.Sprintf("SendTokens: Failed to add initiator peer info to db; err: %v", err)
			c.log.Error(errMsg)
			crep.Message = errMsg
			return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
		}
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
		if err := c.SyncTransactionChainsFromPeer(initiatorDID, syncTokenIDs, prevTxIDs, []string{currentTxID}, sendTokensRequest.NFTOwnershipTransfer, false); err != nil {
			c.log.Warn("SendTokens: chain sync from sender failed (non-fatal)", "initiator", initiatorDID, "err", err)
		}
	}

	// On NFT transfer, materialise artifact + subscribe before persisting so
	// wallet state and on-disk files stay in sync.
	if sendTokensRequest.NFTOwnershipTransfer && sendTokensRequest.TransactionInfo.Tokens != nil {
		for _, nft := range sendTokensRequest.TransactionInfo.Tokens.NFT {
			if nft == nil || nft.TokenID == "" {
				continue
			}
			if err := c.ensureNFTArtifactAndSubscription(nft.TokenID); err != nil {
				c.log.Error("SendTokens: failed to materialise NFT locally", "nft", nft.TokenID, "err", err)
				crep.Message = fmt.Sprintf("SendTokens: failed to materialise NFT %s: %v", nft.TokenID, err)
				return c.l.RenderJSON(request, &crep, http.StatusInternalServerError)
			}
		}
	}

	persistErr := c.w.PersistPostConsensus(c.Ctx, &wallet.PostConsensusPersistenceRequest{
		TransactionInfo:           sendTokensRequest.TransactionInfo,
		Signature:                 sendTokensRequest.Signature,
		DID:                       receiverDID,
		ExecutionRole:             wallet.ExecutionRoleReceiver,
		SkipSignatureVerification: true,
		TransferNFTOwnership:      sendTokensRequest.NFTOwnershipTransfer,
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
	transactionDetail, err := c.w.GetTransactionByID(txId, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions details for tx: %v, err: %v", txId, err)
	}

	txInfo := &models.TransactionInfo{}
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

func (c *Core) GetTransactionsByDIDAndTokenType(did, tokenType string) ([]models.Transactions, error) {
	return c.w.GetTransactionsByDIDAndTokenType(did, tokenType)
}
