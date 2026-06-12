package core

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	QuorumRequired       int = 1
	MinQuorumRequired    int = 1
	MinConsensusRequired int = 1
)

const (
	RBTTransferMode int = iota
	NFTDeployMode       // This value should be confirmed so that this won't break the existing code
	DTCommitMode
	NFTSaleContractMode
	SmartContractDeployMode
	SmartContractExecuteMode
	SelfTransferMode
	PinningServiceMode
	NFTExecuteMode
	FTTransferMode
)
const (
	RBTTokenType int = iota
	SmartContractTokenType
	NFTTokenType
	FTTokenType
)

// Timeout configuration for quorum operations
const (
	// Individual quorum timeout - how long to wait for each quorum response
	IndividualQuorumTimeout = 10 * time.Minute

	// Total timeout - maximum time to wait for all quorum responses
	TotalQuorumTimeout = 15 * time.Minute

	// Adaptive timeout multipliers
	AdaptiveTimeoutMultiplier = 2.5 // 2.5x the first response time
	MinAdaptiveTimeout        = 5 * time.Minute
	MaxAdaptiveTimeout        = 30 * time.Minute

	// Token-based timeout scaling (optimized for aggressive settings)
	BaseTokenProcessingTime = 150 * time.Millisecond // Reduced from 200ms with better performance
	TokenBatchSize          = 100                    // Tokens processed in parallel
	MinTokenTimeout         = 30 * time.Second       // Minimum timeout for token processing
)

// operation type in integer to define transaction type in string, to disstinguish between transactions
const (
	TokenSelfTransferredType int = 20
)

// ******************
// We can rename this file entirely
//Once the entire consensus logic has been written we can remove most of the functions here
// **********************

// PingSetup will setup the ping route
func (c *Core) QuorumSetup() {
	c.l.AddRoute(APIInitiateConsensus, "POST", c.initiateConsensusHandler)
	c.l.AddRoute(APIRequestPledgeToken, "POST", c.requestPledgeTokenHandler)
}

