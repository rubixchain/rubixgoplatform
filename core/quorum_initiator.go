package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	wallet "github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	MinQuorumRequired    int = 5
	MinConsensusRequired int = 5
)
const (
	RBTTransferMode int = iota
	NFTTransferMode
	DTCommitMode
	NFTSaleContractMode
	SmartContractDeployMode
	SmartContractExecuteMode
)
const (
	AlphaQuorumType int = iota
	BetaQuorumType
	GammaQuorumType
)

type ConensusRequest struct {
	ReqID              string   `json:"req_id"`
	Type               int      `json:"type"`
	Mode               int      `json:"mode"`
	SenderPeerID       string   `json:"sender_peerd_id"`
	ReceiverPeerID     string   `json:"receiver_peerd_id"`
	ContractBlock      []byte   `json:"contract_block"`
	QuorumList         []string `json:"quorum_list"`
	DeployerPeerID     string   `json:"deployer_peerd_id"`
	SmartContractToken string   `json:"smart_contract_token"`
	ExecuterPeerID     string   `json:"executor_peer_id"`
	TransactionID      string   `json:"transaction_id"`
}

type ConensusReply struct {
	ReqID    string `json:"req_id"`
	Status   bool   `json:"status"`
	Message  string `json:"message"`
	Hash     string `json:"hash"`
	ShareSig []byte `json:"share_sig"`
	PrivSig  []byte `json:"priv_sig"`
}

type ConsensusResult struct {
	RunningCount int
	SuccessCount int
	FailedCount  int
}

type ConsensusStatus struct {
	Credit     CreditScore
	PledgeLock sync.Mutex
	P          map[string]*ipfsport.Peer
	Result     ConsensusResult
}

type PledgeDetails struct {
	//TransferAmount         float64
	RemPledgeTokens        float64
	NumPledgedTokens       int
	PledgedTokens          map[string][]string
	PledgedTokenChainBlock map[string]interface{}
	TokenList              []string
}

type PledgeRequest struct {
	TokensRequired float64 `json:"tokens_required"`
}

type SignatureRequest struct {
	TokenChainBlock []byte `json:"token_chain_block"`
}

type SignatureReply struct {
	model.BasicResponse
	Signature string `json:"signature"`
}

type UpdatePledgeRequest struct {
	Mode            int      `json:"mode"`
	PledgedTokens   []string `json:"pledged_tokens"`
	TokenChainBlock []byte   `json:"token_chain_block"`
}

type SendTokenRequest struct {
	Address         string               `json:"peer_id"`
	TokenInfo       []contract.TokenInfo `json:"token_info"`
	TokenChainBlock []byte               `json:"token_chain_block"`
	QuorumList      []string             `json:"quorum_list"`
}

type PledgeReply struct {
	model.BasicResponse
	Tokens          []string  `json:"tokens"`
	TokenValue      []float64 `json:"token_value"`
	TokenChainBlock [][]byte  `json:"token_chain_block"`
}

type PledgeToken struct {
	Token string
	DID   string
}

type CreditScore struct {
	Credit []CreditSignature
}

type CreditSignature struct {
	Signature     string `json:"signature"`
	PrivSignature string `json:"priv_signature"`
	DID           string `json:"did"`
	Hash          string `json:"hash"`
}

type TokenArbitrationReq struct {
	Block []byte `json:"block"`
}

type ArbitaryStatus struct {
	p      *ipfsport.Peer
	sig    string
	ds     bool
	status bool
}

// PingSetup will setup the ping route
func (c *Core) QuroumSetup() {
	c.l.AddRoute(APICreditStatus, "GET", c.creditStatus)
	c.l.AddRoute(APIQuorumConsensus, "POST", c.quorumConensus)
	c.l.AddRoute(APIQuorumCredit, "POST", c.quorumCredit)
	c.l.AddRoute(APIReqPledgeToken, "POST", c.reqPledgeToken)
	c.l.AddRoute(APIUpdatePledgeToken, "POST", c.updatePledgeToken)
	c.l.AddRoute(APISignatureRequest, "POST", c.signatureRequest)
	c.l.AddRoute(APISendReceiverToken, "POST", c.updateReceiverToken)
	if c.arbitaryMode {
		c.l.AddRoute(APIMapDIDArbitration, "POST", c.mapDIDArbitration)
		c.l.AddRoute(APICheckDIDArbitration, "GET", c.chekDIDArbitration)
		c.l.AddRoute(APITokenArbitration, "POST", c.tokenArbitration)
		c.l.AddRoute(APIGetTokenNumber, "POST", c.getTokenNumber)
		c.l.AddRoute(APIGetMigratedTokenStatus, "POST", c.getMigratedTokenStatus)
		c.l.AddRoute(APISyncDIDArbitration, "POST", c.syncDIDArbitration)
	}
}

func (c *Core) SetupQuorum(didStr string, pwd string, pvtKeyPwd string) error {
	if !c.w.IsDIDExist(didStr) {
		c.log.Error("DID does not exist", "did", didStr)
		return fmt.Errorf("DID does not exist")
	}
	dc := did.InitDIDQuorumc(didStr, c.didDir, pwd)
	if dc == nil {
		c.log.Error("Failed to setup quorum")
		return fmt.Errorf("failed to setup quorum")
	}
	c.qc[didStr] = dc
	if pvtKeyPwd != "" {
		dc := did.InitDIDBasicWithPassword(didStr, c.didDir, pvtKeyPwd)
		if dc == nil {
			c.log.Error("Failed to setup quorum")
			return fmt.Errorf("failed to setup quorum")
		}
		c.pqc[didStr] = dc
	}
	c.up.RunUnpledge()
	return nil
}

