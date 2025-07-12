package core

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	wallet "github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	QuorumRequired       int = 7
	MinQuorumRequired    int = 5
	MinConsensusRequired int = 5
)
const (
	RBTTransferMode int = iota
	NFTDeployMode       //This value should be confirmed so that this won't break the existing code
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
	AlphaQuorumType int = iota
	BetaQuorumType
	GammaQuorumType
)

type ConensusRequest struct {
	ReqID              string       `json:"req_id"`
	Type               int          `json:"type"`
	Mode               int          `json:"mode"`
	SenderPeerID       string       `json:"sender_peerd_id"`
	ReceiverPeerID     string       `json:"receiver_peerd_id"`
	ContractBlock      []byte       `json:"contract_block"`
	QuorumList         []string     `json:"quorum_list"`
	DeployerPeerID     string       `json:"deployer_peerd_id"`
	SmartContractToken string       `json:"smart_contract_token"`
	ExecuterPeerID     string       `json:"executor_peer_id"`
	TransactionID      string       `json:"transaction_id"`
	TransactionEpoch   int          `json:"transaction_epoch"`
	PinningNodePeerID  string       `json:"pinning_node_peer_id"`
	NFT                string       `json:"nft"`
	FTinfo             model.FTInfo `json:"ft_info"`
	// TransTokenSyncInfo map[string]GenesisAndLatestBlocks `json:"tokens_sync_info"`
}

type ConensusReply struct {
	ReqID    string   `json:"req_id"`
	Status   bool     `json:"status"`
	Message  string   `json:"message"`
	Result   []string `json:"result"`
	Hash     string   `json:"hash"`
	ShareSig []byte   `json:"share_sig"`
	PrivSig  []byte   `json:"priv_sig"`
}

type ConsensusResult struct {
	RunningCount          int
	SuccessCount          int
	FailedCount           int
	PledgingDIDSignStatus bool
}

type ConsensusStatus struct {
	Credit     CreditScore
	PledgeLock sync.Mutex
	P          map[string]*ipfsport.Peer
	Result     ConsensusResult
}

type PledgeDetails struct {
	TransferAmount         float64
	RemPledgeTokens        float64
	NumPledgedTokens       int
	PledgedTokens          map[string][]string
	PledgedTokenChainBlock map[string]interface{}
	TokenList              []Token
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
	Mode                        int      `json:"mode"`
	PledgedTokens               []string `json:"pledged_tokens"`
	TokenChainBlock             []byte   `json:"token_chain_block"`
	TransferredTokenStateHashes []string `json:"token_state_hash_info"`
	TransactionID               string   `json:"transaction_id"`
	TransactionEpoch            int      `json:"transaction_epoch"`
}

type SendTokenRequest struct {
	Address            string               `json:"peer_id"`
	TokenInfo          []contract.TokenInfo `json:"token_info"`
	TokenChainBlock    []byte               `json:"token_chain_block"`
	QuorumList         []string             `json:"quorum_list"`
	QuorumInfo         []QuorumDIDPeerMap   `json:"quorum_info"`
	TransactionEpoch   int                  `json:"transaction_epoch"`
	PinningServiceMode bool                 `json:"pinning_service_mode"`
	FTInfo             model.FTInfo         `json:"ft_info"`
	// TransTokenSyncInfo map[string]GenesisAndLatestBlocks `json:"token_chain_sync_info"`
}

type GenesisAndLatestBlocks struct {
	GenesisBlock       []byte `json:"genesis"`
	LatestBlock        []byte `json:"latest"`
	ParentGenesisBlock []byte `json:"parent_genesis"`
	ParentLatestBlock  []byte `json:"parent_latest"`
}

