package core

import (
	"encoding/json"
	"fmt"
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
)
const (
	AlphaQuorumType int = iota
	BetaQuorumType
	GammaQuorumType
)

type ConensusRequest struct {
	ReqID          string   `json:"req_id"`
	Type           int      `json:"type"`
	Mode           int      `json:"mode"`
	SenderPeerID   string   `json:"sender_peerd_id"`
	ReceiverPeerID string   `json:"receiver_peerd_id"`
	ContractBlock  []byte   `json:"contract_block"`
	QuorumList     []string `json:"quorum_list"`
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
	PledgedTokenChainBlock map[string]interface{}
	TokenList              []string
}

type PledgeRequest struct {
	NumTokens int `json:"num_tokens"`
}

type SignatureRequest struct {
	TokenChainBlock []byte `json:"token_chain_block"`
}

type SignatureReply struct {
	model.BasicResponse
	Signature string `json:"signature"`
}

type UpdatePledgeRequest struct {
	PledgedTokens   []string `json:"pledged_tokens"`
	TokenChainBlock []byte   `json:"token_chain_block"`
}

type SendTokenRequest struct {
	Address         string   `json:"peer_id"`
	WholeTokens     []string `json:"whole_tokens"`
	PartTokens      []string `json:"part_tokens"`
	TokenChainBlock []byte   `json:"token_chain_block"`
}

type PledgeReply struct {
	model.BasicResponse
	Tokens          []string `json:"tokens"`
	TokenChainBlock [][]byte `json:"token_chain_block"`
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
	}
}

func (c *Core) SetupQuorum(didStr string, pwd string) error {
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
	return nil
}

func (c *Core) GetAllQuorum() []string {
	return c.qm.GetQuorum(QuorumTypeTwo)
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

func (c *Core) initiateConsensus(cr *ConensusRequest, sc *contract.Contract, dc did.DIDCrypto) error {
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
	var reqPledgeTokens int
	// TODO:: Need to correct for part tokens
	switch cr.Mode {
	case RBTTransferMode:
		reqPledgeTokens = len(sc.GetWholeTokens())
	case DTCommitMode:
		reqPledgeTokens = 1
	}
	pd := PledgeDetials{
		RemPledgeTokens:        reqPledgeTokens,
		NumPledgedTokens:       0,
		PledgedTokens:          make(map[string][]string),
		PledgedTokenChainBlock: make(map[string]interface{}),
		TokenList:              make([]string, 0),
	}
	ql := c.qm.GetQuorum(cr.Type)
	if ql == nil || len(ql) < MinQuorumRequired {
		c.log.Error("Failed to get required quorums")
		return fmt.Errorf("failed to get required quorums")
	}
	c.qlock.Lock()
	c.quorumRequest[cr.ReqID] = &cs
	c.pd[cr.ReqID] = &pd
	c.qlock.Unlock()
	cr.QuorumList = ql

	for _, a := range ql {
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
				c.log.Debug("Consensus finished successfully")
			} else if cs.Result.RunningCount == 0 {
				loop = false
				err = fmt.Errorf("consensus failed")
				c.log.Error("Consensus failed")
			}
		}
		c.qlock.Unlock()
		if !loop {
			break
		}
	}
	if err == nil {
		tid := util.HexToStr(util.CalculateHash(sc.GetBlock(), "SHA3-256"))
		nb, err := c.pledgeQuorumToken(cr, sc, tid, dc)
		if err != nil {
			c.log.Error("Failed to pledge token", "err", err)
			return err
		}
		c.sendQuorumCredit(cr)
		rp, err := c.getPeer(cr.ReceiverPeerID + "." + sc.GetReceiverDID())
		if err != nil {
			c.log.Error("Receiver not connected", "err", err)
			return err
		}
		sr := SendTokenRequest{
			Address:         cr.SenderPeerID + "." + sc.GetSenderDID(),
			WholeTokens:     sc.GetWholeTokens(),
			PartTokens:      sc.GetPartTokens(),
			TokenChainBlock: nb.GetBlock(),
		}
		var br model.BasicResponse
		err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)

		if err != nil {
			c.log.Error("Unable to send tokens to receiver", "err", err)
			return err
		}
		if !br.Status {
			c.log.Error("Unable to send tokens to receiver", "msg", br.Message)
			return fmt.Errorf("unable to send tokens to receiver, " + br.Message)
		}
		err = c.w.TokensTransferred(sc.GetSenderDID(), sc.GetWholeTokens(), sc.GetPartTokens(), nb)
		if err != nil {
			c.log.Error("Failed to transfer tokens", "err", err)
			return err
		}
		for _, t := range sc.GetWholeTokens() {
			c.w.UnPin(t, wallet.PrevSender, sc.GetSenderDID())
		}
		for _, t := range sc.GetPartTokens() {
			c.w.UnPin(t, wallet.PrevSender, sc.GetSenderDID())
		}
	}
	return err
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

