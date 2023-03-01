package core

import (
	"fmt"
	"time"

	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) InitiateRBTTransfer(reqID string, req *model.RBTTransferRequest) {
	br := c.initiateRBTTransfer(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateRBTTransfer(reqID string, req *model.RBTTransferRequest) *model.BasicResponse {
	st := time.Now()
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
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked"
		return resp
	}
	// release the locked tokens before exit
	defer c.w.ReleaseTokens(wt, pt)

	for i := range wt {
		c.w.Pin(wt[i].TokenID, wallet.Owner, did)
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
	wtcb := make([][]byte, 0)
	for i := range wt {
		wta = append(wta, wt[i].TokenID)
		wtca = append(wtca, wt[i].TokenChainID)
		blk := c.w.GetLatestTokenBlock(wt[i].TokenID)
		if blk == nil {
			c.log.Error("Failed to get latest token chain block")
			resp.Message = "Failed to get latest token chain block"
			return resp
		}
		wtcb = append(wtcb, blk.GetBlock())
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	tis := make([]contract.TokenInfo, 0)
	for i := range wt {
		blk := c.w.GetLatestTokenBlock(wt[i].TokenID)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		bid, err := blk.GetBlockID(wt[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		ti := contract.TokenInfo{
			Token:     wt[i].TokenID,
			TokenType: token.RBTTokenType,
			OwnerDID:  wt[i].DID,
			BlockID:   bid,
		}
		tis = append(tis, ti)
	}
	sct := &contract.ContractType{
		Type:       contract.SCRBTDirectType,
		PledgeMode: contract.POWPledgeMode,
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			ReceiverDID: rdid,
			Comment:     req.Comment,
			TransTokens: tis,
		},
	}
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	cr := &ConensusRequest{
		ReqID:          uuid.New().String(),
		Type:           req.Type,
		SenderPeerID:   c.peerID,
		ReceiverPeerID: rpeerid,
		ContractBlock:  sc.GetBlock(),
	}
	err = c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	c.log.Info("Transfer finished successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("Transfer finished successfully in %v", dif)
	resp.Message = msg
	tid := util.HexToStr(util.CalculateHash(cr.ContractBlock, "SHA3-256"))
	// tokenList := make([]string, 0)
	// tokenList = append(tokenList, wta...)
	// tokenList = append(tokenList, pta...)
	// pd := c.pd[cr.ReqID]
	// pm := make(map[string]interface{})
	// index := 0
	// ctcb := make(map[string]*block.Block)
	// for _, v := range pd.PledgedTokens {
	// 	for _, t := range v {
	// 		b := c.w.GetLatestTokenBlock(tokenList[index])
	// 		if b == nil {
	// 			c.log.Error("Failed to get latest token chain block")
	// 		}
	// 		ctcb[tokenList[index]] = b
	// 		pm[tokenList[index]] = t
	// 		index++
	// 	}
	// }
	// td := &wallet.TransactionDetails{
	// 	TransactionID:   tid,
	// 	SenderDID:       did,
	// 	ReceiverDID:     rdid,
	// 	Amount:          req.TokenCount,
	// 	Comment:         req.Comment,
	// 	DateTime:        time.Now().UTC(),
	// 	TotalTime:       int(dif),
	// 	WholeTokens:     wta,
	// 	PartTokens:      pta,
	// 	QuorumList:      cr.QuorumList,
	// 	PledgedTokenMap: pm,
	// }
	// c.w.AddTransactionHistory(td)
	etrans := &ExplorerTrans{
		TID:         tid,
		SenderDID:   did,
		ReceiverDID: rdid,
		Amount:      req.TokenCount,
		TrasnType:   req.Type,
		TokenIDs:    wta,
		QuorumList:  cr.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
	}
	c.ec.ExplorerTransaction(etrans)
	return resp
}