type SendFTRequest struct {
	Address          string               `json:"peer_id"`
	TokenInfo        []contract.TokenInfo `json:"token_info"`
	TokenChainBlock  []byte               `json:"token_chain_block"`
	QuorumList       []string             `json:"quorum_list"`
	QuorumInfo       []QuorumDIDPeerMap   `json:"quorum_info"`
	TransactionEpoch int                  `json:"transaction_epoch"`
	FTInfo           model.FTInfo         `json:"ft_info"`
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
	SignType      string `json:"sign_type"` //represents sign type (PkiSign == 0 or NlssSign==1)
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

type TokenList struct {
	Tokens []string
	DID    string
}

// PingSetup will setup the ping route
func (c *Core) QuroumSetup() {
	c.l.AddRoute(APICreditStatus, "GET", c.creditStatus)
	c.l.AddRoute(APIQuorumConsensus, "POST", c.quorumConensus)
	c.l.AddRoute(APIQuorumCredit, "POST", c.quorumCredit)
	c.l.AddRoute(APIReqPledgeToken, "POST", c.reqPledgeToken)
	c.l.AddRoute(APIUpdatePledgeToken, "POST", c.updatePledgeToken)
	c.l.AddRoute(APISignatureRequest, "POST", c.signatureRequest)
	c.l.AddRoute(APISendReceiverToken, "POST", c.updateReceiverTokenHandle)
	c.l.AddRoute(APIUnlockTokens, "POST", c.unlockTokens)
	c.l.AddRoute(APIUpdateTokenHashDetails, "POST", c.updateTokenHashDetails)
	c.l.AddRoute(APIAddUnpledgeDetails, "POST", c.addUnpledgeDetails)
	c.l.AddRoute(APIRecoverPinnedRBT, "POST", c.recoverPinnedToken)
	c.l.AddRoute(APIRequestSigningHash, "GET", c.requestSigningHash)
	c.l.AddRoute(APISendFTToken, "POST", c.updateReceiverFTHandle)
	c.l.AddRoute(APICheckPinRole, "GET", c.checkPinRole)
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

	dt, err := c.w.GetDID(didStr)
	if err != nil {
		c.log.Error("DID could not fetch", "did", didStr)
		return fmt.Errorf("DID does not exist")
	}

	//To support NLSS backward compatibility,
	//If the Quorum's did is created in lite mode,
	//it will initiate DIDQuorum_Lt, and if  it is in basic mode,
	//it will initiate DIDQuorumc
	switch dt.Type {
	case did.LiteDIDMode:
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
	case did.BasicDIDMode:
		dc := did.InitDIDQuorumc(didStr, c.didDir, pwd)
		if dc == nil {
			c.log.Error("Failed to setup basic mode quorum")
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
	default:
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
	}

	return nil
}

func (c *Core) GetAllQuorum() []string {
	return c.qm.GetQuorum(QuorumTypeTwo, "", c.peerID)
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

func (c *Core) initiateConsensus(cr *ConensusRequest, sc *contract.Contract, dc did.DIDCrypto) (*model.TransactionDetails, map[string]map[string]float64, *PledgeDetails, error) {
	cs := ConsensusStatus{
		Credit: CreditScore{
			Credit: make([]CreditSignature, 0),
		},
		P: make(map[string]*ipfsport.Peer),
		Result: ConsensusResult{
			RunningCount:          0,
			SuccessCount:          0,
			FailedCount:           0,
			PledgingDIDSignStatus: false,
		},
	}
	reqPledgeTokens := float64(0)

	// TODO:: Need to correct for part tokens
	switch cr.Mode {
	case RBTTransferMode, NFTSaleContractMode, SelfTransferMode, PinningServiceMode:
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
	case NFTDeployMode:
		reqPledgeTokens := sc.GetTotalRBTs()
		if reqPledgeTokens == 0 {
			reqPledgeTokens = 1
		}
	case SmartContractExecuteMode, NFTExecuteMode:
		reqPledgeTokens = sc.GetTotalRBTs()
	case FTTransferMode:
		ti := sc.GetTransTokenInfo()
		for i := range ti {
			reqPledgeTokens = reqPledgeTokens + ti[i].TokenValue
		}
	}
	minValue := MinDecimalValue(MaxDecimalPlaces)
	minTotalPledgeAmount := minValue * float64(MinQuorumRequired)
	if reqPledgeTokens < minTotalPledgeAmount {
		reqPledgeTokens = minTotalPledgeAmount
	}
	reqPledgeTokens = floatPrecision(reqPledgeTokens, MaxDecimalPlaces)
	pd := PledgeDetails{
		TransferAmount:         reqPledgeTokens,
		RemPledgeTokens:        floatPrecision(reqPledgeTokens, MaxDecimalPlaces),
		NumPledgedTokens:       0,
		PledgedTokens:          make(map[string][]string),
		PledgedTokenChainBlock: make(map[string]interface{}),
		TokenList:              make([]Token, 0),
	}
	//getting last character from TID
	tid := util.HexToStr(util.CalculateHash(sc.GetBlock(), "SHA3-256"))
	lastCharTID := string(tid[len(tid)-1])
	cr.TransactionID = tid

	ql := c.qm.GetQuorum(cr.Type, lastCharTID, c.peerID) //passing lastCharTID as a parameter. Made changes in GetQuorum function to take 2 arguments
	if ql == nil || len(ql) < MinQuorumRequired {
		c.log.Error("Failed to get required quorums")
		return nil, nil, nil, fmt.Errorf("failed to get required quorums")
	}

	var activeQuorumLists []string

	if cr.Type == 2 {
		activeQuorumLists = c.GetActiveQuorums(ql)
		c.log.Debug(fmt.Sprintf("Total Active Quorums: %d", len(activeQuorumLists)))
		if len(activeQuorumLists) < MinQuorumRequired {
			c.log.Error("quorum(s) are unavailable for this trnx")
			return nil, nil, nil, fmt.Errorf("quorum(s) are unavailable for this trnx. retry trnx after some time")
		}

		cr.QuorumList = activeQuorumLists
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
	c.quorumCount = QuorumRequired - len(cr.QuorumList)
	c.noBalanceQuorumCount = QuorumRequired - len(cr.QuorumList)
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
				err = fmt.Errorf("consensus failed, retry transaction after sometime")
				c.log.Error("Consensus failed, retry transaction after sometime")
			}
		}
		c.qlock.Unlock()
		if !loop {
			break
		}
	}
	if err != nil {
		unlockErr := c.checkLockedTokens(cr, ql)
		if unlockErr != nil {
			c.log.Error(unlockErr.Error() + "Locked tokens could not be unlocked")
		}
		return nil, nil, nil, err
	}

	if c.noBalanceQuorumCount > (QuorumRequired - MinConsensusRequired) {
		c.log.Error("Consensus failed due to insufficient balance in Quorum(s), Retry transaction after sometime")
		return nil, nil, nil, fmt.Errorf("Consensus failed due to insufficient balance in Quorum(s)")
	}

	nb, err := c.pledgeQuorumToken(cr, sc, tid, dc)
	if err != nil {
		c.log.Error("Failed to pledge token", "err", err)
		return nil, nil, nil, err
	}

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

	switch cr.Mode {
	case RBTTransferMode:
		rp, err := c.getPeer(cr.ReceiverPeerID + "." + sc.GetReceiverDID())
		if err != nil {
			c.log.Error("Receiver not connected", "err", err)
			return nil, nil, nil, err
		}
		defer rp.Close()

		var txnTokenHashes []string = make([]string, 0)
		for _, info := range ti {
			t := info.Token
			blockId, _ := nb.GetBlockID(t)
			tokenIDTokenStateData := t + blockId
			tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
			tokenIDTokenStateHash, _ := c.ipfs.Add(tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			txnTokenHashes = append(txnTokenHashes, tokenIDTokenStateHash)
		}
		//calling quorum pledge finality before calling the APISendReceiver Token
		//trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, txnTokenHashes, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		sr := SendTokenRequest{
			Address:            cr.SenderPeerID + "." + sc.GetSenderDID(),
			TokenInfo:          ti,
			TokenChainBlock:    nb.GetBlock(),
			QuorumList:         cr.QuorumList,
			TransactionEpoch:   cr.TransactionEpoch,
			PinningServiceMode: false,
			// TransTokenSyncInfo: cr.TransTokenSyncInfo,
		}

		//fetching quorums' info from PeerDIDTable to share with the receiver
		for _, qrm := range sr.QuorumList {
			//fetch peer id & did of the quorum
			qpid, qdid, ok := util.ParseAddress(qrm)
			if !ok {
				c.log.Error("could not parse quorum address:", qrm)
			}

			var qrmInfo QuorumDIDPeerMap
			//fetch did info of the quorum
			qDidInfo, err := c.GetPeerDIDInfo(qdid)
			if err != nil {
				if strings.Contains(err.Error(), "retry") {
					c.AddPeerDetails(*qDidInfo)
				}
			}
			if qDidInfo.DIDType == nil {
				qdidPeerMap, err := c.GetPeerDIDInfo(qdid)
				if err != nil {
					c.log.Error("could not fetch did type of quorum", qdid, "err", err)
					qrmInfo.DIDType = nil
				} else {
					qrmInfo.DIDType = qdidPeerMap.DIDType
					c.log.Debug("DID type of quorum is fetched from explorer", qdid, "DID type", qrmInfo.DIDType)
				}
			}

			if qDidInfo == nil || *qDidInfo.DIDType == -1 {
				c.log.Error("could not fetch did type of quorum", qdid, "err", err)
				qrmInfo.DIDType = nil
			} else {
				qrmInfo.DIDType = qDidInfo.DIDType
			}
			if qpid == "" {
				qpid = qDidInfo.PeerID
			}
			qrmInfo.DID = qdid
			qrmInfo.PeerID = qpid
			//add quorum details to the data to be shared
			sr.QuorumInfo = append(sr.QuorumInfo, qrmInfo)
		}

		var br model.BasicResponse
		err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)
		if err != nil {
			c.log.Error("Unable to send tokens to receiver", "err", err)
			return nil, nil, nil, err
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
				return nil, nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("issue type in int is ", issueTypeInt)
			syncIssueTokenDetails, err2 := c.w.ReadToken(token)
			if err2 != nil {
				errMsg := fmt.Sprintf("Consensus failed due to tokenchain sync issue, err %v", err2)
				c.log.Error(errMsg)
				return nil, nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("sync issue token details ", syncIssueTokenDetails)
			if issueTypeInt == TokenChainNotSynced {
				syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
				c.log.Debug("sync issue token details status updated", syncIssueTokenDetails)
				c.w.UpdateToken(syncIssueTokenDetails)
				return nil, nil, nil, errors.New(br.Message)
			}
		}
		if !br.Status {
			c.log.Error("Unable to send tokens to receiver", "msg", br.Message)
			return nil, nil, nil, fmt.Errorf("unable to send tokens to receiver, " + br.Message)
		}

		//TODO:Remove this below commented out code, once after testing the quorum pledge finality location change.
		// br.Result will contain the new token state after sending tokens to receiver as a response to APISendReceiverToken
		// newtokenhashresult, ok := br.Result.([]interface{})
		// if !ok {
		// 	c.log.Error("Type assertion to string failed")
		// 	return nil, nil, nil, fmt.Errorf("Type assertion to string failed")
		// }
		// var newtokenhashes []string
		// for i, newTokenHash := range newtokenhashresult {
		// 	statehash, ok := newTokenHash.(string)
		// 	if !ok {
		// 		c.log.Error("Type assertion to string failed at index", i)
		// 		return nil, nil, nil, fmt.Errorf("Type assertion to string failed at index", i)
		// 	}
		// 	newtokenhashes = append(newtokenhashes, statehash)
		// }

		// //trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		// pledgeFinalityError := c.quorumPledgeFinality(cr, nb, newtokenhashes, tid)
		// if pledgeFinalityError != nil {
		// 	c.log.Error("Pledge finlaity not achieved", "err", pledgeFinalityError)
		// 	return nil, nil, nil, pledgeFinalityError
		// }

		//Checking prev block details (i.e. the latest block before transferring) by sender. Sender will connect with old quorums, and update about the exhausted token state hashes to quorums for them to unpledge their tokens.
		for _, tokeninfo := range ti {
			b := c.w.GetLatestTokenBlock(tokeninfo.Token, tokeninfo.TokenType)

			blockHeight, err := b.GetBlockNumber(tokeninfo.Token)
			if err != nil {
				c.log.Error("failed to get latest block height of token ", tokeninfo.Token)
			}

			// if latest block is genesis block of a whole token, then the signer(s) is(are) advisory node(s), not quorum(s)
			// this is the case of all migrated RBTs
			if blockHeight == 0 && tokeninfo.TokenValue == 1.0 {
				continue
			}
			previousQuorumDIDs, err := b.GetSigner()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", tokeninfo.Token, err)
			}

			//if signer is similar to sender did skip this token, as the block is the genesis block
			if previousQuorumDIDs[0] == sc.GetSenderDID() {
				continue
			}

			//concat tokenId and BlockID
			bid, errBlockID := b.GetBlockID(tokeninfo.Token)
			if errBlockID != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch current block id for Token %v, err: %v", tokeninfo.Token, err)
			}
			prevtokenIDTokenStateData := tokeninfo.Token + bid
			prevtokenIDTokenStateBuffer := bytes.NewBuffer([]byte(prevtokenIDTokenStateData))

			//add to ipfs get only the hash of the token+tokenstate. This is the hash just before transferring i.e. the exhausted token state hash, and updating in Sender side
			prevtokenIDTokenStateHash, errIpfsAdd := IpfsAddWithBackoff(c.ipfs, prevtokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if errIpfsAdd != nil {
				return nil, nil, nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", tokeninfo.Token, errIpfsAdd)
			}
			//send this exhausted hash to old quorums to unpledge
			for _, previousQuorumDID := range previousQuorumDIDs {
				// fetch previous quorum's peer Id
				previousQuorumInfo, err := c.GetPeerDIDInfo(previousQuorumDID)
				if previousQuorumInfo.PeerID == "" || err != nil {
					return nil, nil, nil, fmt.Errorf("unable to get peerID for signer DID: %v. It is likely that either the DID is not created anywhere or ", previousQuorumDID)
				}

				previousQuorumAddress := previousQuorumInfo.PeerID + "." + previousQuorumDID
				previousQuorumPeer, errGetPeer := c.getPeer(previousQuorumAddress)
				if errGetPeer != nil {
					return nil, nil, nil, fmt.Errorf("unable to retrieve peer information for %v, err: %v", previousQuorumInfo.PeerID, errGetPeer)
				}

				updateTokenHashDetailsQuery := make(map[string]string)
				updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = prevtokenIDTokenStateHash
				previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, updateTokenHashDetailsQuery, nil, nil, true)
				previousQuorumPeer.Close()
			}
		}

		err = c.w.TokensTransferred(sc.GetSenderDID(), ti, nb, rp.IsLocal(), sr.PinningServiceMode)
		if err != nil {
			c.log.Error("Failed to transfer tokens", "err", err)
			return nil, nil, nil, err
		}
		for _, t := range ti {
			c.w.UnPin(t.Token, wallet.PrevSenderRole, sc.GetSenderDID())
		}
		//call ipfs repo gc after unpinnning
		c.ipfsRepoGc()
		nbid, err := nb.GetBlockID(ti[0].Token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return nil, nil, nil, err
		}

		td := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         nbid,
			Mode:            wallet.SendMode,
			SenderDID:       sc.GetSenderDID(),
			ReceiverDID:     sc.GetReceiverDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, td.TransactionID, td.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &td, pl, pds, nil
	case FTTransferMode:
		// #### FT Tranfer starts here
		// Connect to the receiver's peer
		rp, err := c.getPeer(cr.ReceiverPeerID + "." + sc.GetReceiverDID())
		if err != nil {
			c.log.Error("Receiver not connected", "err", err)
			return nil, nil, nil, err
		}
		defer rp.Close()

		var txnTokenHashes []string = make([]string, 0)
		for _, info := range ti {
			t := info.Token
			blockId, _ := nb.GetBlockID(t)
			tokenIDTokenStateData := t + blockId
			tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
			tokenIDTokenStateHash, _ := c.ipfs.Add(tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			txnTokenHashes = append(txnTokenHashes, tokenIDTokenStateHash)
		}
		//trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, txnTokenHashes, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		// Prepare the send request with the necessary information
		sr := SendFTRequest{
			Address:          cr.SenderPeerID + "." + sc.GetSenderDID(),
			TokenInfo:        ti,
			TokenChainBlock:  nb.GetBlock(),
			QuorumList:       cr.QuorumList,
			TransactionEpoch: cr.TransactionEpoch,
			FTInfo:           cr.FTinfo,
		}

		// Populate quorum details for each quorum in the QuorumList to send to receiver
		for _, qrm := range sr.QuorumList {
			qpid, qdid, ok := util.ParseAddress(qrm)
			if !ok {
				c.log.Error("could not parse quorum address:", qrm)
			}

			var qrmInfo QuorumDIDPeerMap
			//fetch did info of the quorum
			qDidInfo, err := c.GetPeerDIDInfo(qdid)
			if err != nil {
				if strings.Contains(err.Error(), "retry") {
					c.AddPeerDetails(*qDidInfo)
				}
			}
			if qDidInfo == nil || *qDidInfo.DIDType == -1 {
				c.log.Error("could not fetch did type of quorum", qdid, "err", err)
				qrmInfo.DIDType = nil
			} else {
				qrmInfo.DIDType = qDidInfo.DIDType
			}
			if qpid == "" {
				qpid = qDidInfo.PeerID
			}
			qrmInfo.DID = qdid
			qrmInfo.PeerID = qpid
			//add quorum details to the data to be shared
			sr.QuorumInfo = append(sr.QuorumInfo, qrmInfo)
		}

		// Send the FT transfer request to the receiver
		var br model.BasicResponse
		err = rp.SendJSONRequest("POST", APISendFTToken, nil, &sr, &br, true)
		if err != nil {
			c.log.Error("Unable to send tokens to receiver", "err", err)
			return nil, nil, nil, err
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
				return nil, nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("issue type in int is ", issueTypeInt)

			//loop through each items in br.message and update for each tkn

			syncIssueTokenDetails, err2 := c.w.ReadFTToken(token)
			if err2 != nil {
				errMsg := fmt.Sprintf("Consensus failed due to tokenchain sync issue, err %v", err2)
				c.log.Error(errMsg)
				return nil, nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("sync issue token details ", syncIssueTokenDetails)

			if issueTypeInt == TokenChainNotSynced {
				syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
				c.log.Debug("Token sync issue details updated:", syncIssueTokenDetails)
				c.w.UpdateFTToken(syncIssueTokenDetails)
				return nil, nil, nil, errors.New(br.Message)
			}
		}
		if !br.Status {
			c.log.Error("Unable to send FT tokens to receiver", "msg", br.Message)
			return nil, nil, nil, fmt.Errorf("unable to send FT tokens to receiver, " + br.Message)
		}

		//TODO: remove the following commented out code once after testing, calling the quorum pledge finality before APISendFT token
		// // Extract new token state hashes from response
		// newTokenHashResult, ok := br.Result.([]interface{})
		// if !ok {
		// 	c.log.Error("Failed to assert type for new token hashes")
		// 	return nil, nil, nil, fmt.Errorf("Type assertion to string failed")
		// }

		// var newTokenHashes []string
		// for i, newTokenHash := range newTokenHashResult {
		// 	stateHash, ok := newTokenHash.(string)
		// 	if !ok {
		// 		c.log.Error("Type assertion to string failed at index", i)
		// 		return nil, nil, nil, fmt.Errorf("Type assertion to string failed at index %d", i)
		// 	}
		// 	newTokenHashes = append(newTokenHashes, stateHash)
		// }

		// //trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		// pledgeFinalityError := c.quorumPledgeFinality(cr, nb, newTokenHashes, tid)
		// if pledgeFinalityError != nil {
		// 	c.log.Error("Pledge finlaity not achieved", "err", err)
		// 	return nil, nil, nil, pledgeFinalityError
		// }

		//Checking prev block details (i.e. the latest block before transferring) by sender. Sender will connect with old quorums, and update about the exhausted token state hashes to quorums for them to unpledge their tokens.
		for _, tokeninfo := range ti {
			b := c.w.GetLatestTokenBlock(tokeninfo.Token, tokeninfo.TokenType)
			previousQuorumDIDs, err := b.GetSigner()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", tokeninfo.Token, err)
			}

			//if signer is similar to sender did skip this token, as the block is the genesis block
			if previousQuorumDIDs[0] == sc.GetSenderDID() {
				continue
			}

			//concat tokenId and BlockID
			bid, errBlockID := b.GetBlockID(tokeninfo.Token)
			if errBlockID != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch current block id for Token %v, err: %v", tokeninfo.Token, err)
			}
			prevtokenIDTokenStateData := tokeninfo.Token + bid
			prevtokenIDTokenStateBuffer := bytes.NewBuffer([]byte(prevtokenIDTokenStateData))

			//add to ipfs get only the hash of the token+tokenstate. This is the hash just before transferring i.e. the exhausted token state hash, and updating in Sender side
			prevtokenIDTokenStateHash, errIpfsAdd := IpfsAddWithBackoff(c.ipfs, prevtokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if errIpfsAdd != nil {
				return nil, nil, nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", tokeninfo.Token, errIpfsAdd)
			}
			//send this exhausted hash to old quorums to unpledge
			for _, previousQuorumDID := range previousQuorumDIDs {
				// fetch previous quorum's peer Id
				previousQuorumInfo, err := c.GetPeerDIDInfo(previousQuorumDID)
				if previousQuorumInfo.PeerID == "" || err != nil {
					return nil, nil, nil, fmt.Errorf("unable to get peerID for signer DID: %v. It is likely that either the DID is not created anywhere or ", previousQuorumDID)
				}

				previousQuorumAddress := previousQuorumInfo.PeerID + "." + previousQuorumDID
				previousQuorumPeer, errGetPeer := c.getPeer(previousQuorumAddress)
				if errGetPeer != nil {
					return nil, nil, nil, fmt.Errorf("unable to retrieve peer information for %v, err: %v", previousQuorumInfo.PeerID, errGetPeer)
				}
				updateTokenHashDetailsQuery := make(map[string]string)
				updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = prevtokenIDTokenStateHash
				previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, updateTokenHashDetailsQuery, nil, nil, true)
				previousQuorumPeer.Close()

			}
		}
		err = c.w.FTTokensTransffered(sc.GetSenderDID(), ti, nb, rp.IsLocal())
		if err != nil {
			c.log.Error("Failed to transfer tokens", "err", err)
			return nil, nil, nil, err
		}
		for _, t := range ti {
			c.w.UnPin(t.Token, wallet.PrevSenderRole, sc.GetSenderDID())
		}
		//call ipfs repo gc after unpinnning
		c.ipfsRepoGc()
		nbid, err := nb.GetBlockID(ti[0].Token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return nil, nil, nil, err
		}

		td := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         nbid,
			Mode:            wallet.FTTransferMode,
			SenderDID:       sc.GetSenderDID(),
			ReceiverDID:     sc.GetReceiverDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, td.TransactionID, td.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &td, pl, pds, nil
	case PinningServiceMode:
		c.log.Debug("Mode = PinningServiceMode ")
		c.log.Debug("Pinning Node PeerId", cr.PinningNodePeerID)
		c.log.Debug("Pinning Service DID", sc.GetPinningServiceDID())
		rp, err := c.getPeer(cr.PinningNodePeerID + "." + sc.GetPinningServiceDID())
		if err != nil {
			c.log.Error("Pinning Node not connected", "err", err)
			return nil, nil, nil, err
		}
		defer rp.Close()
		sr := SendTokenRequest{
			Address:            cr.SenderPeerID + "." + sc.GetSenderDID(),
			TokenInfo:          ti,
			TokenChainBlock:    nb.GetBlock(),
			QuorumList:         cr.QuorumList,
			PinningServiceMode: true,
		}
		//fetching quorums' info from PeerDIDTable to share with the receiver
		for _, qrm := range sr.QuorumList {
			//fetch peer id & did of the quorum
			qpid, qdid, ok := util.ParseAddress(qrm)
			if !ok {
				c.log.Error("could not parse quorum address:", qrm)
			}

			var qrmInfo QuorumDIDPeerMap
			//fetch did info of the quorum
			qDidInfo, err := c.GetPeerDIDInfo(qdid)
			if err != nil {
				if strings.Contains(err.Error(), "retry") {
					c.AddPeerDetails(*qDidInfo)
				}
			}
			if qDidInfo == nil || *qDidInfo.DIDType == -1 {
				c.log.Error("could not fetch did type of quorum", qdid, "err", err)
				qrmInfo.DIDType = nil
			} else {
				qrmInfo.DIDType = qDidInfo.DIDType
			}
			if qpid == "" {
				qpid = qDidInfo.PeerID
			}
			qrmInfo.DID = qdid
			qrmInfo.PeerID = qpid
			//add quorum details to the data to be shared
			sr.QuorumInfo = append(sr.QuorumInfo, qrmInfo)
		}
		var br model.BasicResponse
		err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)
		if err != nil {
			c.log.Error("Unable to send tokens to receiver", "err", err)
			return nil, nil, nil, err
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
				return nil, nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("issue type in int is ", issueTypeInt)
			syncIssueTokenDetails, err2 := c.w.ReadToken(token)
			if err2 != nil {
				errMsg := fmt.Sprintf("Consensus failed due to tokenchain sync issue, err %v", err2)
				c.log.Error(errMsg)
				return nil, nil, nil, fmt.Errorf(errMsg)
			}
			c.log.Debug("sync issue token details ", syncIssueTokenDetails)
			if issueTypeInt == TokenChainNotSynced {
				syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
				c.log.Debug("sync issue token details status updated", syncIssueTokenDetails)
				c.w.UpdateToken(syncIssueTokenDetails)
				return nil, nil, nil, errors.New(br.Message)
			}
		}

		if !br.Status {
			c.log.Error("Unable to send tokens to pinning node", "msg", br.Message)
			return nil, nil, nil, fmt.Errorf("unable to send tokens to pinning node, " + br.Message)
		}

		// br.Result will contain the new token state after sending tokens to receiver as a response to APISendReceiverToken
		newtokenhashresult, ok := br.Result.([]interface{})
		if !ok {
			c.log.Error("Type assertion to string failed")
			return nil, nil, nil, fmt.Errorf("Type assertion to string failed")
		}
		var newtokenhashes []string
		for i, newTokenHash := range newtokenhashresult {
			statehash, ok := newTokenHash.(string)
			if !ok {
				c.log.Error("Type assertion to string failed at index", i)
				return nil, nil, nil, fmt.Errorf("Type assertion to string failed at index", i)
			}
			newtokenhashes = append(newtokenhashes, statehash)
		}

		//trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, newtokenhashes, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		//Checking prev block details (i.e. the latest block before transferring) by sender. Sender will connect with old quorums, and update about the exhausted token state hashes to quorums for them to unpledge their tokens.
		for _, tokeninfo := range ti {
			b := c.w.GetLatestTokenBlock(tokeninfo.Token, tokeninfo.TokenType)
			previousQuorumDIDs, err := b.GetSigner()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", tokeninfo.Token, err)
			}

			//if signer is similar to sender did skip this token, as the block is the genesis block
			if previousQuorumDIDs[0] == sc.GetSenderDID() {
				continue
			}

			//concat tokenId and BlockID
			bid, errBlockID := b.GetBlockID(tokeninfo.Token)
			if errBlockID != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch current block id for Token %v, err: %v", tokeninfo.Token, err)
			}
			prevtokenIDTokenStateData := tokeninfo.Token + bid
			prevtokenIDTokenStateBuffer := bytes.NewBuffer([]byte(prevtokenIDTokenStateData))

			//add to ipfs get only the hash of the token+tokenstate. This is the hash just before transferring i.e. the exhausted token state hash, and updating in Sender side
			prevtokenIDTokenStateHash, errIpfsAdd := IpfsAddWithBackoff(c.ipfs, prevtokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if errIpfsAdd != nil {
				return nil, nil, nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", tokeninfo.Token, errIpfsAdd)
			}
			//send this exhausted hash to old quorums to unpledge
			for _, previousQuorumDID := range previousQuorumDIDs {
				// fetch previous quorum's peer Id
				previousQuorumInfo, err := c.GetPeerDIDInfo(previousQuorumDID)
				if previousQuorumInfo.PeerID == "" || err != nil {
					return nil, nil, nil, fmt.Errorf("unable to get peerID for signer DID: %v. It is likely that either the DID is not created anywhere or ", previousQuorumDID)
				}

				previousQuorumAddress := previousQuorumInfo.PeerID + "." + previousQuorumDID
				previousQuorumPeer, errGetPeer := c.getPeer(previousQuorumAddress)
				if errGetPeer != nil {
					return nil, nil, nil, fmt.Errorf("unable to retrieve peer information for %v, err: %v", previousQuorumInfo.PeerID, errGetPeer)
				}

				updateTokenHashDetailsQuery := make(map[string]string)
				updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = prevtokenIDTokenStateHash
				previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, updateTokenHashDetailsQuery, nil, nil, true)
				previousQuorumPeer.Close()

			}
		}

		err = c.w.TokensTransferred(sc.GetSenderDID(), ti, nb, rp.IsLocal(), sr.PinningServiceMode)
		if err != nil {
			c.log.Error("Failed to transfer tokens", "err", err)
			return nil, nil, nil, err
		}
		//Commented out this unpinning part so that the unpin is not done from the sender side
		// for _, t := range ti {
		// 	c.w.UnPin(t.Token, wallet.PrevSenderRole, sc.GetSenderDID())
		// }
		//call ipfs repo gc after unpinnning
		// c.ipfsRepoGc()
		nbid, err := nb.GetBlockID(ti[0].Token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return nil, nil, nil, err
		}

		td := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         nbid,
			Mode:            wallet.PinningServiceMode,
			SenderDID:       sc.GetSenderDID(),
			ReceiverDID:     sc.GetPinningServiceDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}
		err = c.initiateUnpledgingProcess(cr, td.TransactionID, td.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}
		return &td, pl, pds, nil
	case SelfTransferMode:
		var quorumInfo []QuorumDIDPeerMap = make([]QuorumDIDPeerMap, 0)
		var selfAddress string = cr.ReceiverPeerID + "." + sc.GetReceiverDID()

		//fetching quorums' info from PeerDIDTable to share with the receiver
		for _, qrm := range cr.QuorumList {
			//fetch peer id & did of the quorum
			qpid, qdid, ok := util.ParseAddress(qrm)
			if !ok {
				c.log.Error("could not parse quorum address:", qrm)
			}

			var qrmInfo QuorumDIDPeerMap
			//fetch did info of the quorum
			qDidInfo, err := c.GetPeerDIDInfo(qdid)
			if err != nil {
				if qDidInfo == nil {
					c.log.Error("could not fetch did info of quorum", qdid, "err", err)
					qrmInfo.DIDType = nil
				}
				if strings.Contains(err.Error(), "retry") {
					c.AddPeerDetails(*qDidInfo)
				}
			}
			if *qDidInfo.DIDType == -1 {
				c.log.Error("could not fetch did type of quorum", qdid, "err", err)
				qrmInfo.DIDType = nil
			} else {
				qrmInfo.DIDType = qDidInfo.DIDType
			}
			if qpid == "" {
				qpid = qDidInfo.PeerID
			}
			qrmInfo.DID = qdid
			qrmInfo.PeerID = qpid
			//add quorum details to the data to be shared
			quorumInfo = append(quorumInfo, qrmInfo)
		}

		// Self update for self transfer tokens
		updatedTokenHashes, _, err := c.updateReceiverToken(selfAddress, "", ti, nb.GetBlock(), cr.QuorumList, quorumInfo, cr.TransactionEpoch, false)
		if err != nil {
			errMsg := fmt.Errorf("failed while update of self transfer tokens, err: %v", err)
			c.log.Error(errMsg.Error())
			return nil, nil, nil, errMsg

		}

		//trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, updatedTokenHashes, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		//Checking prev block details (i.e. the latest block before transferring) by sender. Sender will connect with old quorums, and update about the exhausted token state hashes to quorums for them to unpledge their tokens.
		for _, tokeninfo := range ti {
			b := c.w.GetLatestTokenBlock(tokeninfo.Token, tokeninfo.TokenType)
			previousQuorumDIDs, err := b.GetSigner()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", tokeninfo.Token, err)
			}

			//if signer is similar to sender did skip this token, as the block is the genesys block
			if previousQuorumDIDs[0] == sc.GetSenderDID() {
				continue
			}
			// Contrary to general RBT transfer where we can take the latest block ID since the token chain wasn't updated,
			// in case of Self Transfer, the tokechain gets updated after calling updateReceiverToken, hence we have to consider
			// the previous block ID
			bid, errBlockID := b.GetPrevBlockID(tokeninfo.Token)
			if errBlockID != nil {
				return nil, nil, nil, fmt.Errorf("unable to fetch previous block id for Token %v, err: %v", tokeninfo.Token, err)
			}

			prevtokenIDTokenStateData := tokeninfo.Token + bid
			prevtokenIDTokenStateBuffer := bytes.NewBuffer([]byte(prevtokenIDTokenStateData))

			//add to ipfs get only the hash of the token+tokenstate. This is the hash just before transferring i.e. the exhausted token state hash, and updating in Sender side
			prevtokenIDTokenStateHash, errIpfsAdd := IpfsAddWithBackoff(c.ipfs, prevtokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if errIpfsAdd != nil {
				return nil, nil, nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", tokeninfo.Token, errIpfsAdd)
			}

			//send this exhausted hash to old quorums to unpledge
			for _, previousQuorumDID := range previousQuorumDIDs {
				// fetch previous quorum's peer Id
				previousQuorumInfo, err := c.GetPeerDIDInfo(previousQuorumDID)
				if previousQuorumInfo.PeerID == "" || err != nil {
					return nil, nil, nil, fmt.Errorf("unable to get peerID for signer DID: %v. It is likely that either the DID is not created anywhere or ", previousQuorumDID)
				}

				previousQuorumAddress := previousQuorumInfo.PeerID + "." + previousQuorumDID
				previousQuorumPeer, errGetPeer := c.getPeer(previousQuorumAddress)
				if errGetPeer != nil {
					return nil, nil, nil, fmt.Errorf("unable to retrieve peer information for %v, err: %v", previousQuorumInfo.PeerID, errGetPeer)
				}

				updateTokenHashDetailsQuery := make(map[string]string)
				updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = prevtokenIDTokenStateHash
				previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, updateTokenHashDetailsQuery, nil, nil, true)
				previousQuorumPeer.Close()

			}
		}

		nbid, err := nb.GetBlockID(ti[0].Token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return nil, nil, nil, err
		}

		td := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         nbid,
			Mode:            wallet.SendMode,
			SenderDID:       sc.GetSenderDID(),
			ReceiverDID:     sc.GetReceiverDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, td.TransactionID, td.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &td, pl, pds, nil
	case DTCommitMode:
		err = c.w.CreateTokenBlock(nb)
		if err != nil {
			c.log.Error("Failed to create token block", "err", err)
			return nil, nil, nil, err
		}
		td := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			DateTime:        time.Now(),
			Status:          true,
		}

		//trigger pledge finality to the quorum
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, nil, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}
		return &td, pl, pds, nil
	case SmartContractDeployMode:
		//Create tokechain for the smart contract token and add genesys block
		err = c.w.AddTokenBlock(cr.SmartContractToken, nb)
		if err != nil {
			c.log.Error("smart contract token chain creation failed", "err", err)
			return nil, nil, nil, err
		}
		//update smart contracttoken status to deployed in DB
		err = c.w.UpdateSmartContractStatus(cr.SmartContractToken, wallet.TokenIsDeployed)
		if err != nil {
			c.log.Error("Failed to update smart contract Token deploy detail in storage", err)
			return nil, nil, nil, err
		}
		c.log.Debug("creating commited token block")
		//create new committed block to be updated to the commited RBT tokens
		err = c.createCommitedTokensBlock(nb, cr.SmartContractToken, dc)
		if err != nil {
			c.log.Error("Failed to create commited RBT tokens block ", "err", err)
			return nil, nil, nil, err
		}
		//update committed RBT token with the new block also and lock the RBT
		//and change token status to commited, to prevent being used for txn or pledging
		commitedRbtTokens, err := nb.GetCommitedTokenDetials(cr.SmartContractToken)
		if err != nil {
			c.log.Error("Failed to fetch commited rbt tokens", "err", err)
			return nil, nil, nil, err
		}
		err = c.w.CommitTokens(sc.GetDeployerDID(), commitedRbtTokens)
		if err != nil {
			c.log.Error("Failed to update commited RBT tokens in DB ", "err", err)
			return nil, nil, nil, err
		}

		newBlockId, err := nb.GetBlockID(cr.SmartContractToken)
		if err != nil {
			c.log.Error("failed to get new block id ", "err", err)
			return nil, nil, nil, err
		}

		//Latest Smart contract token hash after being deployed.
		scTokenStateData := cr.SmartContractToken + newBlockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(scTokenStateData))
		newtokenIDTokenStateHash, err := IpfsAddWithBackoff(c.ipfs, tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		c.log.Info(fmt.Sprintf("New smart contract token hash after being deployed : %s", newtokenIDTokenStateHash))

		//trigger pledge finality to the quorum and adding the details in token hash table
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, []string{newtokenIDTokenStateHash}, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
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

		txnDetails := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         newBlockId,
			Mode:            wallet.DeployMode,
			DeployerDID:     sc.GetDeployerDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, txnDetails.TransactionID, txnDetails.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &txnDetails, pl, pds, nil
	case SmartContractExecuteMode:
		//Get the latest block details before being executed to get the old signers
		b := c.w.GetLatestTokenBlock(cr.SmartContractToken, nb.GetTokenType(cr.SmartContractToken))

		previousQuorumDIDs, err := b.GetSigner()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", cr.SmartContractToken, err)
		}

		//Create tokechain for the smart contract token and add genesys block
		err = c.w.AddTokenBlock(cr.SmartContractToken, nb)
		if err != nil {
			c.log.Error("smart contract token chain creation failed", "err", err)
			return nil, nil, nil, err
		}
		//update smart contracttoken status to deployed in DB
		err = c.w.UpdateSmartContractStatus(cr.SmartContractToken, wallet.TokenIsExecuted)
		if err != nil {
			c.log.Error("Failed to update smart contract Token execute detail in storage", err)
			return nil, nil, nil, err
		}

		newBlockId, err := nb.GetBlockID(cr.SmartContractToken)
		if err != nil {
			c.log.Error("failed to get new block id ", "err", err)
			return nil, nil, nil, err
		}

		//Latest Smart contract token hash after being executed.
		scTokenStateData := cr.SmartContractToken + newBlockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(scTokenStateData))
		newtokenIDTokenStateHash, err := IpfsAddWithBackoff(c.ipfs, tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		c.log.Info(fmt.Sprintf("New smart contract token hash after being executed : %s", newtokenIDTokenStateHash))

		//trigger pledge finality to the quorum and adding the details in token hash table
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, []string{newtokenIDTokenStateHash}, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		//Todo pubsub - publish smart contract token details
		newEvent := model.NewContractEvent{
			SmartContractToken:     cr.SmartContractToken,
			Did:                    sc.GetExecutorDID(),
			Type:                   ExecuteType,
			SmartContractBlockHash: newBlockId,
			SmartContractData:      sc.GetSmartContractData(),
		}

		err = c.publishNewEvent(&newEvent)
		if err != nil {
			c.log.Error("Failed to publish smart contract Executed info")
		}

		//inform old quorums about exhausted smart contract token hash
		prevBlockId, errBlockID := nb.GetPrevBlockID((cr.SmartContractToken))
		if errBlockID != nil {
			return nil, nil, nil, fmt.Errorf("unable to fetch previous block id for Token %v, err: %v", cr.SmartContractToken, err)
		}

		scTokenStateDataOld := cr.SmartContractToken + prevBlockId
		scTokenStateDataOldBuffer := bytes.NewBuffer([]byte(scTokenStateDataOld))
		oldsctokenIDTokenStateHash, errIpfsAdd := IpfsAddWithBackoff(c.ipfs, scTokenStateDataOldBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if errIpfsAdd != nil {
			return nil, nil, nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", cr.SmartContractToken, errIpfsAdd)
		}

		for _, previousQuorumDID := range previousQuorumDIDs {
			// fetch previous quorum's peer Id
			previousQuorumInfo, err := c.GetPeerDIDInfo(previousQuorumDID)
			if previousQuorumInfo.PeerID == "" || err != nil {
				return nil, nil, nil, fmt.Errorf("unable to get peerID for signer DID: %v. It is likely that either the DID is not created anywhere or ", previousQuorumDID)
			}

			previousQuorumAddress := previousQuorumInfo.PeerID + "." + previousQuorumDID
			previousQuorumPeer, errGetPeer := c.getPeer(previousQuorumAddress)
			if errGetPeer != nil {
				return nil, nil, nil, fmt.Errorf("unable to retrieve peer information for %v, err: %v", previousQuorumInfo.PeerID, errGetPeer)
			}

			updateTokenHashDetailsQuery := make(map[string]string)
			updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = oldsctokenIDTokenStateHash
			previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, updateTokenHashDetailsQuery, nil, nil, true)
			previousQuorumPeer.Close()

		}

		txnDetails := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         newBlockId,
			Mode:            wallet.ExecuteMode,
			DeployerDID:     sc.GetExecutorDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, txnDetails.TransactionID, txnDetails.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &txnDetails, pl, pds, nil
	case NFTDeployMode:
		//Create tokechain for the smart contract token and add genesys block
		err = c.w.AddTokenBlock(cr.NFT, nb)
		if err != nil {
			c.log.Error("NFT token chain creation failed", "err", err)
			return nil, nil, nil, err
		}
		newBlockId, err := nb.GetBlockID(cr.NFT)
		if err != nil {
			c.log.Error("failed to get new block id of the NFT ", "err", err)
			return nil, nil, nil, err
		}

		//Latest NFT token hash after being deployed.
		nftStateData := cr.NFT + newBlockId
		nftIDTokenStateBuffer := bytes.NewBuffer([]byte(nftStateData))
		newnftIDTokenStateHash, err := IpfsAddWithBackoff(c.ipfs, nftIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		c.log.Info(fmt.Sprintf("New nft state hash after being deployed : %s", newnftIDTokenStateHash))

		//trigger pledge finality to the quorum and adding the details in token hash table
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, []string{newnftIDTokenStateHash}, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved while deploying nft", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		txnDetails := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         newBlockId,
			Mode:            wallet.DeployMode,
			DeployerDID:     sc.GetDeployerDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, txnDetails.TransactionID, txnDetails.Epoch)
		if err != nil {
			c.log.Error("Failed to store transaction details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &txnDetails, pl, pds, nil
	case NFTExecuteMode:
		//Get the latest block details before being executed to get the old signers
		b := c.w.GetLatestTokenBlock(cr.NFT, nb.GetTokenType(cr.NFT))
		previousQuorumDIDs, err := b.GetSigner()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", cr.NFT, err)
		}

		err = c.w.AddTokenBlock(cr.NFT, nb)
		if err != nil {
			c.log.Error("NFT chain creation failed", "err", err)
			return nil, nil, nil, err
		}
		newBlockId, err := nb.GetBlockID(cr.NFT)
		if err != nil {
			c.log.Error("failed to get new block id ", "err", err)
			return nil, nil, nil, err
		}

		//Latest Smart contract token hash after being executed.
		nftStateData := cr.NFT + newBlockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(nftStateData))
		newtokenIDTokenStateHash, err := IpfsAddWithBackoff(c.ipfs, tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		c.log.Info(fmt.Sprintf("New NFT state hash after being executed : %s", newtokenIDTokenStateHash))

		//trigger pledge finality to the quorum and adding the details in token hash table
		pledgeFinalityError := c.quorumPledgeFinality(cr, nb, []string{newtokenIDTokenStateHash}, tid)
		if pledgeFinalityError != nil {
			c.log.Error("Pledge finlaity not achieved", "err", err)
			return nil, nil, nil, pledgeFinalityError
		}

		prevBlockId, _ := nb.GetPrevBlockID((cr.NFT))
		nftTokenStateDataOld := cr.NFT + prevBlockId
		nftTokenStateDataOldBuffer := bytes.NewBuffer([]byte(nftTokenStateDataOld))
		oldnfttokenIDTokenStateHash, errIpfsAdd := IpfsAddWithBackoff(c.ipfs, nftTokenStateDataOldBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if errIpfsAdd != nil {
			return nil, nil, nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", nftTokenStateDataOldBuffer, errIpfsAdd)
		}

		for _, previousQuorumDID := range previousQuorumDIDs {
			// fetch previous quorum's peer Id
			previousQuorumInfo, err := c.GetPeerDIDInfo(previousQuorumDID)
			if previousQuorumInfo.PeerID == "" || err != nil {
				return nil, nil, nil, fmt.Errorf("unable to get peerID for signer DID: %v. It is likely that either the DID is not created anywhere or ", previousQuorumDID)
			}

			previousQuorumAddress := previousQuorumInfo.PeerID + "." + previousQuorumDID
			previousQuorumPeer, errGetPeer := c.getPeer(previousQuorumAddress)
			if errGetPeer != nil {
				return nil, nil, nil, fmt.Errorf("unable to retrieve peer information for %v, err: %v", previousQuorumInfo.PeerID, errGetPeer)
			}

			updateTokenHashDetailsQuery := make(map[string]string)
			updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = oldnfttokenIDTokenStateHash
			previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, updateTokenHashDetailsQuery, nil, nil, true)
			previousQuorumPeer.Close()

		}

		txnDetails := model.TransactionDetails{
			TransactionID:   tid,
			TransactionType: nb.GetTransType(),
			BlockID:         newBlockId,
			Mode:            wallet.ExecuteMode,
			DeployerDID:     sc.GetExecutorDID(),
			Comment:         sc.GetComment(),
			DateTime:        time.Now(),
			Status:          true,
			Epoch:           int64(cr.TransactionEpoch),
		}

		err = c.initiateUnpledgingProcess(cr, txnDetails.TransactionID, txnDetails.Epoch)
		if err != nil {
			c.log.Error("Failed to store transactiond details with quorum ", "err", err)
			return nil, nil, nil, err
		}

		return &txnDetails, pl, pds, nil

	default:
		err := fmt.Errorf("invalid consensus request mode: %v", cr.Mode)
		c.log.Error(err.Error())
		return nil, nil, nil, err
	}
}