func (c *Core) GetAllQuorum() []string {
	return c.qm.GetQuorum(QuorumTypeTwo, "")
}

func (c *Core) AddQuorum(ql []QuorumData) error {
	return c.qm.AddQuorum(ql)
}

func (c *Core) RemoveAllQuorum() error {
	// TODO:: needs to handle other types
	return c.qm.RemoveAllQuorum(QuorumTypeTwo)
}

func (c *Core) sendQuorumCredit(cr *ConensusRequest) {
	c.qlock.Lock()
	cs, ok := c.quorumRequest[cr.ReqID]
	c.qlock.Unlock()
	if !ok {
		c.log.Error("No quorum exist")
		return
	}
	for _, v := range cs.Credit.Credit {
		p, ok := cs.P[v.DID]
		if !ok {
			c.log.Error("Failed to get peer connection, not able to send credit", "addr", v.DID)
			continue
		}
		var resp model.BasicResponse
		err := p.SendJSONRequest("POST", APIQuorumCredit, nil, &cs.Credit, &resp, true)
		p.Close()
		if err != nil {
			c.log.Error("Failed to send quorum credits", "err", err)
			continue
		}
		if !resp.Status {
			c.log.Error("Quorum failed to accept credits", "msg", resp.Message)
			continue
		}
	}
	// c.qlock.Lock()
	// delete(c.quorumRequest, cr.ReqID)
	// c.qlock.Unlock()
}

