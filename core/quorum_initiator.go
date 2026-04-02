package core

import (
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
	networkMode, err := util.GetNetworkMode(c.testnet, c.mainnet, c.localnet)
	if err != nil {
		c.log.Error("requestPledgeTokenHandler : Failed to determine network mode", "err", err)
		response.Message = "requestPledgeTokenHandler : Failed to determine network mode"
		return c.l.RenderJSON(request, &response, http.StatusInternalServerError)
	}
	pledgeTokenResponse, err := consensus.ReqPledgeToken(dc, c.w, pledgeTokenRequest.TransactionValue, networkMode, c.log, c.ps, pledgeTokenRequest.ReferenceId)
	if err != nil {
		c.log.Error("requestPledgeTokenHandler : Failed to process pledge token request", "err", err)
		response.Message = "requestPledgeTokenHandler : " + err.Error()
		return c.l.RenderJSON(request, &response, http.StatusInternalServerError)
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

	consensusResponse, err := consensus.InitiateConsensus(consensusRequest, quorumDc, c.w, c.log)
	if err != nil {
		c.log.Error("initiateConsensusHandler : Consensus failed", "err", err)
		response.Message = err.Error()
		return c.l.RenderJSON(request, &response, http.StatusInternalServerError)
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
