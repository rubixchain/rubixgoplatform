package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/service"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	didcrypto "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
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

func (c *Core) quorumDTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	sc := contract.InitContract(cr.ContractBlock, nil)
	// setup the did to verify the signature
	dc, err := c.SetupForienDID(sc.GetSenderDID())
	if err != nil {
		c.log.Error("Failed to get DID", "err", err)
		crep.Message = "Failed to get DID"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = sc.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to verify sender signature", "err", err)
		crep.Message = "Failed to verify sender signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//check if token has multiple pins
	dt := sc.GetDataTokens()
	if dt == nil {
		c.log.Error("Consensus failed, data token missing")
		crep.Message = "Consensus failed, data token missing"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	for k, _ := range dt {
		c.ipfs.Pin(k)
	}
	qHash := util.CalculateHash(sc.GetBlock(), "SHA3-256")
	qsb, ppb, err := qdc.Sign(util.HexToStr(qHash))
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

func (c *Core) quorumRBTConsensus(req *ensweb.Request, did string, qdc didcrypto.DIDCrypto, cr *ConensusRequest) *ensweb.Result {
	crep := ConensusReply{
		ReqID:  cr.ReqID,
		Status: false,
	}
	sc := contract.InitContract(cr.ContractBlock, nil)
	// setup the did to verify the signature
	dc, err := c.SetupForienDID(sc.GetSenderDID())
	if err != nil {
		c.log.Error("Failed to get DID", "err", err)
		crep.Message = "Failed to get DID"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	err = sc.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to verify sender signature", "err", err)
		crep.Message = "Failed to verify sender signature"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//check if token has multiple pins
	t := sc.GetWholeTokens()
	for i := range t {
		multiPincheck, owners, err := c.pinCheck(t[i], cr.SenderPeerID, cr.ReceiverPeerID)
		if err != nil {
			c.log.Error("Error occurede", "error", err)
			crep.Message = "Token multiple Pin check error triggered"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if multiPincheck {
			c.log.Debug("Token ", "Token", t[i])
			c.log.Debug("Has Multiple owners", "owners", owners)
			crep.Message = "Token has multiple owners"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		c.log.Debug("Token ", "Token", t[i])
		c.log.Debug("Multiple pin check passed, does not have multiplke owners")
	}
	// check token ownership
	if !c.validateTokenOwnership(cr, sc) {
		c.log.Error("Token ownership check failed")
		crep.Message = "Token ownership check failed"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	//check if token is pledgedtoken
	wt := sc.GetWholeTokens()

	for i := range wt {
		if c.checkTokenIsPledged(wt[i]) {
			c.log.Error("Pledge Token check Failed, Token ", wt[i], " is Pledged Token")
			crep.Message = "Pledge Token check Failed, Token " + wt[i] + " is Pledged Token"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	qHash := util.CalculateHash(sc.GetBlock(), "SHA3-256")
	qsb, ppb, err := qdc.Sign(util.HexToStr(qHash))
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
	case DTCommitMode:
		return c.quorumDTConsensus(req, did, qdc, &cr)
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
		TokenChainBlock: make([][]byte, 0),
	}
	for i := 0; i < tl; i++ {
		presp.Tokens = append(presp.Tokens, wt[i].TokenID)
		tc := c.w.GetLatestTokenBlock(wt[i].TokenID)
		if tc == nil {
			c.log.Error("Failed to get latest token chain block")
			crep.Message = "Failed to get latest token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		presp.TokenChainBlock = append(presp.TokenChainBlock, tc.GetBlock())
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
	b := block.InitBlock(sr.TokenChainBlock, nil)
	if b == nil {
		c.log.Error("Invalid token chain block", "err", err)
		crep.Message = "Invalid token chain block"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}

	for _, t := range sr.WholeTokens {
		pblkID, err := b.GetPrevBlockID(t)
		if err != nil {
			c.log.Error("Failed to sync token chain block, missing previous block id", "err", err)
			crep.Message = "Failed to sync token chain block, missing previous block id"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		senderPeerId, _, ok := util.ParseAddress(sr.Address)
		if !ok {
			c.log.Error("Error occurede", "error", err)
			crep.Message = "Unable to parse sender address"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		multiPincheck, owners, err := c.pinCheck(t, senderPeerId, c.peerID)
		if err != nil {
			c.log.Error("Error occurede", "error", err)
			crep.Message = "Token multiple Pin check error triggered"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		if multiPincheck {
			c.log.Debug("Token ", "Token", t)
			c.log.Debug("Has Multiple owners", "owners", owners)
			crep.Message = "Token has multiple owners"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		err = c.syncTokenChainFrom(sr.Address, pblkID, t)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			crep.Message = "Failed to sync token chain block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		ptcbArray, err := c.w.GetTokenBlock(t, pblkID)
		if err != nil {
			c.log.Error("Failed to fetch previous block", "err", err)
			crep.Message = "Failed to fetch previous block"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
		ptcb := block.InitBlock(ptcbArray, nil)
		if c.checkIsPledged(ptcb, t) {
			c.log.Error("Token", t, "is a pledged Token")
			crep.Message = "Token " + t + " is a pledged Token"
			return c.l.RenderJSON(req, &crep, http.StatusOK)
		}
	}

	err = c.w.TokensReceived(did, sr.WholeTokens, sr.PartTokens, b)
	if err != nil {
		c.log.Error("Failed to update token status", "err", err)
		crep.Message = "Failed to update token status"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	amount := (float64(len(sr.WholeTokens)) + float64(len(sr.PartTokens))*0.001)
	crep.Status = true
	crep.Message = "Token received successfully"
	c.log.Info("Token received Suffessfully from", sr.Address)
	td := &wallet.TransactionDetails{
		TransactionID:   b.GetTid(),
		SenderDID:       b.GetSenderDID(),
		ReceiverDID:     b.GetReceiverDID(),
		Amount:          amount,
		Comment:         b.GetComment(),
		DateTime:        time.Now().UTC(),
		TotalTime:       0,
		WholeTokens:     sr.WholeTokens,
		PartTokens:      sr.PartTokens,
		PledgedTokenMap: b.GetTokenPledgeMap(),
	}
	c.w.AddTransactionHistory(td)
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) signatureRequest(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SignatureRequest
	err := c.l.ParseJSON(req, &sr)
	srep := SignatureReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		srep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	dc, ok := c.qc[did]
	if !ok {
		c.log.Error("Failed to setup quorum crypto")
		srep.Message = "Failed to setup quorum crypto"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	b := block.InitBlock(sr.TokenChainBlock, nil, block.NoSignature())
	if b == nil {
		c.log.Error("Failed to do signature, invalid token chain block")
		srep.Message = "Failed to do signature, invalid token chanin block"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	sig, err := b.GetSignature(dc)
	if err != nil {
		c.log.Error("Failed to do signature", "err", err)
		srep.Message = "Failed to do signature, " + err.Error()
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	srep.Signature = sig
	srep.Status = true
	srep.Message = "Signature done"
	return c.l.RenderJSON(req, &srep, http.StatusOK)
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
	b := block.InitBlock(ur.TokenChainBlock, nil)
	tcb := block.TokenChainBlock{
		TransactionType:   wallet.TokenPledgedType,
		TokenOwner:        did,
		Comment:           "Token is pledged at " + time.Now().String(),
		TokenChainDetials: b.GetBlockMap(),
	}
	ctcb := make(map[string]*block.Block)
	for _, t := range ur.PledgedTokens {
		lb := c.w.GetLatestTokenBlock(t)
		if lb == nil {
			c.log.Error("Failed to get token chain block")
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
	err = nb.UpdateSignature(did, dc)
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

func (c *Core) mapDIDArbitration(req *ensweb.Request) *ensweb.Result {
	var m map[string]string
	err := c.l.ParseJSON(req, &m)
	br := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		br.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	od, ok := m["olddid"]
	if !ok {
		c.log.Error("Missing old did value")
		br.Message = "Missing old did value"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	nd, ok := m["newdid"]
	if !ok {
		c.log.Error("Missing new did value")
		br.Message = "Missing new did value"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	err = c.srv.UpdateTokenDetials(nd)
	if err != nil {
		c.log.Error("Failed to update table detials", "err", err)
		br.Message = "Failed to update token detials"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	dm := &service.DIDMap{
		OldDID: od,
		NewDID: nd,
	}
	err = c.srv.UpdateDIDMap(dm)
	if err != nil {
		c.log.Error("Failed to update map table", "err", err)
		br.Message = "Failed to update map table"
		return c.l.RenderJSON(req, &br, http.StatusOK)
	}
	br.Status = true
	br.Message = "DID mapped successfully"
	return c.l.RenderJSON(req, &br, http.StatusOK)
}

func (c *Core) chekDIDArbitration(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "olddid")
	br := model.BasicResponse{
		Status: true,
	}
	if c.srv.IsDIDExist(did) {
		br.Message = "DID exist"
		br.Result = true
	} else {
		br.Message = "DID does not exist"
		br.Result = false
	}
	return c.l.RenderJSON(req, &br, http.StatusOK)
}

func (c *Core) tokenArbitration(req *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuerry(req, "did")
	var sr SignatureRequest
	err := c.l.ParseJSON(req, &sr)
	srep := SignatureReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		srep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}

	b := block.InitBlock(sr.TokenChainBlock, nil, block.NoSignature())
	if b == nil {
		c.log.Error("Failed to do token abitration, invalid token chain block")
		srep.Message = "Failed to do token abitration, invalid token chanin block"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	scb := b.GetContract()
	if scb == nil {
		c.log.Error("Failed to do token abitration, invalid token chain block, missing smart contract")
		srep.Message = "Failed to do token abitration, invalid token chain block, missing smart contract"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	sc := contract.InitContract(scb, nil)
	if sc == nil {
		c.log.Error("Failed to do token abitration, invalid smart contract")
		srep.Message = "Failed to do token abitration, invalid smart contract"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	t := sc.GetMigratedToken()
	if t == "" {
		c.log.Error("Failed to do token abitration, invalid token")
		srep.Message = "Failed to do token abitration, invalid token"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	tl, tn, err := b.GetTokenDetials()
	if err != nil {
		c.log.Error("Failed to do token abitration, invalid token detials", "err", err)
		srep.Message = "Failed to do token abitration, invalid token detials"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	str := token.GetTokenString(tl, tn)
	tbr := bytes.NewBuffer([]byte(str))
	thash, err := c.ipfs.Add(tbr, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	if err != nil {
		c.log.Error("Failed to do token abitration, failed to get ipfs hash", "err", err)
		srep.Message = "Failed to do token abitration, failed to get ipfs hash"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	if thash != t {
		c.log.Error("Failed to do token abitration, token hash not matching", "thash", thash, "token", t)
		srep.Message = "Failed to do token abitration, token hash not matching"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	odid := sc.GetOwnerDID()
	if odid == "" {
		c.log.Error("Failed to do token abitration, invalid owner did")
		srep.Message = "Failed to do token abitration, invalid owner did"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	td, err := c.srv.GetTokenDetials(t)
	if err == nil && td.Token == t {
		c.log.Error("Failed to do token abitration, token is already migrated", "token", t, "did", odid)
		srep.Message = "Failed to do token abitration, token is already migrated"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	dc, err := c.SetupForienDID(odid)
	if err != nil {
		c.log.Error("Failed to do token abitration, failed to setup did crypto", "token", t, "did", odid)
		srep.Message = "Failed to do token abitration, failed to setup did crypto"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	err = sc.VerifySignature(dc)
	if err != nil {
		c.log.Error("Failed to do token abitration, signature verification failed", "err", err)
		srep.Message = "Failed to do token abitration, signature verification failed"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}

	dc, ok := c.qc[did]
	if !ok {
		c.log.Error("Failed to setup quorum crypto")
		srep.Message = "Failed to setup quorum crypto"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	sig, err := b.GetSignature(dc)
	if err != nil {
		c.log.Error("Failed to do token abitration, failed to get signature", "err", err)
		srep.Message = "Failed to do token abitration, failed to get signature"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	err = c.srv.UpdateTempTokenDetials(&service.TokenDetials{Token: t, DID: odid})
	if err != nil {
		c.log.Error("Failed to do token abitration, failed update token detials", "err", err)
		srep.Message = "Failed to do token abitration, failed update token detials"
		return c.l.RenderJSON(req, &srep, http.StatusOK)
	}
	srep.Signature = sig
	srep.Status = true
	srep.Message = "Signature done"
	return c.l.RenderJSON(req, &srep, http.StatusOK)
}