func (c *Core) initiateConsensus(cr *ConensusRequest, sc *contract.Contract, dc did.DIDCrypto) (*wallet.TransactionDetails, map[string]map[string]float64, error) {
	cs := ConsensusStatus{
		Credit: CreditScore{
			Credit: make([]CreditSignature, 0),
		},
		P: make(map[string]*ipfsport.Peer),
		Result: ConsensusResult{
			RunningCount: 0,
			SuccessCount: 0,
			FailedCount:  0,
		},
	}
	reqPledgeTokens := float64(0)
	// TODO:: Need to correct for part tokens
	switch cr.Mode {
	case RBTTransferMode, NFTSaleContractMode:
		ti := sc.GetTransTokenInfo()
		for i := range ti {
			reqPledgeTokens = reqPledgeTokens + ti[i].TokenValue
		}

	case DTCommitMode:
		reqPledgeTokens = 1

	case SmartContractDeployMode:
		tokenInfo := sc.GetTransTokenInfo()
		for i := range tokenInfo {
			reqPledgeTokens = reqPledgeTokens + tokenInfo[i].TokenValue
		}
	case SmartContractExecuteMode:
		reqPledgeTokens = sc.GetTotalRBTs()
	}
	pd := PledgeDetails{
		//TransferAmount:         reqPledgeTokens,
		RemPledgeTokens:        floatPrecision(reqPledgeTokens, MaxDecimalPlaces),
		NumPledgedTokens:       0,
		PledgedTokens:          make(map[string][]string),
		PledgedTokenChainBlock: make(map[string]interface{}),
		TokenList:              make([]string, 0),
	}
	//getting last character from TID
	tid := util.HexToStr(util.CalculateHash(sc.GetBlock(), "SHA3-256"))
	lastCharTID := string(tid[len(tid)-1])
	cr.TransactionID = tid

	ql := c.qm.GetQuorum(cr.Type, lastCharTID) //passing lastCharTID as a parameter. Made changes in GetQuorum function to take 2 arguments
	if ql == nil || len(ql) < MinQuorumRequired {
		c.log.Error("Failed to get required quorums")
		return nil, nil, fmt.Errorf("failed to get required quorums")
	}
	var finalQl []string
	var errFQL error
	if cr.Type == 2 {
		finalQl, errFQL = c.GetFinalQuorumList(ql)
		if errFQL != nil {
			c.log.Error("unable to get consensus from quorum(s). err: ", errFQL)
			return nil, nil, errFQL
		}
		cr.QuorumList = finalQl
		if len(finalQl) != MinQuorumRequired {
			c.log.Error("quorum(s) are unavailable for this trnx")
			return nil, nil, fmt.Errorf("quorum(s) are unavailable for this trnx. retry trnx after some time")
		}
	} else {
		cr.QuorumList = ql
	}

	c.qlock.Lock()
	c.quorumRequest[cr.ReqID] = &cs
	c.pd[cr.ReqID] = &pd
	c.qlock.Unlock()
	defer func() {
		c.qlock.Lock()
		delete(c.quorumRequest, cr.ReqID)
		delete(c.pd, cr.ReqID)
		c.qlock.Unlock()
	}()
	for _, a := range cr.QuorumList {
		//This part of code is trying to connect to the quorums in quorum list, where various functions are called to pledge the tokens
		//and checking of transaction by the quorum i.e. consensus for the transaction. Once the quorum is connected, it pledges and
		//checks the consensus. For type 1 quorums, along with connecting to the quorums, we are checking the balance of the quorum DID
		//as well. Each quorums should pledge equal amount of tokens and hence, it should have a total of (Transacting RBTs/5) tokens
		//available for pledging.
		go c.connectQuorum(cr, a, AlphaQuorumType, sc)
	}
	loop := true
	var err error
	err = nil
	for {
		time.Sleep(time.Second)
		c.qlock.Lock()
		cs, ok := c.quorumRequest[cr.ReqID]
		if !ok {
			loop = false
			err = fmt.Errorf("invalid request")
		} else {
			if cs.Result.SuccessCount >= MinConsensusRequired {
				loop = false
			} else if cs.Result.RunningCount == 0 {
				loop = false
				err = fmt.Errorf("consensus failed, retry transaction after sometimes")
				c.log.Error("Consensus failed, retry transaction after sometimes")
			}
		}
		c.qlock.Unlock()
		if !loop {
			break
		}
	}
	if err != nil {
		return nil, nil, err
	}

	nb, err := c.pledgeQuorumToken(cr, sc, tid, dc)
	if err != nil {
		c.log.Error("Failed to pledge token", "err", err)
		return nil, nil, err
	}
	c.sendQuorumCredit(cr)
	ti := sc.GetTransTokenInfo()
	c.qlock.Lock()
	pds := c.pd[cr.ReqID]
	c.qlock.Unlock()
	pl := make(map[string]map[string]float64)
	for _, d := range cr.QuorumList {
		ds := strings.Split(d, ".")
		if len(ds) == 2 {
			ss, ok := pds.PledgedTokens[ds[1]]
			if ok {
				m := make(map[string]float64)
				for i := range ss {
					m[ss[i]] = 1
				}
				pl[ds[1]] = m
			}
		}
	}
	if cr.Mode == RBTTransferMode {
		rp, err := c.getPeer(cr.ReceiverPeerID + "." + sc.GetReceiverDID())
		if err != nil {
			c.log.Error("Receiver not connected", "err", err)
			return nil, nil, err
		}
		defer rp.Close()
		sr := SendTokenRequest{
			Address:         cr.SenderPeerID + "." + sc.GetSenderDID(),
			TokenInfo:       ti,
			TokenChainBlock: nb.GetBlock(),
			QuorumList:      cr.QuorumList,
		}
		var br model.BasicResponse
		err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)
		if err != nil {
			c.log.Error("Unable to send tokens to receiver", "err", err)
			return nil, nil, err
		}
		if strings.Contains(br.Message, "failed to sync tokenchain") {
			tokenPrefix := "Token: "
			issueTypePrefix := "issueType: "

			// Find the starting indexes of pt and issueType values
			ptStart := strings.Index(br.Message, tokenPrefix) + len(tokenPrefix)
			issueTypeStart := strings.Index(br.Message, issueTypePrefix) + len(issueTypePrefix)

			// Extracting the substrings from the message
			token := br.Message[ptStart : strings.Index(br.Message[ptStart:], ",")+ptStart]
			issueType := br.Message[issueTypeStart:]

			c.log.Debug("String: token is ", token, " issuetype is ", issueType)
			issueTypeInt, err1 := strconv.Atoi(issueType)
			if err1 != nil {
				errMsg := fmt.Sprintf("Consensus failed due to token chain sync issue, issueType string conversion, err %v", err1)
				c.log.Error(errMsg)
				return nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("issue type in int is ", issueTypeInt)
			syncIssueTokenDetails, err2 := c.w.ReadToken(token)
			if err2 != nil {
				errMsg := fmt.Sprintf("Consensus failed due to tokenchain sync issue, err %v", err2)
				c.log.Error(errMsg)
				return nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("sync issue token details ", syncIssueTokenDetails)
			if issueTypeInt == TokenChainNotSynced {
				syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
				c.log.Debug("sync issue token details status updated", syncIssueTokenDetails)
				c.w.UpdateToken(syncIssueTokenDetails)
				return nil, nil, errors.New(br.Message)
			}
		}
		if !br.Status {
			c.log.Error("Unable to send tokens to receiver", "msg", br.Message)
			return nil, nil, fmt.Errorf("unable to send tokens to receiver, " + br.Message)
		}

		//trigger pledge finality to the quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, pledgeFinalityError
		}

		err = c.w.TokensTransferred(sc.GetSenderDID(), ti, nb, rp.IsLocal())
		if err != nil {
			c.log.Error("Failed to transfer tokens", "err", err)
			return nil, nil, err
		}
		for _, t := range ti {
			c.w.UnPin(t.Token, wallet.PrevSenderRole, sc.GetSenderDID())
		}
		//call ipfs repo gc after unpinnning
		c.ipfsRepoGc()
		nbid, err := nb.GetBlockID(ti[0].Token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return nil, nil, err
		}

		td := wallet.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         nbid,
			Mode:            wallet.SendMode,
			SenderDID:       sc.GetSenderDID(),
			ReceiverDID:     sc.GetReceiverDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
		}
		return &td, pl, nil
	} else if cr.Mode == DTCommitMode {
		err = c.w.CreateTokenBlock(nb)
		if err != nil {
			c.log.Error("Failed to create token block", "err", err)
			return nil, nil, err
		}
		td := wallet.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			DateTime:        time.Now(),
			Status:          true,
		}

		//trigger pledge finality to the quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, pledgeFinalityError
		}
		return &td, pl, nil
	} else if cr.Mode == SmartContractDeployMode {
		//Create tokechain for the smart contract token and add genesys block
		err = c.w.AddTokenBlock(cr.SmartContractToken, nb)
		if err != nil {
			c.log.Error("smart contract token chain creation failed", "err", err)
			return nil, nil, err
		}
		//update smart contracttoken status to deployed in DB
		err = c.w.UpdateSmartContractStatus(cr.SmartContractToken, wallet.TokenIsDeployed)
		if err != nil {
			c.log.Error("Failed to update smart contract Token deploy detail in storage", err)
			return nil, nil, err
		}
		c.log.Debug("creating commited token block")
		//create new committed block to be updated to the commited RBT tokens
		err = c.createCommitedTokensBlock(nb, cr.SmartContractToken, dc)
		if err != nil {
			c.log.Error("Failed to create commited RBT tokens block ", "err", err)
			return nil, nil, err
		}
		//update committed RBT token with the new block also and lock the RBT
		//and change token status to commited, to prevent being used for txn or pledging
		commitedRbtTokens, err := nb.GetCommitedTokenDetials(cr.SmartContractToken)
		if err != nil {
			c.log.Error("Failed to fetch commited rbt tokens", "err", err)
			return nil, nil, err
		}
		err = c.w.CommitTokens(sc.GetDeployerDID(), commitedRbtTokens)
		if err != nil {
			c.log.Error("Failed to update commited RBT tokens in DB ", "err", err)
			return nil, nil, err
		}

		newBlockId, err := nb.GetBlockID(cr.SmartContractToken)
		if err != nil {
			c.log.Error("failed to get new block id ", "err", err)
			return nil, nil, err
		}

		//trigger pledge finality to the quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, pledgeFinalityError
		}

		//Todo pubsub - publish smart contract token details
		newEvent := model.NewContractEvent{
			SmartContractToken:     cr.SmartContractToken,
			Did:                    sc.GetDeployerDID(),
			Type:                   DeployType,
			SmartContractBlockHash: newBlockId,
		}

		err = c.publishNewEvent(&newEvent)
		if err != nil {
			c.log.Error("Failed to publish smart contract deployed info")
		}

		txnDetails := wallet.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         newBlockId,
			Mode:            wallet.DeployMode,
			DeployerDID:     sc.GetDeployerDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
		}
		return &txnDetails, pl, nil
	} else { //execute mode

		//Create tokechain for the smart contract token and add genesys block
		err = c.w.AddTokenBlock(cr.SmartContractToken, nb)
		if err != nil {
			c.log.Error("smart contract token chain creation failed", "err", err)
			return nil, nil, err
		}
		//update smart contracttoken status to deployed in DB
		err = c.w.UpdateSmartContractStatus(cr.SmartContractToken, wallet.TokenIsExecuted)
		if err != nil {
			c.log.Error("Failed to update smart contract Token execute detail in storage", err)
			return nil, nil, err
		}

		newBlockId, err := nb.GetBlockID(cr.SmartContractToken)
		if err != nil {
			c.log.Error("failed to get new block id ", "err", err)
			return nil, nil, err
		}

		//trigger pledge finality to the quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, pledgeFinalityError
		}

		//Todo pubsub - publish smart contract token details
		newEvent := model.NewContractEvent{
			SmartContractToken:     cr.SmartContractToken,
			Did:                    sc.GetExecutorDID(),
			Type:                   ExecuteType,
			SmartContractBlockHash: newBlockId,
		}

		err = c.publishNewEvent(&newEvent)
		if err != nil {
			c.log.Error("Failed to publish smart contract Executed info")
		}

		txnDetails := wallet.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         newBlockId,
			Mode:            wallet.ExecuteMode,
			DeployerDID:     sc.GetExecutorDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
		}
		return &txnDetails, pl, nil
	}
}