func (c *Core) requestPledgeTokenHandler(request *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuery(request, "did")
	var pledgeTokenRequest models.PledgeTokenRequest
	response := models.BasicResponse{Status: false}
	err := c.l.ParseJSON(request, &pledgeTokenRequest)
	c.log.Debug("Request for pledge tokens", "did", did)
	if err != nil {
		c.log.Error("requestPledgeTokenHandler : Failed to parse json request", "err", err)
		response.Message = "requestPledgeTokenHandler : Invalid request body"
		return c.l.RenderJSON(request, &response, http.StatusBadRequest)
	}
	_, ok := c.qc[did]
	if !ok {
		c.log.Error("requestPledgeTokenHandler : Quorum is not setup", "did", did)
		response.Message = "requestPledgeTokenHandler : Quorum is not setup"
		return c.l.RenderJSON(request, &response, http.StatusNotFound)
	}
	dc := c.pqc[did]

	// add initiator peer details to dids table, if it is not already present
	if isExist := c.IsDIDExist(pledgeTokenRequest.InitiatorPeerInfo.DID); !isExist {
		if pledgeTokenRequest.InitiatorPeerInfo.PeerID != c.peerID {
			pledgeTokenRequest.InitiatorPeerInfo.Local = false
		} else {
			pledgeTokenRequest.InitiatorPeerInfo.Local = true
		}
		err = c.AddPeerDetails(*pledgeTokenRequest.InitiatorPeerInfo)
		if err != nil {
			errMsg := fmt.Sprintf("requestPledgeTokenHandler: Failed to add initiator peer info to db; err: %v", err)
			c.log.Error(errMsg)
			response.Message = errMsg
			return c.l.RenderJSON(request, &response, http.StatusBadRequest)
		}
	}

	// Strict minimal liquidity guard (260409-0ko):
	// Cheap pre-check to short-circuit pledge requests when the quorum clearly
	// does not have enough free RBT balance. This avoids entering
	// LockTokensForSplit under load for trivially-rejectable requests. Note
	// that this check is advisory — concurrent requests between here and
	// LockTokensForSplit are still handled by the existing SKIP LOCKED /
	// retry logic in LockTokensForSplit. Uses strict "<" comparison: equal
	// balance is allowed through.
	freeBalance, balErr := c.w.GetFreeRBTBalanceByDID(did)
	if balErr != nil {
		c.log.Error("requestPledgeTokenHandler : failed to fetch free balance", "did", did, "err", balErr)
		response.Message = "requestPledgeTokenHandler : failed to fetch free balance"
		return c.l.RenderJSON(request, &response, http.StatusInternalServerError)
	}
	if freeBalance < pledgeTokenRequest.TransactionValue {
		c.log.Debug("requestPledgeTokenHandler : insufficient quorum liquidity",
			"did", did,
			"freeBalance", freeBalance,
			"required", pledgeTokenRequest.TransactionValue,
		)
		response.Message = "insufficient quorum liquidity"
		return c.l.RenderJSON(request, &response, http.StatusOK)
	}
	pledgeTokenResponse, err := consensus.ReqPledgeToken(dc, c.w, pledgeTokenRequest.TransactionValue, c.networkMode, c.log, c.ps, pledgeTokenRequest.ReferenceId)
	if err != nil {
		c.log.Error("requestPledgeTokenHandler : Failed to process pledge token request", "err", err)
		// Release ALL locked tokens since pledge selection failed — LockTokensForSplit locked them
		// but no subset was successfully selected, so all must be returned to Free.
		if releaseErr := c.w.ReleaseAllLockedRBTTokensForDID(c.w.Ctx, did, pledgeTokenRequest.ReferenceId); releaseErr != nil {
			c.log.Error("requestPledgeTokenHandler: failed to release locked tokens after pledge failure", "err", releaseErr)
		}
		response.Message = "requestPledgeTokenHandler : " + err.Error()
		return c.l.RenderJSON(request, &response, http.StatusInternalServerError)
	}

	// Pledge succeeded. Release only the non-selected locked tokens (candidates not chosen).
	// The selected pledge tokens MUST remain Locked — PledgeTokens (called during consensus)
	// checks that they are in Locked state before transitioning them to Pledged.
	selectedTokenIDs := make([]string, 0, len(pledgeTokenResponse.PledgeTokens))
	for _, t := range pledgeTokenResponse.PledgeTokens {
		if t != nil {
			selectedTokenIDs = append(selectedTokenIDs, t.TokenID)
		}
	}
	if releaseErr := c.w.ReleaseNonSelectedLockedRBTTokensForDID(c.w.Ctx, did, selectedTokenIDs, pledgeTokenRequest.ReferenceId); releaseErr != nil {
		c.log.Error("requestPledgeTokenHandler: failed to release non-selected locked tokens", "err", releaseErr)
	} else {
		c.log.Info("requestPledgeTokenHandler: released non-selected locked tokens", "did", did, "selectedCount", len(selectedTokenIDs))
	}

	return c.l.RenderJSON(request, &pledgeTokenResponse, http.StatusOK)

}

