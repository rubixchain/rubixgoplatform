package core

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/consensus"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
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
	c.l.AddRoute(APICreditStatus, "GET", c.creditStatus)
	c.l.AddRoute(APIQuorumConsensus, "POST", c.quorumConensus)
	c.l.AddRoute(APIQuorumCredit, "POST", c.quorumCredit)
	c.l.AddRoute(APIUpdatePledgeToken, "POST", c.updatePledgeToken)
	c.l.AddRoute(APISignatureRequest, "POST", c.signatureRequest)
	c.l.AddRoute(APISendReceiverToken, "POST", c.updateReceiverTokenHandle)
	c.l.AddRoute(APIConfirmTokenTransfer, "POST", c.confirmTokenTransfer)
	c.l.AddRoute(APIUnlockTokens, "POST", c.unlockTokens)
	c.l.AddRoute(APIUpdateTokenHashDetails, "POST", c.updateTokenHashDetails)
	c.l.AddRoute(APIAddUnpledgeDetails, "POST", c.addUnpledgeDetails)
	c.l.AddRoute(APISendFTToken, "POST", c.updateReceiverFTHandle)
	c.l.AddRoute(APICheckPinRole, "GET", c.checkPinRole)
	c.l.AddRoute(APIInitiateConsensus, "POST", c.initiateConsensusHandler)
	c.l.AddRoute(APIRequestPledgeToken, "POST", c.requestPledgeTokenHandler)
}

func (c *Core) requestPledgeTokenHandler(request *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuery(request, "did")
	var pledgeTokenRequest models.PledgeTokenRequest
	response := model.BasicResponse{Status: false}
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

	// Validation pipeline: run stateless checks BEFORE calling InitiateConsensus

	// Check 0: Nil-check TransactionInfo
	if consensusRequest.TransactionInfo == nil {
		c.log.Error("initiateConsensusHandler: missing TransactionInfo in consensus request")
		response.Message = "initiateConsensusHandler: missing TransactionInfo in consensus request"
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	txnInfo := consensusRequest.TransactionInfo

	// Check 1: Validate transaction info fields
	if err := consensus.ValidateTransactionInfoFields(txnInfo); err != nil {
		c.log.Error("initiateConsensusHandler: transaction info fields validation failed", "err", err)
		response.Message = "initiateConsensusHandler: " + err.Error()
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	// Compute txID once for logging and downstream use
	txID, err := util.GetTransactionID(txnInfo)
	if err != nil {
		c.log.Error("initiateConsensusHandler: failed to compute transaction ID", "err", err)
		response.Message = "initiateConsensusHandler: failed to compute transaction ID: " + err.Error()
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	// Check 2: Validate new token content for each RBT token
	if txnInfo.Tokens != nil {
		for _, rbtToken := range txnInfo.Tokens.RBT {
			if rbtToken == nil {
				continue
			}
			if err := consensus.ValidateNewTokenContent(rbtToken.TokenID, true, c.testnet, c.mainnet, c.localnet, c.log); err != nil {
				c.log.Error("initiateConsensusHandler: token content validation failed", "tokenID", rbtToken.TokenID, "err", err)
				response.Message = "initiateConsensusHandler: " + err.Error()
				return c.l.RenderJSON(request, response, http.StatusBadRequest)
			}
		}
	}

	// Check 3: Validate transaction value matches pledge
	if err := consensus.ValidateTransactionValueAndPledge(txnInfo); err != nil {
		c.log.Error("initiateConsensusHandler: transaction value/pledge validation failed", "err", err)
		response.Message = "initiateConsensusHandler: " + err.Error()
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	// Check 4: Verify initiator signature
	initiatorDC, err := c.SetupForienDID(txnInfo.Initiator, quorumDid)
	if err != nil {
		c.log.Error("initiateConsensusHandler: failed to setup initiator DID", "initiator", txnInfo.Initiator, "err", err)
		response.Message = "initiateConsensusHandler: failed to setup initiator DID: " + err.Error()
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	if err := util.VerifySignature(initiatorDC, txnInfo, consensusRequest.InitiatorSignature); err != nil {
		c.log.Error("initiateConsensusHandler: initiator signature verification failed", "err", err)
		response.Message = "initiateConsensusHandler: initiator signature verification failed"
		return c.l.RenderJSON(request, response, http.StatusBadRequest)
	}

	c.log.Info("initiateConsensusHandler: all stateless validations passed", "txID", txID)

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
	// tokenInfos is the slice that will actually be passed to PledgeV2.
	// In the current flow this is the same as pledgeTokenDetails; the
	// consistency check here ensures the two views remain in sync.
	tokenInfos := pledgeTokenDetails
	if len(tokenInfos) != len(pledgeTokenDetails) {
		c.log.Error("initiateConsensusHandler: pledge token count mismatch",
			"fromTxnInfo", len(pledgeTokenDetails),
			"actual", len(tokenInfos))
		response.Message = "initiateConsensusHandler: pledge token count mismatch"
		return c.l.RenderJSON(request, response, http.StatusInternalServerError)
	}
	for _, ti := range tokenInfos {
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

	if err := c.PledgeV2(
		context.Background(),
		tokenInfos,
		txID,
		quorumDid,
		txnInfo.Epoch,
		txnInfo.Network,
		txnInfo,
		consensusRequest.InitiatorSignature,
		consensusResponse.QuorumSignature,
	); err != nil {
		c.log.Error("initiateConsensusHandler: PledgeV2 failed", "err", err)
		response.Message = "initiateConsensusHandler: PledgeV2 failed: " + err.Error()
		// ### Note: consensus succeeded but pledge failed, which is a critical state.
		// Alerting is needed to investigate and resolve the underlying issue.
		return c.l.RenderJSON(request, response, http.StatusInternalServerError)
	}

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