func (c *Core) quorumPledgeFinality(cr *ConensusRequest, newBlock *block.Block) error {
	c.log.Debug("Proceeding for pledge finality")
	c.qlock.Lock()
	pd, ok1 := c.pd[cr.ReqID]
	cs, ok2 := c.quorumRequest[cr.ReqID]
	c.qlock.Unlock()
	if !ok1 || !ok2 {
		c.log.Error("Invalid pledge request")
		return fmt.Errorf("invalid pledge request")
	}
	for k, v := range pd.PledgedTokens {
		p, ok := cs.P[k]
		if !ok {
			c.log.Error("Invalid pledge request")
			return fmt.Errorf("invalid pledge request")
		}
		if p == nil {
			c.log.Error("Invalid pledge request")
			return fmt.Errorf("invalid pledge request")
		}
		var qAddress string
		for _, quorumValue := range cr.QuorumList {
			// Check if the value of p.GetPeerDID() exists in the QuorumList as a substring
			if strings.Contains(quorumValue, p.GetPeerDID()) {
				qAddress = quorumValue
			}
		}
		qPeer, err := c.getPeer(qAddress)
		if err != nil {
			c.log.Error("Quorum not connected", "err", err)
			return err
		}
		defer qPeer.Close()
		var br model.BasicResponse
		ur := UpdatePledgeRequest{
			Mode:            cr.Mode,
			PledgedTokens:   v,
			TokenChainBlock: newBlock.GetBlock(),
		}

		err = qPeer.SendJSONRequest("POST", APIUpdatePledgeToken, nil, &ur, &br, true)
		if err != nil {
			c.log.Error("Failed to update pledge token status", "err", err)
			return fmt.Errorf("failed to update pledge token status")
		}
		if !br.Status {
			c.log.Error("Failed to update pledge token status", "msg", br.Message)
			return fmt.Errorf("failed to update pledge token status")
		}
	}
	return nil
}