func (c *Core) initiateUnpledgingProcess(cr *ConensusRequest, transactionHash string, transactionEpoch int64) error {
	// Get the information about Pledging Quorum from pledged tokens
	// TODO: Need to refactor after incorporating Distributed Pledging
	c.qlock.Lock()
	pd, ok1 := c.pd[cr.ReqID]
	cs, ok2 := c.quorumRequest[cr.ReqID]
	c.qlock.Unlock()

	if !ok1 || !ok2 {
		c.log.Error("invalid consensus request for quorum transaction detail storage")
		return errors.New("invalid consensus request for quorum transaction detail storage")
	}

	if len(pd.PledgedTokens) == 0 {
		c.log.Error("unable to get pledged tokens")
		return errors.New("unable to get pledged tokens")
	}

	for did, pledgeTokenHashes := range pd.PledgedTokens {
		p, ok := cs.P[did]
		if !ok {
			c.log.Error("unable to get the peer detail")
			return fmt.Errorf("unable to get the peer detail")
		}
		if p == nil {
			c.log.Error("peer object is returned as nil")
			return fmt.Errorf("peer object is returned as nil")
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
			c.log.Error("Quorum not connected (storing tx info)", "err", err)
			return err
		}
		defer qPeer.Close()

		var br model.BasicResponse
		initiateUnpledgeRequest := &model.AddUnpledgeDetailsRequest{
			TransactionHash:   transactionHash,
			QuorumDID:         qPeer.GetPeerDID(),
			PledgeTokenHashes: pledgeTokenHashes,
			TransactionEpoch:  transactionEpoch,
		}

		err = qPeer.SendJSONRequest("POST", APIAddUnpledgeDetails, nil, initiateUnpledgeRequest, &br, true)
		if err != nil {
			c.log.Error(err.Error())
			return err
		}
		if !br.Status {
			c.log.Error(br.Message)
			return fmt.Errorf(br.Message)
		}
	}

	return nil
}

