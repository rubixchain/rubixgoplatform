package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/util"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

const (
	MinQuorumRequired    int = 5
	MinConsensusRequired int = 5
)
const (
	RBTTransferMode int = iota
	NFTTransferMode
)
const (
	AlphaQuorumType int = iota
	BetaQuorumType
	GammaQuorumType
)

type ConensusRequest struct {
	ReqID           string   `json:"req_id"`
	Type            int      `json:"type"`
	Mode            int      `json:"mode"`
	SenderPeerID    string   `json:"sender_peerd_id"`
	ReceiverPeerID  string   `json:"receiver_peerd_id"`
	SenderDID       string   `json:"sender_did"`
	ReceiverDID     string   `json:"receiver_did"`
	WholeTokens     []string `json:"whole_tokens"`
	WholeTokenChain []string `json:"whole_token_chain"`
	PartTokens      []string `json:"part_tokens"`
	PartTokenChain  []string `json:"part_token_chain"`
	Comment         string   `json:"comment"`
	ShareSig        []byte   `json:"share_sig"`
	PrivSig         []byte   `json:"priv_sig"`
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

type PledgeDetials struct {
	RemPledgeTokens        int
	NumPledgedTokens       int
	PledgedTokens          map[string][]string
	PledgedTokenChainBlock map[string]map[string]interface{}
	ProofPledge            map[string]string
	TokenList              []string
}

type PledgeRequest struct {
	NumTokens int `json:"num_tokens"`
}

type UpdatePledgeRequest struct {
	PledgedTokens   []string               `json:"pledged_tokens"`
	TokenChainBlock map[string]interface{} `json:"token_chain_block"`
}

type SendTokenRequest struct {
	WholeTokens     []string               `json:"whole_tokens"`
	PartTokens      []string               `json:"part_tokens"`
	TokenChainBlock map[string]interface{} `json:"token_chain_block"`
}

type PledgeReply struct {
	model.BasicResponse
	Tokens          []string                 `json:"tokens"`
	TokenChainBlock []map[string]interface{} `json:"token_chain_block"`
	ProofChain      []string                 `json:"proof_chain"`
}

type CreditScore struct {
	Credit []CreditSignature
}

type CreditSignature struct {
	Signature     string `json:"signature"`
	PrivSingature string `json:"priv_signature"`
	DID           string `json:"did"`
	Hash          string `json:"hash"`
}

// PingSetup will setup the ping route
func (c *Core) QuroumSetup() {
	c.l.AddRoute(APICreditStatus, "GET", c.creditStatus)
	c.l.AddRoute(APIQuorumConsensus, "POST", c.quorumConensus)
	c.l.AddRoute(APIQuorumCredit, "POST", c.quorumCredit)
	c.l.AddRoute(APIReqPledgeToken, "POST", c.reqPledgeToken)
	c.l.AddRoute(APIUpdatePledgeToken, "POST", c.updatePledgeToken)
	c.l.AddRoute(APISendReceiverToken, "POST", c.updateReceiverToken)
}

func (c *Core) SetupQuorum(didStr string, pwd string) error {
	_, ok := c.cfg.CfgData.DIDConfig[didStr]
	if !ok {
		c.log.Error("DID does not exist", "did", didStr)
		return fmt.Errorf("DID does not exist")
	}
	dc := did.InitDIDQuorumc(didStr, c.cfg.DirPath+"/Rubix", pwd)
	c.qc[didStr] = dc
	return nil
}

func (c *Core) GetAllQuorum() *model.QuorumList {
	var ql model.QuorumList
	for _, a := range c.cfg.CfgData.QuorumList.Alpha {
		if ql.Quorum == nil {
			ql.Quorum = make([]model.Quorum, 0)
		}
		q := model.Quorum{
			Type:    model.AlphaType,
			Address: a,
		}
		ql.Quorum = append(ql.Quorum, q)
	}
	for _, a := range c.cfg.CfgData.QuorumList.Beta {
		if ql.Quorum == nil {
			ql.Quorum = make([]model.Quorum, 0)
		}
		q := model.Quorum{
			Type:    model.BetaType,
			Address: a,
		}
		ql.Quorum = append(ql.Quorum, q)
	}
	for _, a := range c.cfg.CfgData.QuorumList.Gamma {
		if ql.Quorum == nil {
			ql.Quorum = make([]model.Quorum, 0)
		}
		q := model.Quorum{
			Type:    model.GammaType,
			Address: a,
		}
		ql.Quorum = append(ql.Quorum, q)
	}
	return &ql
}

func (c *Core) AddQuorum(ql *model.QuorumList) error {
	update := false
	for _, q := range ql.Quorum {
		switch q.Type {
		case model.AlphaType:
			update = true
			if c.cfg.CfgData.QuorumList.Alpha == nil {
				c.cfg.CfgData.QuorumList.Alpha = make([]string, 0)
			}
			c.cfg.CfgData.QuorumList.Alpha = append(c.cfg.CfgData.QuorumList.Alpha, q.Address)
		case model.BetaType:
			update = true
			if c.cfg.CfgData.QuorumList.Beta == nil {
				c.cfg.CfgData.QuorumList.Beta = make([]string, 0)
			}
			c.cfg.CfgData.QuorumList.Beta = append(c.cfg.CfgData.QuorumList.Beta, q.Address)
		case model.GammaType:
			update = true
			if c.cfg.CfgData.QuorumList.Gamma == nil {
				c.cfg.CfgData.QuorumList.Gamma = make([]string, 0)
			}
			c.cfg.CfgData.QuorumList.Gamma = append(c.cfg.CfgData.QuorumList.Gamma, q.Address)
		}
	}
	if update {
		return c.updateConfig()
	} else {
		return nil
	}
}

func (c *Core) RemoveAllQuorum() error {
	c.cfg.CfgData.QuorumList.Alpha = nil
	c.cfg.CfgData.QuorumList.Beta = nil
	c.cfg.CfgData.QuorumList.Gamma = nil
	return c.updateConfig()
}

func (c *Core) getQuorumList(t int) *config.QuorumList {
	var ql config.QuorumList
	switch t {
	case 1:
		return nil
		// TODO :: Add method to get type1 quorurm
	case 2:
		ql = c.cfg.CfgData.QuorumList
	}

	if len(ql.Alpha) < MinQuorumRequired {
		c.log.Error("Failed to get required quorur", "al", len(ql.Alpha))
		return nil
	}
	return &ql
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

func (c *Core) initiateConsensus(cr *ConensusRequest, dc did.DIDCrypto) error {
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
	// TODO:: Need to correct for part tokens
	reqPledgeTokens := len(cr.WholeTokens)
	pd := PledgeDetials{
		RemPledgeTokens:        reqPledgeTokens,
		NumPledgedTokens:       0,
		PledgedTokens:          make(map[string][]string),
		PledgedTokenChainBlock: make(map[string]map[string]interface{}),
		ProofPledge:            make(map[string]string),
		TokenList:              make([]string, 0),
	}
	ql := c.getQuorumList(cr.Type)
	if ql == nil {
		c.log.Error("Failed to get required quorums")
		return fmt.Errorf("Failed to get required quorums")
	}
	c.qlock.Lock()
	c.quorumRequest[cr.ReqID] = &cs
	c.pd[cr.ReqID] = &pd
	c.qlock.Unlock()

	for _, a := range ql.Alpha {
		go c.connectQuorum(cr, a, AlphaQuorumType)
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
				c.log.Debug("Conensus finished successfully")
			} else if cs.Result.RunningCount == 0 {
				loop = false
				err = fmt.Errorf("consensus failed")
				c.log.Error("Conensus failed")
			}
		}
		c.qlock.Unlock()
		if !loop {
			break
		}
	}
	if err == nil {
		authHash := util.CalculateHashString(util.ConvertToJson(cr.WholeTokens)+util.ConvertToJson(cr.WholeTokenChain)+util.ConvertToJson(cr.PartTokens)+util.ConvertToJson(cr.PartTokenChain)+cr.ReceiverDID+cr.SenderDID+cr.Comment, "SHA3-256")
		tid := util.CalculateHashString(authHash, "SHA3-256")
		ntcb, err := c.pledgeQuorumToken(cr, tid, dc)
		if err != nil {
			c.log.Error("Failed to pledge token", "err", err)
			return err
		}
		c.sendQuorumCredit(cr)
		rp, err := c.getPeer(cr.ReceiverPeerID + "." + cr.ReceiverDID)
		if err != nil {
			c.log.Error("Receiver not connected", "err", err)
			return err
		}
		sr := SendTokenRequest{
			WholeTokens:     cr.WholeTokens,
			PartTokens:      cr.PartTokens,
			TokenChainBlock: ntcb,
		}
		var br model.BasicResponse
		err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)
		if err != nil {
			c.log.Error("Unable to send tokens to receiver", "err", err)
			return err
		}
		err = c.w.TokensTransferred(cr.SenderDID, cr.WholeTokens, cr.PartTokens, ntcb)
		if err != nil {
			c.log.Error("Failed to transfer tokens", "err", err)
			return err
		}
		for _, t := range cr.WholeTokens {
			c.ipfs.Unpin(t)
		}
		for _, t := range cr.PartTokens {
			c.ipfs.Unpin(t)
		}
	}
	return err
}