func (c *Core) startConsensus(id string, qt int) {
	c.qlock.Lock()
	defer c.qlock.Unlock()
	cs, ok := c.quorumRequest[id]
	if !ok {
		return
	}
	switch qt {
	case 0:
		cs.Result.RunningCount++
	}
}

func (c *Core) finishConsensus(id string, qt int, p *ipfsport.Peer, status bool, hash string, ss []byte, ps []byte) {
	c.qlock.Lock()
	defer c.qlock.Unlock()
	cs, ok := c.quorumRequest[id]
	if !ok {
		if p != nil {
			p.Close()
		}
		return
	}
	switch qt {
	case 0:
		cs.Result.RunningCount--
		if status {
			did := p.GetPeerDID()
			if cs.Result.SuccessCount < MinConsensusRequired {
				csig := CreditSignature{
					Signature:     util.HexToStr(ss),
					PrivSignature: util.HexToStr(ps),
					DID:           did,
					Hash:          hash,
				}
				cs.P[did] = p
				cs.Credit.Credit = append(cs.Credit.Credit, csig)
			} else {
				p.Close()
			}
			cs.Result.SuccessCount++
		} else {
			cs.Result.FailedCount++
			if p != nil {
				p.Close()
			}
		}
	default:
		if p != nil {
			p.Close()
		}
	}
}

