package core

import (
	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/util"
)

func (c *Core) InitiateRBTTransfer(req *model.RBTTransferRequest) *model.RBTTransferReply {
	resp := &model.RBTTransferReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	_, did, ok := util.ParseAddress(req.Sender)
	if !ok {
		resp.Message = "Invalid sender DID"
		return resp
	}
	// Get the required tokens from the DID bank
	// this method locks the token needs to be released or
	// removed once it done with the trasnfer
	wt, pt, err := c.w.GetTokens(did, req.TokenCount)
	if err != nil {
		resp.Message = "Insufficient tokens or tokens are locked"
		return resp
	}
	// release the locked tokens before exit
	defer c.w.ReleaseTokens(wt, pt)

	// Get the receiver & do sanity check
	p, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer p.Close()
	cr := ConensusRequest{
		ReqID: uuid.New().String(),
		Type:  req.Type,
	}
	err = c.initiateConsensus(cr)
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