func (c *Core) quorumPledgeFinality(cr *ConensusRequest, newBlock *block.Block, newTokenStateHashes []string, transactionId string) error {
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
			Mode:                        cr.Mode,
			PledgedTokens:               v,
			TokenChainBlock:             newBlock.GetBlock(),
			TransactionID:               transactionId,
			TransferredTokenStateHashes: nil,
			TransactionEpoch:            cr.TransactionEpoch,
		}

		if newTokenStateHashes != nil {
			// ur.TransferredTokenStateHashes = newTokenStateHashes[countofTokenStateHash : countofTokenStateHash+len(v)]
			ur.TransferredTokenStateHashes = newTokenStateHashes
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
	pd, ok := c.pd[id] //getting details of quorums who pledged
	if !ok {
		if p != nil {
			p.Close()
		}
		return
	}
	var signType string
	//signType = 0 => Pki based sign in lite mode
	//signType = 1 => Nlss based sign in basic mode
	if util.HexToStr(ss) == "" {
		signType = "0"
	} else {
		signType = "1"
	}

	switch qt {
	case 0:
		cs.Result.RunningCount--
		if status {
			did := p.GetPeerDID()
			csig := CreditSignature{
				Signature:     util.HexToStr(ss),
				PrivSignature: util.HexToStr(ps),
				DID:           did,
				Hash:          hash,
				SignType:      signType,
			}
			if cs.Result.SuccessCount < MinConsensusRequired {
				if _, ok := pd.PledgedTokens[did]; ok {
					cs.P[did] = p
					cs.Credit.Credit = append(cs.Credit.Credit, csig)
					cs.Result.SuccessCount++
				}
			}
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
	c.startConsensus(cr.ReqID, qt)
	var p *ipfsport.Peer
	var err error
	p, err = c.getPeer(addr)
	if err != nil {
		c.log.Error(fmt.Sprintf("Failed to get peer connection while connecting to quorum address %v, err: %v", addr, err))
		c.finishConsensus(cr.ReqID, qt, nil, false, "", nil, nil)
		return
	}
	err = c.initPledgeQuorumToken(cr, p, qt)
	if err != nil {
		if strings.Contains(err.Error(), "don't have enough balance to pledge") {
			c.log.Error("Quorum failed to pledge token")
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Error("Failed to pledge token", "err", err)
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}
	var cresp ConensusReply
	err = p.SendJSONRequest("POST", APIQuorumConsensus, nil, cr, &cresp, true, 60*time.Minute)
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
		//tokenPrefix := "Token: "
		issueTypePrefix := "issueType: "

		// Find the starting indexes of pt and issueType values
		//ptStart := strings.Index(cresp.Message, tokenPrefix) + len(tokenPrefix)
		issueTypeStart := strings.Index(cresp.Message, issueTypePrefix) + len(issueTypePrefix)

		// Extracting the substrings from the message
		//	token := cresp.Message[ptStart : strings.Index(cresp.Message[ptStart:], ",")+ptStart]
		issueType := cresp.Message[issueTypeStart:]

		c.log.Debug("In connectQuorum, token is ", cresp.Result, "||| issuetype is ", issueType)
		issueTypeInt, err1 := strconv.Atoi(issueType)
		if err1 != nil {
			c.log.Error("Consensus failed due to token chain sync issue, issueType string conversion", "err", err1)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Debug("issue type in int is ", issueTypeInt)
		for _, token := range cresp.Result {
			if cr.Mode == FTTransferMode {
				FTsyncIssueTokenDetails, err2 := c.w.ReadFTToken(token)
				if err2 != nil {
					c.log.Error("Consensus failed due to tokenchain sync issue ", "err", err2)
					//	c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
					//	return
				}
				c.log.Debug("In connectQuorum, sync issue token details ", FTsyncIssueTokenDetails)
				if issueTypeInt == TokenChainNotSynced {
					FTsyncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
					c.log.Debug("In connectQuorum, sync issue token details status updated", FTsyncIssueTokenDetails)
					c.w.UpdateFTToken(FTsyncIssueTokenDetails)
				}
			} else if cr.Mode == RBTTransferMode {
				syncIssueTokenDetails, err2 := c.w.ReadToken(token)
				if err2 != nil {
					c.log.Error("Consensus failed due to tokenchain sync issue, token not found in tab ", "err", err2)
					//	c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
					//	return
				}
				c.log.Debug("In connectQuorum, sync issue token details ", syncIssueTokenDetails)
				if issueTypeInt == TokenChainNotSynced {
					syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
					c.log.Debug("In connectQuorum, sync issue token details status updated", syncIssueTokenDetails)
					c.w.UpdateToken(syncIssueTokenDetails)
					//c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
					//return
				}
			}
		}
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}

	if strings.Contains(cresp.Message, "Token state is exhausted, Token is being Double spent.") {
		tokenPrefix := "Token : "
		tStart := strings.Index(cresp.Message, tokenPrefix) + len(tokenPrefix)
		var token string
		if tStart >= len(tokenPrefix) {
			token = cresp.Message[tStart:]
			c.log.Debug("Token is being Double spent. Token is ", token)
		}
		if cr.Mode == FTTransferMode {
			doubleSpendFTDetails, err2 := c.w.ReadFTToken(token)
			if err2 != nil {
				c.log.Error("Consensus failed due to token being double spent ", "err", err2)
				c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
				return
			}
			c.log.Debug("Double spend token details ", doubleSpendFTDetails)
			doubleSpendFTDetails.TokenStatus = wallet.TokenIsBeingDoubleSpent
			c.log.Debug("Double spend token details status updated", doubleSpendFTDetails)
			c.w.UpdateFTToken(doubleSpendFTDetails)
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
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
		c.log.Error("Failed to get consensus", "msg", cresp.Message, "|| crep is ", cresp)
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
	credit := make([]block.CreditSignature, 0)
	for _, csig := range cs.Credit.Credit {
		credit_ := block.CreditSignature{
			Signature:     csig.Signature,
			PrivSignature: csig.PrivSignature,
			DID:           csig.DID,
			Hash:          csig.Hash,
			SignType:      csig.SignType,
		}
		credit = append(credit, credit_)
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

		//Fetching deployer signature to add it to transaction details
		signData, deployerNLSSShare, deployerPrivSign, err := sc.GetHashSig(bti.DeployerDID)
		if err != nil {
			c.log.Error("failed to fetch deployer sign", "err", err)
			return nil, fmt.Errorf("failed to fetch deployer sign")
		}
		deployerSignType := dc.GetSignType()
		deployerSign := &block.InitiatorSignature{
			NLSSShare:   deployerNLSSShare,
			PrivateSign: deployerPrivSign,
			DID:         bti.DeployerDID,
			Hash:        signData,
			SignType:    deployerSignType,
		}

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
			TransactionType:    block.TokenDeployedType,
			TokenOwner:         sc.GetDeployerDID(),
			TransInfo:          bti,
			QuorumSignature:    credit,
			SmartContract:      sc.GetBlock(),
			GenesisBlock:       smartContractGensisBlock,
			PledgeDetails:      ptds,
			InitiatorSignature: deployerSign,
			Epoch:              cr.TransactionEpoch,
		}
	} else if cr.Mode == SmartContractExecuteMode {
		bti.ExecutorDID = sc.GetExecutorDID()

		//Fetching executor signature to add it to transaction details
		signData, executorNLSSShare, executorPrivSign, err := sc.GetHashSig(bti.ExecutorDID)
		if err != nil {
			c.log.Error("failed to fetch executor sign", "err", err)
			return nil, fmt.Errorf("failed to fetch executor sign")
		}
		executorSignType := dc.GetSignType()
		executorSign := &block.InitiatorSignature{
			NLSSShare:   executorNLSSShare,
			PrivateSign: executorPrivSign,
			DID:         bti.ExecutorDID,
			Hash:        signData,
			SignType:    executorSignType,
		}

		tcb = block.TokenChainBlock{
			TransactionType:    block.TokenExecutedType,
			TokenOwner:         sc.GetExecutorDID(),
			TransInfo:          bti,
			QuorumSignature:    credit,
			SmartContract:      sc.GetBlock(),
			PledgeDetails:      ptds,
			SmartContractData:  sc.GetSmartContractData(),
			InitiatorSignature: executorSign,
			Epoch:              cr.TransactionEpoch,
		}

	} else if cr.Mode == NFTExecuteMode {
		bti.ExecutorDID = sc.GetExecutorDID()

		//Fetching executor signature to add it to transaction details
		signData, executorNLSSsign, executorPrivSign, err := sc.GetHashSig(bti.ExecutorDID)
		if err != nil {
			c.log.Error("failed to fetch executor sign", "err", err)
			return nil, fmt.Errorf("failed to fetch executor sign")
		}
		executorSignType := dc.GetSignType()
		executor_sign := &block.InitiatorSignature{
			NLSSShare:   executorNLSSsign,
			PrivateSign: executorPrivSign,
			DID:         bti.ExecutorDID,
			Hash:        signData,
			SignType:    executorSignType,
		}

		tcb = block.TokenChainBlock{
			TransactionType:    block.TokenExecutedType,
			TokenOwner:         sc.GetReceiverDID(),
			TransInfo:          bti,
			QuorumSignature:    credit,
			NFT:                sc.GetBlock(),
			NFTData:            sc.GetNFTData(),
			PledgeDetails:      ptds,
			TokenValue:         sc.GetTotalRBTs(),
			InitiatorSignature: executor_sign,
			Epoch:              cr.TransactionEpoch,
		}

	} else if cr.Mode == NFTDeployMode {
		bti.DeployerDID = sc.GetDeployerDID()

		//Fetching deployer signature to add it to transaction details
		signData, deployerShareSign, deployerPrivSign, err := sc.GetHashSig(bti.DeployerDID)
		if err != nil {
			c.log.Error("failed to fetch deployer sign", "err", err)
			return nil, fmt.Errorf("failed to fetch deployer sign")
		}
		deployerSignType := dc.GetSignType()
		deployer_sign := &block.InitiatorSignature{
			NLSSShare:   deployerShareSign,
			PrivateSign: deployerPrivSign,
			DID:         bti.DeployerDID,
			Hash:        signData,
			SignType:    deployerSignType,
		}

		nftValue := sc.GetTotalRBTs()

		nftGenesisBlock := &block.GenesisBlock{
			Type: block.TokenGeneratedType,
			Info: []block.GenesisTokenInfo{
				{Token: cr.NFT, NFTValue: nftValue, NFTData: sc.GetNFTData()},
			},
		}
		tcb = block.TokenChainBlock{
			TransactionType:    block.TokenDeployedType,
			TokenOwner:         sc.GetDeployerDID(),
			TransInfo:          bti,
			QuorumSignature:    credit,
			NFT:                sc.GetBlock(),
			NFTData:            sc.GetNFTData(),
			TokenValue:         sc.GetTotalRBTs(),
			GenesisBlock:       nftGenesisBlock,
			PledgeDetails:      ptds,
			InitiatorSignature: deployer_sign,
			Epoch:              cr.TransactionEpoch,
		}

	} else if cr.Mode == PinningServiceMode {
		bti.SenderDID = sc.GetSenderDID()
		bti.PinningNodeDID = sc.GetPinningServiceDID()
		tcb = block.TokenChainBlock{
			TransactionType: block.TokenPinnedAsService,
			TokenOwner:      sc.GetSenderDID(),
			TransInfo:       bti,
			QuorumSignature: credit,
			SmartContract:   sc.GetBlock(),
			PledgeDetails:   ptds,
		}
	} else {
		//Fetching sender signature to add it to transaction details
		senderdid := sc.GetSenderDID()
		signData, senderNLSSShare, senderPrivSign, err := sc.GetHashSig(senderdid)
		if err != nil {
			c.log.Error("failed to fetch sender sign", "err", err)
			return nil, fmt.Errorf("failed to fetch sender sign")
		}
		senderSignType := dc.GetSignType()
		senderSign := &block.InitiatorSignature{
			NLSSShare:   senderNLSSShare,
			PrivateSign: senderPrivSign,
			DID:         senderdid,
			Hash:        signData,
			SignType:    senderSignType,
		}

		bti.SenderDID = sc.GetSenderDID()
		bti.ReceiverDID = sc.GetReceiverDID()
		tcb = block.TokenChainBlock{
			TransactionType:    block.TokenTransferredType,
			TokenOwner:         sc.GetReceiverDID(),
			TransInfo:          bti,
			QuorumSignature:    credit,
			SmartContract:      sc.GetBlock(),
			PledgeDetails:      ptds,
			InitiatorSignature: senderSign,
			Epoch:              cr.TransactionEpoch,
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
	pledgeTokensPerQuorum := pd.TransferAmount / float64(MinQuorumRequired)

	// Request pledge token
	if (c.quorumCount - c.noBalanceQuorumCount) < MinConsensusRequired {
		pr := PledgeRequest{
			TokensRequired: CeilfloatPrecision(pledgeTokensPerQuorum, MaxDecimalPlaces), // Request the determined number of tokens per quorum,
		}
		var prs PledgeReply
		err := p.SendJSONRequest("POST", APIReqPledgeToken, nil, &pr, &prs, true)
		if err != nil {
			c.log.Error("Invalid response for pledge request", "err", err)
			err := fmt.Errorf("invalid pledge request")
			cs.PledgeLock.Unlock()
			return err
		}
		if strings.Contains(prs.Message, "Quorum don't have enough balance to pledge") {
			c.quorumCount++
			c.noBalanceQuorumCount++
			cs.PledgeLock.Unlock()
			did := p.GetPeerDID()
			c.log.Error("Quorum (DID:" + did + ") don't have enough balance to pledge")
			return fmt.Errorf("Quorum (DID:" + did + ") don't have enough balance to pledge")
		}
		if prs.Status {
			c.quorumCount++
			did := p.GetPeerDID()
			pd.PledgedTokens[did] = make([]string, 0)
			for i, t := range prs.Tokens {
				ptcb := block.InitBlock(prs.TokenChainBlock[i], nil)
				if !c.checkIsPledged(ptcb) {
					pd.NumPledgedTokens++
					pd.RemPledgeTokens = pd.RemPledgeTokens - prs.TokenValue[i]
					pd.RemPledgeTokens = floatPrecision(pd.RemPledgeTokens, MaxDecimalPlaces)
					pd.PledgedTokenChainBlock[t] = prs.TokenChainBlock[i]
					pd.PledgedTokens[did] = append(pd.PledgedTokens[did], t)
					pd.TokenList = append(pd.TokenList, Token{TokenHash: prs.Tokens[i], TokenValue: prs.TokenValue[i]})

				}
			}
			c.qlock.Lock()
			c.pd[cr.ReqID] = pd
			c.qlock.Unlock()
		}
	}
	cs.PledgeLock.Unlock()
	count := 0
	for {
		time.Sleep(time.Second)
		count++
		c.qlock.Lock()
		c.qlock.Unlock()
		if !ok {
			err := fmt.Errorf("invalid pledge request")
			return err
		}

		// check if total connected quorums, with balance, has reached the minimum consensus threshold
		if (c.quorumCount - c.noBalanceQuorumCount) < MinConsensusRequired {
			if c.quorumCount < QuorumRequired {
				if count == 300 {
					err := fmt.Errorf("Unable to pledge after wait")
					return err
				}
			} else if c.quorumCount == QuorumRequired {
				err := fmt.Errorf("Unable to pledge")
				return err
			}
		} else {
			return nil
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

func (c *Core) checkLockedTokens(cr *ConensusRequest, quorumList []string) error {
	pd := c.pd[cr.ReqID]

	pledgingQuorumDID := make([]string, 0, len(pd.PledgedTokens))
	for k := range pd.PledgedTokens {
		pledgingQuorumDID = append(pledgingQuorumDID, k)
	}

	var br model.BasicResponse
	for _, pledgingDID := range pledgingQuorumDID {
		for _, addr := range quorumList {
			peerID, did, _ := util.ParseAddress(addr)
			if did == pledgingDID {
				p, err := c.pm.OpenPeerConn(peerID, did, c.getCoreAppName(peerID))
				if err != nil {
					c.log.Error("Failed to get peer connection", "err", err)
					return err
				}
				tokenList := TokenList{
					Tokens: pd.PledgedTokens[pledgingDID],
					DID:    pledgingDID,
				}
				err = p.SendJSONRequest("POST", APIUnlockTokens, nil, &tokenList, &br, true)
				if err != nil {
					c.log.Error("Invalid response for pledge request", "err", err)
					return err
				}
			}
		}
	}
	return nil
}