func (c *Core) initiateConsensusHandler(request *ensweb.Request) *ensweb.Result {
	quorumDid := c.l.GetQuery(request, "did")
	response := &models.ConsensusResponse{Status: false}

	c.log.Info("initiateConsensusHandler: Request received", "quorumDID", quorumDid)

	// Check if quorum is setup
	_, ok := c.qc[quorumDid]
	if !ok {
		c.log.Error("initiateConsensusHandler: Quorum is not setup", "did", quorumDid)
		response.Message = "initiateConsensusHandler: Quorum is not setup"
		return c.l.RenderJSON(request, response, http.StatusNotFound)
	}

	// Get DID crypto from pre-loaded quorum crypto map (c.pqc).
	// We cannot use c.SetupDID(request.ID, quorumDid) here because that function
	// expects a web request channel (from c.webReq map), but P2P requests don't
	// have web request channels. Since the quorum DID crypto is already loaded
	// during quorum setup, we access it directly from c.pqc, same approach as
	// requestPledgeTokenHandler.
	quorumDc := c.pqc[quorumDid]

	c.log.Info("initiateConsensusHandler: Quorum DID crypto loaded", "quorumDID", quorumDid)

	var consensusRequest models.ConsensusRequest
	err := c.l.ParseJSON(request, &consensusRequest)
	if err != nil {
		c.log.Error("initiateConsensusHandler : Failed to parse json request", "err", err)
		response.Message = "initiateConsensusHandler : Invalid request body"
		return c.l.RenderJSON(request, &response, http.StatusBadRequest)
	}
	c.log.Info("Consensus request parsed successfully", "request", consensusRequest)

	consensusSucceeded := false
	defer func() {
		if !consensusSucceeded {
			if releaseErr := c.w.ReleaseAllLockedRBTTokensForDID(c.w.Ctx, quorumDid, consensusRequest.ReferenceId); releaseErr != nil {
				c.log.Error("initiateConsensusHandler: failed to release quorum's locked tokens after failure", "err", releaseErr)
			} else {
				c.log.Info("initiateConsensusHandler: released quorum's locked RBT tokens after failed transaction", "did", quorumDid)
			}
		}
	}()

	// Validation pipeline: run stateless checks BEFORE calling InitiateConsensus

	// Check 0: Nil-check TransactionInfo
	if consensusRequest.TransactionInfo == nil {
		c.log.Error("initiateConsensusHandler: missing TransactionInfo in consensus request")
		response.Message = "initiateConsensusHandler: missing TransactionInfo in consensus request"
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	txnInfo := consensusRequest.TransactionInfo
	var transactionTokens []string
	if txnInfo.Tokens != nil {
		for _, rbt := range txnInfo.Tokens.RBT {
			transactionTokens = append(transactionTokens, rbt.TokenID)
		}

		for _, ft := range txnInfo.Tokens.FT {
			transactionTokens = append(transactionTokens, ft.TokenID)
		}

		for _, nft := range txnInfo.Tokens.NFT {
			transactionTokens = append(transactionTokens, nft.TokenID)
		}

		for _, sc := range txnInfo.Tokens.SmartContract {
			transactionTokens = append(transactionTokens, sc.TokenID)
		}
	}

	//Need to call ValidateTransaction function here
	initiatorDIDCrypto, err := c.InitialiseDID(txnInfo.Initiator)
	if err != nil {
		c.log.Error("initiateConsensusHandler: failed to initialise initiator DID", "err", err)
		response.Message = "initiateConsensusHandler: failed to setup initiator DID"
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	txn, err := wallet.BuildTransactionRecordFromPayload(txnInfo, &models.Signature{InitiatorSignature: consensusRequest.InitiatorSignature})
	if err != nil {
		c.log.Error("initiateConsensusHandler: failed to build transaction record", "err", err)
		response.Message = "initiateConsensusHandler: failed to build transaction record"
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}
	syncTxChains := func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error {
		return c.SyncTransactionChainsFromPeer(peerDID, tokenIDs, prevTxIDs, excludeTxIDs, false, c.fullNode)
	}
	isTransactionInfoValidated, err := consensus.ValidateTransaction(txn, c.fullNode, c.w, c.log, initiatorDIDCrypto, nil, c.testnet, c.mainnet, c.localnet, c.checkTokenStateHashPinned, syncTxChains, consensusRequest.TransferNFTOwnership)
	if err != nil || !isTransactionInfoValidated {
		c.log.Error("initiateConsensusHandler: transaction info validation failed", "err", err)
		if err != nil {
			response.Message = fmt.Sprintf("initiateConsensusHandler: transaction info validation failed: %v", err)
		} else {
			response.Message = "initiateConsensusHandler: transaction info validation failed"
		}
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	c.log.Info("initiateConsensusHandler: all stateless validations passed", "txID", txn.ID)

	consensusResponse, err := consensus.InitiateConsensus(consensusRequest, quorumDc, c.log)
	if err != nil {
		c.log.Error("initiateConsensusHandler : Consensus failed", "err", err)
		response.Message = err.Error()
		return c.l.RenderJSON(request, &response, http.StatusInternalServerError)
	}

	// Locate this quorum's pledge token entry by DID match, not by slice index.
	// txnInfo.Quorums may list multiple quorums; the entry for THIS quorum may
	// not be at index 0.
	var pledgeTokenDetails []*models.TokenInfo
	for _, q := range txnInfo.Quorums {
		if q.Did == quorumDid {
			pledgeTokenDetails = q.Tokens
			break
		}
	}
	if len(pledgeTokenDetails) == 0 {
		c.log.Error("initiateConsensusHandler: no quorum tokens found for DID", "quorumDID", quorumDid)
		response.Message = fmt.Errorf("no quorum tokens found for DID %s", quorumDid).Error()
		return c.l.RenderJSON(request, response, http.StatusInternalServerError)
	}

	// Token consistency validation BEFORE PledgeV2.
	// Extract token IDs from both views of the pledge set and confirm they
	// agree on count and membership. In the current single-source code path
	// these are the same slice, so this is a defensive check that will fire
	// only if a future refactor introduces a second token list out of band.
	pledgeTokenIDsFromTxnInfo := make(map[string]struct{}, len(pledgeTokenDetails))
	for _, ti := range pledgeTokenDetails {
		if ti == nil {
			c.log.Error("initiateConsensusHandler: nil TokenInfo in pledgeTokenDetails")
			response.Message = "initiateConsensusHandler: nil TokenInfo in pledgeTokenDetails"
			return c.l.RenderJSON(request, response, http.StatusInternalServerError)
		}
		pledgeTokenIDsFromTxnInfo[ti.TokenID] = struct{}{}
	}
	// pledgeTokenInfos is the slice that will actually be passed to PledgeV2.
	// In the current flow this is the same as pledgeTokenDetails; the
	// consistency check here ensures the two views remain in sync.
	pledgeTokenInfos := pledgeTokenDetails
	if len(pledgeTokenInfos) != len(pledgeTokenDetails) {
		c.log.Error("initiateConsensusHandler: pledge token count mismatch",
			"fromTxnInfo", len(pledgeTokenDetails),
			"actual", len(pledgeTokenInfos))
		response.Message = "initiateConsensusHandler: pledge token count mismatch"
		return c.l.RenderJSON(request, response, http.StatusInternalServerError)
	}
	for _, ti := range pledgeTokenInfos {
		if ti == nil {
			c.log.Error("initiateConsensusHandler: nil TokenInfo in tokenInfos")
			response.Message = "initiateConsensusHandler: nil TokenInfo in tokenInfos"
			return c.l.RenderJSON(request, response, http.StatusInternalServerError)
		}
		if _, ok := pledgeTokenIDsFromTxnInfo[ti.TokenID]; !ok {
			c.log.Error("initiateConsensusHandler: pledge token ID set mismatch",
				"unexpectedTokenID", ti.TokenID)
			response.Message = "initiateConsensusHandler: pledge token ID set mismatch"
			return c.l.RenderJSON(request, response, http.StatusInternalServerError)
		}
	}

	// Soft dedup guard (F-2): log + dedupe + metric, continue on duplicates.
	// Hard guard in PledgeV2 remains the last line of defense; this guard
	// exists so a duplicate under load is visible without cascading failure.
	{
		seenDedup := make(map[string]struct{}, len(pledgeTokenInfos))
		deduped := make([]*models.TokenInfo, 0, len(pledgeTokenInfos))
		dupCount := 0
		for _, ti := range pledgeTokenInfos {
			if ti == nil {
				continue
			}
			if _, dup := seenDedup[ti.TokenID]; dup {
				dupCount++
				c.log.Warn("initiateConsensusHandler: duplicate token in pledge tokenInfos (soft-deduped)",
					"metric", "pledge_v2_duplicate_token_total",
					"count", dupCount,
					"tokenID", ti.TokenID,
					"quorumDID", quorumDid,
					"txID", txn.ID,
					"referenceID", consensusRequest.ReferenceId,
				)
				continue
			}
			seenDedup[ti.TokenID] = struct{}{}
			deduped = append(deduped, ti)
		}
		pledgeTokenInfos = deduped
	}

	// VULN-4: validate lock_reference_id matches the incoming consensus ReferenceId
	// BEFORE pledging. Prevents replayed/interleaved requests from pledging tokens
	// that belong to a different reference_id.
	{
		tokenIDsForLockCheck := make([]string, 0, len(pledgeTokenInfos))
		for _, ti := range pledgeTokenInfos {
			tokenIDsForLockCheck = append(tokenIDsForLockCheck, ti.TokenID)
		}
		lockRefs, err := c.w.GetTokenLockReferenceIDs(context.Background(), tokenIDsForLockCheck)
		if err != nil {
			c.log.Error("initiateConsensusHandler: failed to read lock_reference_id for pledge tokens", "err", err)
			response.Message = "initiateConsensusHandler: failed to validate token locks: " + err.Error()
			return c.l.RenderJSON(request, response, http.StatusInternalServerError)
		}
		for _, id := range tokenIDsForLockCheck {
			ref, found := lockRefs[id]
			if !found {
				c.log.Error("initiateConsensusHandler: pledge token not found in tokens table",
					"tokenID", id, "quorumDID", quorumDid, "txID", txn.ID)
				response.Message = fmt.Sprintf("initiateConsensusHandler: token %q not found in tokens table", id)
				return c.l.RenderJSON(request, response, http.StatusBadRequest)
			}
			if ref == nil {
				c.log.Error("initiateConsensusHandler: pledge token has no lock_reference_id",
					"tokenID", id, "quorumDID", quorumDid, "txID", txn.ID)
				response.Message = fmt.Sprintf("initiateConsensusHandler: token %q has no lock_reference_id (was never locked by this flow)", id)
				return c.l.RenderJSON(request, response, http.StatusBadRequest)
			}
			if *ref != consensusRequest.ReferenceId {
				c.log.Error("initiateConsensusHandler: pledge token lock_reference_id mismatch",
					"tokenID", id, "expected", consensusRequest.ReferenceId, "got", *ref,
					"quorumDID", quorumDid, "txID", txn.ID)
				response.Message = fmt.Sprintf("initiateConsensusHandler: token %q lock_reference_id mismatch: expected %q, got %q",
					id, consensusRequest.ReferenceId, *ref)
				return c.l.RenderJSON(request, response, http.StatusBadRequest)
			}
		}
		c.log.Info("initiateConsensusHandler: lock_reference_id validation passed",
			"txID", txn.ID, "tokens", len(pledgeTokenInfos), "referenceID", consensusRequest.ReferenceId)
	}

	if err := c.PledgeV2(
		context.Background(),
		pledgeTokenInfos,
		txn.ID,
		quorumDid,
		txnInfo.Epoch,
		txnInfo.Network,
		txnInfo,
		consensusRequest.InitiatorSignature,
		consensusResponse.QuorumSignature,
		consensusRequest.ReferenceId,
		transactionTokens,
	); err != nil {
		c.log.Error("initiateConsensusHandler: PledgeV2 failed", "err", err)
		response.Message = "initiateConsensusHandler: PledgeV2 failed: " + err.Error()
		// ### Note: consensus succeeded but pledge failed, which is a critical state.
		// Alerting is needed to investigate and resolve the underlying issue.
		return c.l.RenderJSON(request, response, http.StatusInternalServerError)
	}
	consensusSucceeded = true 

	return c.l.RenderJSON(request, &consensusResponse, http.StatusOK)
}

func (c *Core) SetupQuorum(didStr string, pwd string, pvtKeyPwd string) error {
	didExists, err := c.w.IsDIDExists(didStr)
	if err != nil {
		return fmt.Errorf("unable to check if DID exists, err: %v", err)
	}
	if !didExists {
		return fmt.Errorf("DID %v meant to act as quorum doesn't exists", didStr)
	}

	if pvtKeyPwd == "" {
		c.log.Error("Failed to setup lite quorum as privPWD is not privided")
		return fmt.Errorf("failed to setup lite quorum, as privPWD is not provided")
	}
	quorum_dc := did.InitDIDQuorumLite(didStr, c.didDir, pvtKeyPwd)
	if quorum_dc == nil {
		c.log.Error("Failed to setup lite mode quorum")
		return fmt.Errorf("failed to setup quorum")
	}
	c.qc[didStr] = quorum_dc
	dc := did.InitDIDLiteWithPassword(didStr, c.didDir, pvtKeyPwd)
	if dc == nil {
		c.log.Error("Failed to setup quorum as dc is nil")
		return fmt.Errorf("failed to setup quorum")
	}
	c.pqc[didStr] = dc

	// Subscribe to "rubix_txns" event
	c.SubscribeTxnSetup()
	c.startStaleLockedTokenUnlocker()

	return nil
}

func (c *Core) GetAllQuorum() ([]string, error) {
	qmList, err := c.w.GetAllQuorums()
	if err != nil {
		return nil, err
	}

	var quorumList []string = make([]string, 0)

	for _, quorum := range qmList {
		quorumList = append(quorumList, quorum.Did)
	}

	return quorumList, nil
}

func (c *Core) AddQuorum(quorumDid string) error {
	quormManager := models.QuorumManager{
		Did: quorumDid,
	}

	return c.w.AddQuorum(quormManager)
}

func (c *Core) RemoveAllQuorum() error {
	return c.w.RemoveAllQuorums()
}
