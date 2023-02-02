package core

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/block"
	didcrypto "github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) creditStatus(req *ensweb.Request) *ensweb.Result {
	// ::TODO:: Get proper credit score
	did := c.l.GetQuerry(req, "did")
	c.log.Debug("Getting credit score for", "did", did)
	credits, err := c.w.GetCredit(did)
	var cs model.CreditStatus
	cs.Score = 0
	if err == nil {
		cs.Score = len(credits)
	}
	return c.l.RenderJSON(req, &cs, http.StatusOK)
}

func (c *Core) quorumRBTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	err := c.FetchDID(cr.SenderDID)
	if err != nil {
		c.log.Error("Failed to get DID", "err", err)
		crep.Message = "Failed to get DID"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	// setup the did to verify the signature
	dc := c.SetupForienDID(cr.SenderDID)
	authHash := util.CalculateHashString(util.ConvertToJson(cr.WholeTokens)+util.ConvertToJson(cr.WholeTokenChain)+util.ConvertToJson(cr.PartTokens)+util.ConvertToJson(cr.PartTokenChain)+cr.ReceiverDID+cr.SenderDID+cr.Comment, "SHA3-256")
	ok, err := dc.Verify(authHash, cr.ShareSig, cr.PrivSig)
	if err != nil || !ok {
		c.log.Error("Failed to verify sender signature", "err", err)
		crep.Message = "Failed to verify sender signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	// check token ownership
	if !c.validateTokenOwnership(cr) {
		c.log.Error("Token ownership check failed")
		crep.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	qHash := util.CalculateHashString(authHash+cr.ReceiverDID, "SHA3-256")

	qsb, ppb, err := qdc.Sign(qHash)
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		crep.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	crep.Status = true
	crep.Message = "Conensus finished successfully"
	crep.ShareSig = qsb
	crep.PrivSig = ppb
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) quorumConensus(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var cr ConensusRequest
	err := c.l.ParseJSON(req, &cr)
	c.log.Debug("Conensus", "did", did)
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse request", "err", err)
		crep.Message = "Failed to parse request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	qdc, ok := c.qc[did]
	if !ok {
		c.log.Error("Quorum is not setup")
		crep.Message = "Quorum is not setup"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	switch cr.Mode {
	case RBTTransferMode:
		return c.quorumRBTConsensus(req, did, qdc, &cr)
	default:
		c.log.Error("Invalid consensus mode", "mode", cr.Mode)
		crep.Message = "Invalid consensus mode"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
}

func (c *Core) reqPledgeToken(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var pr PledgeRequest
	err := c.l.ParseJSON(req, &pr)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	wt, err := c.w.GetWholeTokens(did, pr.NumTokens)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		crep.Message = "Failed to get tokens"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	tl := len(wt)
	if tl == 0 {
		c.log.Error("No tokens left to pledge", "err", err)
		crep.Message = "No tokens left to pledge"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	presp := PledgeReply{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got available tokens",
		},
		Tokens:          make([]string, 0),
		TokenChainBlock: make([]interface{}, 0),
	}
	for i := 0; i < tl; i++ {
		presp.Tokens = append(presp.Tokens, wt[i].TokenID)
		tc, err := c.w.GetLatestTokenBlock(wt[i].TokenID)
		if err != nil || tc == nil {
			c.log.Error("Failed to get latest token chain block", "err", err)
			crep.Message = "Failed to get latest token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		presp.TokenChainBlock = append(presp.TokenChainBlock, tc)
	}
	return c.l.RenderJSON(req, &presp, http.StatusOK)
}

func (c *Core) updateReceiverToken(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SendTokenRequest
	err := c.l.ParseJSON(req, &sr)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	for _, t := range sr.WholeTokens {
		err = c.syncTokenChainFrom(sr.Address, t)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			crep.Message = "Failed to sync token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}
	b := block.InitBlock(block.TokenBlockType, sr.TokenChainBlock, nil)
	if b == nil {
		c.log.Error("Invalid token chain block", "err", err)
		crep.Message = "Invalid token chain block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = c.w.TokensReceived(did, sr.WholeTokens, sr.PartTokens, b)
	if err != nil {
		c.log.Error("Failed to update token status", "err", err)
		crep.Message = "Failed to update token status"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	crep.Status = true
	crep.Message = "Token recved successfully"
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) updatePledgeToken(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var ur UpdatePledgeRequest
	err := c.l.ParseJSON(req, &ur)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	dc, ok := c.qc[did]
	if !ok {
		c.log.Error("Failed to setup quorum crypto")
		crep.Message = "Failed to setup quorum crypto"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	b := block.InitBlock(block.TokenBlockType, ur.TokenChainBlock, nil)
	tcb := block.TokenChainBlock{
		TransactionType:   wallet.TokenPledgedType,
		TokenOwner:        did,
		Comment:           "Token is pledged at " + time.Now().String(),
		TokenChainDetials: b.GetBlockMap(),
	}
	ctcb := make(map[string]*block.Block)
	for _, t := range ur.PledgedTokens {
		lb, err := c.w.GetLatestTokenBlock(t)
		if err != nil || lb == nil {
			c.log.Error("Failed to get token chain block", "err", err)
			crep.Message = "Failed to get token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		ctcb[t] = lb
	}
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block")
		crep.Message = "Failed to create new token chain block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	ha, err := nb.GetHash()
	if err != nil {
		c.log.Error("Invalid new token chain block, missing block hash", "err", err)
		crep.Message = "Invalid new token chain block, missing block hash"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	sig, err := dc.PvtSign([]byte(ha))
	if err != nil {
		c.log.Error("Failed to get quorum signature", "err", err)
		crep.Message = "Failed to get quorum signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = nb.UpdateSignature(util.HexToStr(sig))
	if err != nil {
		c.log.Error("Failed to update signature to block", "err", err)
		crep.Message = "Failed to update signature to block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	for _, t := range ur.PledgedTokens {
		err = c.w.PledgeWholeToken(did, t, nb)
		if err != nil {
			c.log.Error("Failed to update pledge token", "err", err)
			crep.Message = "Failed to update pledge token"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}
	crep.Status = true
	crep.Message = "Token pledge status updated"
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) quorumCredit(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var credit CreditScore
	err := c.l.ParseJSON(req, &credit)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse request", "err", err)
		crep.Message = "Failed to parse request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	c.log.Debug("Credits recieved for quorum", "did", did)
	jb, err := json.Marshal(&credit)
	if err != nil {
		c.log.Error("Failed to parse request", "err", err)
		crep.Message = "Failed to parse request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = c.w.StoreCredit(did, base64.StdEncoding.EncodeToString(jb))
	if err != nil {
		c.log.Error("Failed to store credit", "err", err)
		crep.Message = "Failed to store credit"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	crep.Status = true
	crep.Message = "Credit accepted"
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}