func (c *Core) startConensus(id string, qt int) {
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

func (c *Core) finishConensus(id string, qt int, p *ipfsport.Peer, status bool, hash string, ss []byte, ps []byte) {
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
					PrivSingature: util.HexToStr(ps),
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

func (c *Core) validateTokenOwnership(cr *ConensusRequest) bool {
	// ::TODO:: Need to implement
	for i := range cr.WholeTokens {
		c.log.Debug("Finding dht", "token", cr.WholeTokens[i])
		ids, err := c.GetDHTddrs(cr.WholeTokens[i])
		if err != nil || len(ids) == 0 {
			continue
		}
	}
	// for i := range cr.WholeTokens {
	// 	c.log.Debug("Requesting Token status")
	// 	ts := TokenPublish{
	// 		Token: cr.WholeTokens[i],
	// 	}
	// 	c.ps.Publish(TokenStatusTopic, &ts)
	// 	c.log.Debug("Finding dht", "token", cr.WholeTokens[i])
	// 	ids, err := c.GetDHTddrs(cr.WholeTokens[i])
	// 	if err != nil || len(ids) == 0 {
	// 		c.log.Error("Failed to find token owner", "err", err)
	// 		crep.Message = "Failed to find token owner"
	// 		return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 	}
	// 	if len(ids) > 1 {
	// 		// ::TODO:: to more check to findout right pwner
	// 		c.log.Error("Mutiple owner found for the token", "token", cr.WholeTokens, "owners", ids)
	// 		crep.Message = "Mutiple owner found for the token"
	// 		return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 	} else {
	// 		//:TODO:: get peer from the table
	// 		if cr.SenderPeerID != ids[0] {
	// 			c.log.Error("Token peer id mismatched", "expPeerdID", cr.SenderPeerID, "peerID", ids[0])
	// 			crep.Message = "Token peer id mismatched"
	// 			return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 		}
	// 	}
	// }
	return true
}

func (c *Core) checkQuroumCredits(p *ipfsport.Peer, cr *ConensusRequest, qt int) error {
	var cs model.CreditStatus
	err := p.SendJSONRequest("GET", APICreditStatus, nil, nil, &cs, true)
	if err != nil {
		return err
	}
	// TODO::Check credit score & act
	return nil
}

func (c *Core) connectQuorum(cr *ConensusRequest, addr string, qt int) {
	c.startConensus(cr.ReqID, qt)
	p, err := c.getPeer(addr)
	if err != nil {
		c.log.Error("Failed to get peer connection", "err", err)
		c.finishConensus(cr.ReqID, qt, nil, false, "", nil, nil)
		return
	}
	//defer p.Close()
	err = c.initPledgeQuorumToken(cr, p, qt)
	if err != nil {
		c.log.Error("Failed to pleadge token", "err", err)
		c.finishConensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}

	// err = c.checkQuroumCredits(p, cr, qt)
	// if err != nil {
	// 	c.log.Error("Faile to check credit status", "err", err)
	// 	c.finishConensus(cr.ReqID, qt, p, addr, false, "", nil, nil)
	// 	return
	// }
	var cresp ConensusReply
	err = p.SendJSONRequest("POST", APIQuorumConsensus, nil, cr, &cresp, true, 10*time.Minute)
	if err != nil {
		c.log.Error("Faile to get consensus", "err", err)
		c.finishConensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}
	if !cresp.Status {
		c.log.Error("Faile to get consensus", "msg", cresp.Message)
		c.finishConensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}
	c.finishConensus(cr.ReqID, qt, p, true, cresp.Hash, cresp.ShareSig, cresp.PrivSig)
}

func (c *Core) pledgeQuorumToken(cr *ConensusRequest, tid string, dc did.DIDCrypto) (map[string]interface{}, error) {
	c.qlock.Lock()
	pd, ok1 := c.pd[cr.ReqID]
	cs, ok2 := c.quorumRequest[cr.ReqID]
	c.qlock.Unlock()
	if !ok1 || !ok2 {
		c.log.Error("Invalid pledge request")
		return nil, fmt.Errorf("invalid pledge request")
	}
	ntcb := make(map[string]interface{})
	ntcb[wallet.TCTransTypeKey] = wallet.TokenTransferredType
	ntcb[wallet.TCOwnerKey] = cr.ReceiverDID
	ntcb[wallet.TCTokensPledgedWithKey] = pd.TokenList
	tokenList := make([]string, 0)
	tokenList = append(tokenList, cr.WholeTokens...)
	//tokenList = append(tokenList, cr.PartTokens...)
	ntcb[wallet.TCTokensPledgedForKey] = tokenList
	ntcb[wallet.TCSenderDIDKey] = cr.SenderDID
	ntcb[wallet.TCGroupKey] = tokenList
	ntcb[wallet.TCCommentKey] = cr.Comment
	ntcb[wallet.TCTIDKey] = tid
	ntcb[wallet.TCReceiverDIDKey] = cr.ReceiverDID
	do := make(map[string]string)
	index := 0
	l := len(tokenList)
	phm := make(map[string]string)
	for _, v := range pd.PledgedTokens {
		for _, t := range v {
			tcb, err := c.w.GetLatestTokenBlock(tokenList[index])
			if err != nil {
				c.log.Error("Failed to get latest token chain block", "err", err)
				return nil, fmt.Errorf("Failed to get latest token chain block")
			}
			ph, ok := tcb[wallet.TCBlockHashKey]
			if !ok {
				c.log.Error("Failed to get block hash")
				return nil, fmt.Errorf("Failed to get block hash")
			}
			phm[tokenList[index]] = ph.(string)
			if index >= l {
				c.log.Error("Not enough pledged token")
				return nil, fmt.Errorf("Not enough pledged token")
			}
			do[tokenList[index]] = t
			index++
		}
	}
	ntcb[wallet.TCPreviousHashKey] = phm
	hs, err := wallet.TC2HashString(ntcb)
	if err != nil {
		c.log.Error("Failed to get token chain hash", "err", err)
		return nil, fmt.Errorf("Failed to get token chain hash")
	}
	ntcb[wallet.TCBlockHashKey] = hs

	sig, err := dc.PvtSign([]byte(hs))
	if err != nil {
		c.log.Error("Failed to get did signature", "err", err)
		return nil, fmt.Errorf("Failed to get did signature")
	}
	ntcb[wallet.TCSignatureKey] = util.HexToStr(sig)
	for k, v := range pd.PledgedTokens {
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
			PledgedTokens:   v,
			TokenChainBlock: ntcb,
		}
		err = p.SendJSONRequest("POST", APIUpdatePledgeToken, nil, &ur, &br, true)
		if err != nil {
			c.log.Error("Failed to update pledge token status", "err", err)
			return nil, fmt.Errorf("failed to update pledge token status")
		}
		if !br.Status {
			c.log.Error("Failed to update pledge token status", "msg", br.Message)
			return nil, fmt.Errorf("failed to update pledge token status")
		}
	}
	return ntcb, nil
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
		// Request pledage token
		if pd.RemPledgeTokens != 0 {
			pr := PledgeRequest{
				NumTokens: pd.RemPledgeTokens,
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
			if !prs.Status {
				c.log.Info("Unable to plegde token from peer", "did", p.GetPeerDID(), "msg", prs.Message)
			} else {
				did := p.GetPeerDID()
				pd.PledgedTokens[did] = make([]string, 0)
				for i, t := range prs.Tokens {
					pd.NumPledgedTokens++
					pd.RemPledgeTokens--
					pd.PledgedTokenChainBlock[t] = prs.TokenChainBlock[i]
					pd.ProofPledge[t] = prs.ProofChain[i]
					pd.PledgedTokens[did] = append(pd.PledgedTokens[did], t)
					pd.TokenList = append(pd.TokenList, t)
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