func (c *Core) connectQuorum(cr *ConensusRequest, addr string, qt int, sc *contract.Contract) {
	defer c.w.ReleaseAllLockedTokens()
	c.startConsensus(cr.ReqID, qt)
	var p *ipfsport.Peer
	var err error
	p, err = c.getPeer(addr)
	if err != nil {
		c.log.Error("Failed to get peer connection", "err", err)
		c.finishConsensus(cr.ReqID, qt, nil, false, "", nil, nil)
		return
	}
	err = c.initPledgeQuorumToken(cr, p, qt)
	if err != nil {
		c.log.Error("Failed to pledge token", "err", err)
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}
	var cresp ConensusReply
	err = p.SendJSONRequest("POST", APIQuorumConsensus, nil, cr, &cresp, true, 10*time.Minute)
	if err != nil {
		c.log.Error("Failed to get consensus", "err", err)
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}

	if strings.Contains(cresp.Message, "parent token is not in burnt stage") {
		ptPrefix := "pt: "
		issueTypePrefix := "issueType: "
		// Find the starting indexes of pt and issueType values
		ptStart := strings.Index(cresp.Message, ptPrefix) + len(ptPrefix)
		issueTypeStart := strings.Index(cresp.Message, issueTypePrefix) + len(issueTypePrefix)

		// Extracting the substrings from the message
		pt := cresp.Message[ptStart : strings.Index(cresp.Message[ptStart:], ",")+ptStart]
		issueType := cresp.Message[issueTypeStart:]
		c.log.Debug("String: pt is ", pt, " issuetype is ", issueType)

		c.log.Debug("sc.GetSenderDID()", sc.GetSenderDID(), "pt", pt)
		orphanChildTokenList, err1 := c.w.GetChildToken(sc.GetSenderDID(), pt)
		if err1 != nil {
			c.log.Error("Consensus failed due to orphan child token ", "err", err1)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		issueTypeInt, err2 := strconv.Atoi(issueType)
		c.log.Debug("issue type in int is ", issueTypeInt)
		if err2 != nil {
			c.log.Error("Consensus failed due to orphan child token, issueType string conversion", "err", err2)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Debug("Orphan token list ", orphanChildTokenList)
		if issueTypeInt == ParentTokenNotBurned {
			for _, orphanChild := range orphanChildTokenList {
				orphanChild.TokenStatus = wallet.TokenIsOrphaned
				c.log.Debug("Orphan token list status updated", orphanChild)
				c.w.UpdateToken(&orphanChild)
			}
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
	}

	if strings.Contains(cresp.Message, "failed to sync tokenchain") {
		tokenPrefix := "Token: "
		issueTypePrefix := "issueType: "

		// Find the starting indexes of pt and issueType values
		ptStart := strings.Index(cresp.Message, tokenPrefix) + len(tokenPrefix)
		issueTypeStart := strings.Index(cresp.Message, issueTypePrefix) + len(issueTypePrefix)

		// Extracting the substrings from the message
		token := cresp.Message[ptStart : strings.Index(cresp.Message[ptStart:], ",")+ptStart]
		issueType := cresp.Message[issueTypeStart:]

		c.log.Debug("String: token is ", token, " issuetype is ", issueType)
		issueTypeInt, err1 := strconv.Atoi(issueType)
		if err1 != nil {
			c.log.Error("Consensus failed due to token chain sync issue, issueType string conversion", "err", err1)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Debug("issue type in int is ", issueTypeInt)
		syncIssueTokenDetails, err2 := c.w.ReadToken(token)
		if err2 != nil {
			c.log.Error("Consensus failed due to tokenchain sync issue ", "err", err2)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Debug("sync issue token details ", syncIssueTokenDetails)
		if issueTypeInt == TokenChainNotSynced {
			syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
			c.log.Debug("sync issue token details status updated", syncIssueTokenDetails)
			c.w.UpdateToken(syncIssueTokenDetails)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
	}

	if strings.Contains(cresp.Message, "Token state is exhausted, Token is being Double spent.") {
		tokenPrefix := "Token : "
		tStart := strings.Index(cresp.Message, tokenPrefix) + len(tokenPrefix)
		var token string
		if tStart >= len(tokenPrefix) {
			token = cresp.Message[tStart:]
			c.log.Debug("Token is being Double spent. Token is ", token)
		}
		doubleSpendTokenDetails, err2 := c.w.ReadToken(token)
		if err2 != nil {
			c.log.Error("Consensus failed due to token being double spent ", "err", err2)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Debug("Double spend token details ", doubleSpendTokenDetails)
		doubleSpendTokenDetails.TokenStatus = wallet.TokenIsBeingDoubleSpent
		c.log.Debug("Double spend token details status updated", doubleSpendTokenDetails)
		c.w.UpdateToken(doubleSpendTokenDetails)
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}

	if !cresp.Status {
		c.log.Error("Failed to get consensus", "msg", cresp.Message)
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}
	c.finishConsensus(cr.ReqID, qt, p, true, cresp.Hash, cresp.ShareSig, cresp.PrivSig)
}

func (c *Core) pledgeQuorumToken(cr *ConensusRequest, sc *contract.Contract, tid string, dc did.DIDCrypto) (*block.Block, error) {
	c.qlock.Lock()
	pd, ok1 := c.pd[cr.ReqID]
	cs, ok2 := c.quorumRequest[cr.ReqID]
	c.qlock.Unlock()
	if !ok1 || !ok2 {
		c.log.Error("Invalid pledge request")
		return nil, fmt.Errorf("invalid pledge request")
	}
	ti := sc.GetTransTokenInfo()
	credit := make([]string, 0)
	for _, csig := range cs.Credit.Credit {
		jb, err := json.Marshal(csig)
		if err != nil {
			c.log.Error("Failed to parse quorum credit", "err", err)
			return nil, fmt.Errorf("failed to parse quorum credit")
		}
		credit = append(credit, string(jb))
	}
	ptds := make([]block.PledgeDetail, 0)
	for k, v := range pd.PledgedTokens {
		for _, t := range v {
			blk, ok := pd.PledgedTokenChainBlock[t].([]byte)
			if !ok {
				c.log.Error("failed to get pledge token block", "token", t)
				return nil, fmt.Errorf("failed to get pledge token block")
			}
			ptb := block.InitBlock(blk, nil)
			if ptb == nil {
				c.log.Error("invalid pledge token block", "token", t)
				return nil, fmt.Errorf("invalid pledge token block")
			}
			tt := ptb.GetTokenType(t)
			bid, err := ptb.GetBlockID(t)
			if err != nil {
				c.log.Error("Failed to get block id", "err", err, "token", t)
				return nil, fmt.Errorf("failed to get block id")
			}
			ptd := block.PledgeDetail{
				Token:        t,
				TokenType:    tt,
				DID:          k,
				TokenBlockID: bid,
			}
			ptds = append(ptds, ptd)
		}
	}

	tks := make([]block.TransTokens, 0)
	ctcb := make(map[string]*block.Block)

	if sc.GetDeployerDID() != "" {
		tt := block.TransTokens{
			Token:     ti[0].Token,
			TokenType: ti[0].TokenType,
		}
		tks = append(tks, tt)
		ctcb[ti[0].Token] = nil
	} else if sc.GetExecutorDID() != "" {
		tt := block.TransTokens{
			Token:     ti[0].Token,
			TokenType: ti[0].TokenType,
		}
		tks = append(tks, tt)
		b := c.w.GetLatestTokenBlock(ti[0].Token, ti[0].TokenType)
		ctcb[ti[0].Token] = b
	} else {
		for i := range ti {
			tt := block.TransTokens{
				Token:     ti[i].Token,
				TokenType: ti[i].TokenType,
			}
			tks = append(tks, tt)
			b := c.w.GetLatestTokenBlock(ti[i].Token, ti[i].TokenType)
			ctcb[ti[i].Token] = b
		}
	}

	bti := &block.TransInfo{
		Comment: sc.GetComment(),
		TID:     tid,
		Tokens:  tks,
	}
	//tokenList = append(tokenList, cr.PartTokens...)

	var tcb block.TokenChainBlock

	if cr.Mode == SmartContractDeployMode {
		bti.DeployerDID = sc.GetDeployerDID()

		var smartContractTokenValue float64

		commitedTokens := sc.GetCommitedTokensInfo()
		commitedTokenInfoArray := make([]block.TransTokens, 0)
		for i := range commitedTokens {
			commitedTokenInfo := block.TransTokens{
				Token:       commitedTokens[i].Token,
				TokenType:   commitedTokens[i].TokenType,
				CommitedDID: commitedTokens[i].OwnerDID,
			}
			commitedTokenInfoArray = append(commitedTokenInfoArray, commitedTokenInfo)
			smartContractTokenValue = smartContractTokenValue + commitedTokens[i].TokenValue
		}

		smartContractGensisBlock := &block.GenesisBlock{
			Type: block.TokenGeneratedType,
			Info: []block.GenesisTokenInfo{
				{Token: cr.SmartContractToken,
					CommitedTokens:     commitedTokenInfoArray,
					SmartContractValue: smartContractTokenValue},
			},
		}

		tcb = block.TokenChainBlock{
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      sc.GetDeployerDID(),
			TransInfo:       bti,
			QuorumSignature: credit,
			SmartContract:   sc.GetBlock(),
			GenesisBlock:    smartContractGensisBlock,
			PledgeDetails:   ptds,
		}
	} else if cr.Mode == SmartContractExecuteMode {
		bti.ExecutorDID = sc.GetExecutorDID()
		tcb = block.TokenChainBlock{
			TransactionType:   block.TokenGeneratedType,
			TokenOwner:        sc.GetExecutorDID(),
			TransInfo:         bti,
			QuorumSignature:   credit,
			SmartContract:     sc.GetBlock(),
			PledgeDetails:     ptds,
			SmartContractData: sc.GetSmartContractData(),
		}
	} else {
		bti.SenderDID = sc.GetSenderDID()
		bti.ReceiverDID = sc.GetReceiverDID()
		tcb = block.TokenChainBlock{
			TransactionType: block.TokenTransferredType,
			TokenOwner:      sc.GetReceiverDID(),
			TransInfo:       bti,
			QuorumSignature: credit,
			SmartContract:   sc.GetBlock(),
			PledgeDetails:   ptds,
		}
	}

	if cr.Mode == DTCommitMode {
		tcb.TransactionType = block.TokenCommittedType
	}
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block - qrm init")
		return nil, fmt.Errorf("failed to create new token chain block - qrm init")
	}
	blk := nb.GetBlock()
	if blk == nil {
		c.log.Error("Failed to get new block")
		return nil, fmt.Errorf("failed to get new block")
	}
	for k := range pd.PledgedTokens {
		p, ok := cs.P[k]
		if !ok {
			c.log.Error("Invalid pledge request, failed to get peer connection")
			return nil, fmt.Errorf("invalid pledge request, failed to get peer connection")
		}
		sr := SignatureRequest{
			TokenChainBlock: blk,
		}
		var srep SignatureReply
		err := p.SendJSONRequest("POST", APISignatureRequest, nil, &sr, &srep, true)
		if err != nil {
			c.log.Error("Failed to get signature from the quorum", "err", err)
			return nil, fmt.Errorf("failed to get signature from the quorum")
		}
		if !srep.Status {
			c.log.Error("Failed to get signature from the quorum", "msg", srep.Message)
			return nil, fmt.Errorf("failed to get signature from the quorum, " + srep.Message)
		}
		err = nb.ReplaceSignature(k, srep.Signature)
		if err != nil {
			c.log.Error("Failed to update signature to block", "err", err)
			return nil, fmt.Errorf("failed to update signature to block")
		}
	}
	/* for k, v := range pd.PledgedTokens {
		p, ok := cs.P[k]
		if !ok {
			c.log.Error("Invalid pledge request")
			return nil, fmt.Errorf("invalid pledge request")
		}
		if p == nil {
			c.log.Error("Invalid pledge request")
			return nil, fmt.Errorf("invalid pledge request")
		}
		var br model.BasicResponse
		ur := UpdatePledgeRequest{
			Mode:            cr.Mode,
			PledgedTokens:   v,
			TokenChainBlock: nb.GetBlock(),
		}
		err := p.SendJSONRequest("POST", APIUpdatePledgeToken, nil, &ur, &br, true)
		if err != nil {
			c.log.Error("Failed to update pledge token status", "err", err)
			return nil, fmt.Errorf("failed to update pledge token status")
		}
		if !br.Status {
			c.log.Error("Failed to update pledge token status", "msg", br.Message)
			return nil, fmt.Errorf("failed to update pledge token status")
		}
	} */
	return nb, nil
}

func (c *Core) initPledgeQuorumToken(cr *ConensusRequest, p *ipfsport.Peer, qt int) error {
	if qt == AlphaQuorumType {
		c.qlock.Lock()
		cs, ok := c.quorumRequest[cr.ReqID]
		c.qlock.Unlock()
		if !ok {
			c.qlock.Unlock()
			err := fmt.Errorf("invalid request")
			return err
		}
		cs.PledgeLock.Lock()
		c.qlock.Lock()
		pd, ok := c.pd[cr.ReqID]
		c.qlock.Unlock()
		if !ok {
			cs.PledgeLock.Unlock()
			err := fmt.Errorf("invalid pledge request")
			return err
		}

		//pledgeTokensPerQuorum := pd.TransferAmount / float64(MinQuorumRequired)

		// Request pledage token
		if pd.RemPledgeTokens != 0 {
			pr := PledgeRequest{
				TokensRequired: pd.RemPledgeTokens,
			}
			// l := len(pd.PledgedTokens)
			// for i := pd.NumPledgedTokens; i < l; i++ {
			// 	pr.Tokens = append(pr.Tokens, cr.WholeTokens[i])
			// }
			var prs PledgeReply
			err := p.SendJSONRequest("POST", APIReqPledgeToken, nil, &pr, &prs, true)
			if err != nil {
				c.log.Error("Invalid response for pledge request", "err", err)
				err := fmt.Errorf("invalid pledge request")
				cs.PledgeLock.Unlock()
				return err
			}
			if prs.Status {
				did := p.GetPeerDID()
				pd.PledgedTokens[did] = make([]string, 0)
				for i, t := range prs.Tokens {
					ptcb := block.InitBlock(prs.TokenChainBlock[i], nil)
					if !c.checkIsPledged(ptcb) {
						pd.NumPledgedTokens++
						pd.RemPledgeTokens = pd.RemPledgeTokens - prs.TokenValue[i]
						pd.RemPledgeTokens = floatPrecision(pd.RemPledgeTokens, 10)
						pd.PledgedTokenChainBlock[t] = prs.TokenChainBlock[i]
						pd.PledgedTokens[did] = append(pd.PledgedTokens[did], t)
						pd.TokenList = append(pd.TokenList, t)
					}
				}
				c.qlock.Lock()
				c.pd[cr.ReqID] = pd
				c.qlock.Unlock()
			}
		}
		cs.PledgeLock.Unlock()
	}
	count := 0
	for {
		time.Sleep(time.Second)
		count++
		c.qlock.Lock()
		pd, ok := c.pd[cr.ReqID]
		c.qlock.Unlock()
		if !ok {
			err := fmt.Errorf("invalid pledge request")
			return err
		}

		if pd.RemPledgeTokens == 0 {
			return nil
		} else if count == 300 {
			c.log.Error("Unable to pledge token")
			err := fmt.Errorf("unable to pledge token")
			return err
		}
	}
}

func (c *Core) checkDIDMigrated(p *ipfsport.Peer, did string) bool {
	var br model.BasicResponse
	q := make(map[string]string)
	q["olddid"] = did
	err := p.SendJSONRequest("GET", APICheckDIDArbitration, q, nil, &br, true, time.Minute*10)
	if err != nil {
		c.log.Error("Failed to get did detials from arbitray", "err", err)
		return false
	}
	if !br.Status {
		c.log.Error("Failed to get did detials from arbitray", "msg", br.Message)
		return false
	}
	return !br.Result.(bool)
}

func (c *Core) mapMigratedDID(p *ipfsport.Peer, olddid string, newdid string) bool {
	var br model.BasicResponse
	m := make(map[string]string)
	m["olddid"] = olddid
	m["newdid"] = newdid
	err := p.SendJSONRequest("POST", APIMapDIDArbitration, nil, &m, &br, true, time.Minute*10)
	if err != nil {
		c.log.Error("Failed to get did detials from arbitray", "err", err)
		return false
	}
	if !br.Status {
		c.log.Error("Failed to get did detials from arbitray", "msg", br.Message)
		return false
	}
	return true
}

func (c *Core) getArbitrationSignature(p *ipfsport.Peer, sr *SignatureRequest) (string, bool) {
	var srep SignatureReply
	err := p.SendJSONRequest("POST", APITokenArbitration, nil, sr, &srep, true, time.Minute*10)
	if err != nil {
		c.log.Error("Failed to get arbitray signature", "err", err)
		return "", false
	}
	if !srep.Status {
		c.log.Error("Failed to get arbitray signature", "msg", srep.Message)
		if strings.Contains(srep.Message, "token is already migrated") {
			return srep.Message, false
		}
		return "", false
	}
	return srep.Signature, true
}
func (c *Core) checkIsPledged(tcb *block.Block) bool {
	if strings.Compare(tcb.GetTransType(), block.TokenPledgedType) == 0 {
		return true
	}
	return false
}

func (c *Core) checkIsUnpledged(tcb *block.Block) bool {
	if strings.Compare(tcb.GetTransType(), block.TokenUnpledgedType) == 0 {
		return true
	}
	return false
}

func (c *Core) createCommitedTokensBlock(newBlock *block.Block, smartContractToken string, didCryptoLib did.DIDCrypto) error {
	commitedTokens, err := newBlock.GetCommitedTokenDetials(smartContractToken)
	if err != nil {
		c.log.Error("error fetching commited token details", err)
		return err
	}
	smartContractTokenBlockId, err := newBlock.GetBlockID(smartContractToken)
	if err != nil {
		c.log.Error("Failed to get block ID")
		return err
	}
	refID := fmt.Sprintf("%s,%d,%s", smartContractToken, newBlock.GetTokenType(smartContractToken), smartContractTokenBlockId)

	ctcb := make(map[string]*block.Block)
	tsb := make([]block.TransTokens, 0)

	for _, t := range commitedTokens {
		tokenInfoFromDB, err := c.w.ReadToken(t)
		if err != nil {
			c.log.Error("failed to read token from wallet")
			return err
		}
		ts := RBTString
		if tokenInfoFromDB.TokenValue != 1.0 {
			ts = PartString
		}
		tt := block.TransTokens{
			Token:     t,
			TokenType: c.TokenType(ts),
		}
		tsb = append(tsb, tt)
		lb := c.w.GetLatestTokenBlock(t, c.TokenType(ts))
		if lb == nil {
			c.log.Error("Failed to get token chain block")
			return fmt.Errorf("failed to get latest block")
		}
		ctcb[t] = lb
	}
	tcb := block.TokenChainBlock{
		TransactionType: block.TokenContractCommited,
		TokenOwner:      newBlock.GetDeployerDID(),
		TransInfo: &block.TransInfo{
			Comment: "Token is Commited at " + time.Now().String() + " for SmartContract Token : " + smartContractToken,
			RefID:   refID,
			Tokens:  tsb,
		},
	}
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block")
		return fmt.Errorf("Failed to create new token chain block")
	}
	err = nb.UpdateSignature(didCryptoLib)
	if err != nil {
		c.log.Error("Failed to update signature to block", "err", err)
		return fmt.Errorf("Failed to update signature to block")
	}
	err = c.w.CreateTokenBlock(nb)
	if err != nil {
		c.log.Error("Failed to update commited rbt token chain block", "err", err)
		return fmt.Errorf("Failed to update token chain block")
	}
	return nil
}