func (c *Core) connectQuorum(cr *ConensusRequest, addr string, qt int) {
	c.startConsensus(cr.ReqID, qt)
	p, err := c.getPeer(addr)
	if err != nil {
		c.log.Error("Failed to get peer connection", "err", err)
		c.finishConsensus(cr.ReqID, qt, nil, false, "", nil, nil)
		return
	}
	err = c.initPledgeQuorumToken(cr, p, qt)
	if err != nil {
		c.log.Error("Failed to pleadge token", "err", err)
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
	if !cresp.Status {
		c.log.Error("Faile to get consensus", "msg", cresp.Message)
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
	wt := sc.GetWholeTokens()
	wtID := sc.GetWholeTokensID()
	pt := sc.GetPartTokens()
	ptID := sc.GetPartTokensID()
	tokenList := make([]string, 0)
	tokenList = append(tokenList, wt...)
	credit := make([]string, 0)
	for _, csig := range cs.Credit.Credit {
		jb, err := json.Marshal(csig)
		if err != nil {
			c.log.Error("Failed to parse quorum credit", "err", err)
			return nil, fmt.Errorf("failed to parse quorum credit")
		}
		credit = append(credit, string(jb))
	}

	//tokenList = append(tokenList, cr.PartTokens...)
	tcb := block.TokenChainBlock{
		TransactionType:   wallet.TokenTransferredType,
		TokenOwner:        sc.GetReceiverDID(),
		TokensPledgedWith: pd.TokenList,
		TokensPledgedFor:  tokenList,
		SenderDID:         sc.GetSenderDID(),
		WholeTokens:       wt,
		WholeTokensID:     wtID,
		PartTokens:        pt,
		PartTokensID:      ptID,
		QuorumSignature:   credit,
		Comment:           sc.GetComment(),
		TID:               tid,
		ReceiverDID:       sc.GetReceiverDID(),
		Contract:          sc.GetBlock(),
	}
	pm := make(map[string]interface{})
	index := 0
	ctcb := make(map[string]*block.Block)
	for _, v := range pd.PledgedTokens {
		for _, t := range v {
			b := c.w.GetLatestTokenBlock(tokenList[index])
			if b == nil {
				c.log.Error("Failed to get latest token chain block")
				return nil, fmt.Errorf("failed to get latest token chain block")
			}
			ctcb[tokenList[index]] = b
			pm[tokenList[index]] = t
			index++
		}
	}
	tcb.TokensPledgeMap = pm
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block")
		return nil, fmt.Errorf("failed to create new token chain block")
	}
	hs, err := nb.GetHash()
	c.log.Info("Token chain Hash", "hash", hs)
	if err != nil {
		c.log.Error("Failed to hash from new block", "err", err)
		return nil, fmt.Errorf("failed to hash from new block")
	}
	blk := nb.GetBlock()
	if blk == nil {
		c.log.Error("Failed to get new block", "err", err)
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
		err = p.SendJSONRequest("POST", APISignatureRequest, nil, &sr, &srep, true)
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
			TokenChainBlock: nb.GetBlock(),
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
					ptcb := block.InitBlock(prs.TokenChainBlock[i], nil)
					if !c.checkIsPledged(ptcb, t) {
						pd.NumPledgedTokens++
						pd.RemPledgeTokens--
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

func (c *Core) checkDIDMigrated(as *ArbitaryStatus, did string) {
	var br model.BasicResponse
	q := make(map[string]string)
	q["olddid"] = did
	err := as.p.SendJSONRequest("GET", APICheckDIDArbitration, q, nil, &br, true)
	if err != nil {
		c.log.Error("Failed to get did detials from arbitray", "err", err)
		return
	}
	if !br.Status {
		c.log.Error("Failed to get did detials from arbitray", "msg", br.Message)
		return
	}
	as.ds = br.Result.(bool)
	as.status = true
}

func (c *Core) mapMigratedDID(as *ArbitaryStatus, olddid string, newdid string) {
	var br model.BasicResponse
	m := make(map[string]string)
	m["olddid"] = olddid
	m["newdid"] = newdid
	err := as.p.SendJSONRequest("POST", APIMapDIDArbitration, nil, &m, &br, true)
	if err != nil {
		c.log.Error("Failed to get did detials from arbitray", "err", err)
		return
	}
	if !br.Status {
		c.log.Error("Failed to get did detials from arbitray", "msg", br.Message)
		return
	}
	as.status = true
}

func (c *Core) getArbitrationSignature(as *ArbitaryStatus, sr *SignatureRequest) {
	var srep SignatureReply
	err := as.p.SendJSONRequest("POST", APITokenArbitration, nil, sr, &srep, true)
	if err != nil {
		c.log.Error("Failed to get arbitray signature", "err", err)
		return
	}
	if !srep.Status {
		c.log.Error("Failed to get arbitray signature", "msg", srep.Message)
		return
	}
	as.sig = srep.Signature
	as.status = true
}
func (c *Core) checkIsPledged(tcb *block.Block, token string) bool {
	if strings.Compare(tcb.GetTransType(), wallet.TokenPledgedType) == 0 {
		c.log.Debug("Token", token, " is a pledged token. Not Considered for pledging")
		return true
	}
	return false
}
