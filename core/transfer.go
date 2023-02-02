package core

import (
	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/util"
)

func (c *Core) InitiateRBTTransfer(reqID string, req *model.RBTTransferRequest) *model.BasicResponse {
	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(req.Sender)
	if !ok {
		resp.Message = "Invalid sender DID"
		return resp
	}

	rpeerid, rdid, ok := util.ParseAddress(req.Receiver)
	if !ok {
		resp.Message = "Invalid receiver DID"
		return resp
	}
	// Get the required tokens from the DID bank
	// this method locks the token needs to be released or
	// removed once it done with the trasnfer
	wt, pt, err := c.w.GetTokens(did, req.TokenCount)
	if err != nil {
		c.log.Error("Failed to get tkens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked"
		return resp
	}
	// release the locked tokens before exit
	defer c.w.ReleaseTokens(wt, pt)

	//Pinning the Tokens by the Sender, this will be released once consesusu is successful
	for _, t := range wt {
		c.ipfs.Pin(t.TokenID)
	}

	// Get the receiver & do sanity check
	p, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer p.Close()
	wta := make([]string, 0)
	wtca := make([]string, 0)
	wtcb := make([]map[string]interface{}, 0)
	for i := range wt {
		wta = append(wta, wt[i].TokenID)
		wtca = append(wtca, wt[i].TokenChainID)
		tcb, err := c.w.GetLatestTokenBlock(wt[i].TokenID)
		if err != nil {
			resp.Message = "Failed to get latest token chain block"
			return resp
		}
		wtcb = append(wtcb, tcb)
	}
	pta := make([]string, 0)
	ptca := make([]string, 0)
	for i := range pt {
		pta = append(pta, pt[i].TokenID)
		ptca = append(ptca, pt[i].TokenChainID)
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	authHash := util.CalculateHashString(util.ConvertToJson(wta)+util.ConvertToJson(wtca)+util.ConvertToJson(pta)+util.ConvertToJson(ptca)+rdid+did+req.Comment, "SHA3-256")
	ssig, psig, err := dc.Sign(authHash)
	if err != nil {
		c.log.Error("Failed to get signature", "err", err)
		resp.Message = "Failed to get signature, " + err.Error()
		return resp
	}
	cr := &ConensusRequest{
		ReqID:           uuid.New().String(),
		Type:            req.Type,
		SenderPeerID:    c.peerID,
		ReceiverPeerID:  rpeerid,
		SenderDID:       did,
		ReceiverDID:     rdid,
		WholeTokens:     wta,
		WholeTokenChain: wtca,
		WholeTCBlocks:   wtcb,
		PartTokens:      pta,
		PartTokenChain:  ptca,
		Comment:         req.Comment,
		ShareSig:        ssig,
		PrivSig:         psig,
	}
	err = util.CreateDir(c.cfg.DirPath + "Temp/" + cr.ReqID)
	if err != nil {
		c.log.Error("Failed to create directory", "err", err)
		resp.Message = "Failed to create directory, " + err.Error()
		return resp
	}
	err = c.initiateConsensus(cr, dc)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed, " + err.Error()
		return resp
	}
	c.log.Info("Trasnfer finsihed successfully")
	resp.Status = true
	resp.Message = "Trasnfer finsihed successfully"
	return resp
}
