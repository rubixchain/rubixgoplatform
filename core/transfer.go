package core

import (
	"fmt"
	"time"

	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
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
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	// Get the required tokens from the DID bank
	// this method locks the token needs to be released or
	// removed once it done with the transfer
	wt, err := c.GetTokens(dc, did, req.TokenCount)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked"
		return resp
	}
	// release the locked tokens before exit
	defer c.w.ReleaseTokens(wt)

	for i := range wt {
		c.w.Pin(wt[i].TokenID, wallet.OwnerRole, did)
	}

	// Get the receiver & do sanity check
	p, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer p.Close()
	wta := make([]string, 0)
	for i := range wt {
		wta = append(wta, wt[i].TokenID)
	}
	multipleBatch := false
	if len(wt) > TokenBatchSize {
		multipleBatch = true
	}
	tis := make([]contract.TokenInfo, 0)
	btis := make(map[string][]contract.TokenInfo)
	var mtis []contract.TokenInfo
	for i := range wt {
		if multipleBatch {
			if i%TokenBatchSize == 0 {
				mtis = make([]contract.TokenInfo, 0)
			}
		}
		tts := "rbt"
		if wt[i].TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(wt[i].TokenID, tt)
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
			Token:      wt[i].TokenID,
			TokenType:  tt,
			TokenValue: wt[i].TokenValue,
			OwnerDID:   wt[i].DID,
			BlockID:    bid,
		}
		if multipleBatch {
			mtis = append(mtis, ti)
		} else {
			tis = append(tis, ti)
		}
		if (i+1)%TokenBatchSize == 0 {
			d := i / TokenBatchSize
			btis[fmt.Sprintf("%d", d)] = mtis
		}
	}
	sct := &contract.ContractType{
		Type:       contract.SCRBTDirectType,
		PledgeMode: contract.POWPledgeMode,
		TotalRBTs:  req.TokenCount,
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			ReceiverDID: rdid,
			Comment:     req.Comment,
		},
	}
	if multipleBatch {
		sct.TransInfo.BatchTransTokens = btis
	} else {
		sct.TransInfo.TransTokens = tis
	}
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	scb := sc.GetBlock()
	if multipleBatch {
		is := 0
		for range btis {
			st := time.Now()
			cr := &ConensusRequest{
				ReqID:          uuid.New().String(),
				Type:           req.Type,
				SenderPeerID:   c.peerID,
				BatchID:        fmt.Sprintf("%d", is),
				ReceiverPeerID: rpeerid,
				ContractBlock:  scb,
			}
			is++
			td, _, err := c.initiateConsensus(cr, sc)
			if err != nil {
				c.log.Error("Consensus failed", "err", err)
				resp.Message = "Consensus failed" + err.Error()
				return resp
			}
			et := time.Now()
			dif := et.Sub(st)
			td.Amount = req.TokenCount
			td.TotalTime = float64(dif.Milliseconds())
			c.w.AddTransactionHistory(td)
			etrans := &ExplorerTrans{
				TID:         td.TransactionID,
				SenderDID:   did,
				ReceiverDID: rdid,
				Amount:      req.TokenCount,
				TrasnType:   req.Type,
				TokenIDs:    wta,
				QuorumList:  cr.QuorumList,
				TokenTime:   float64(dif.Milliseconds()),
			}
			c.ec.ExplorerTransaction(etrans)
			c.log.Info("Transfer finished successfully", "duration", dif)
		}
		resp.Status = true
		resp.Message = "Transfer finished successfully"
		return resp
	} else {
		cr := &ConensusRequest{
			ReqID:          uuid.New().String(),
			Type:           req.Type,
			SenderPeerID:   c.peerID,
			ReceiverPeerID: rpeerid,
			ContractBlock:  scb,
		}
		td, _, err := c.initiateConsensus(cr, sc)
		if err != nil {
			c.log.Error("Consensus failed", "err", err)
			resp.Message = "Consensus failed" + err.Error()
			return resp
		}
		et := time.Now()
		dif := et.Sub(st)
		td.Amount = req.TokenCount
		td.TotalTime = float64(dif.Milliseconds())
		c.w.AddTransactionHistory(td)
		etrans := &ExplorerTrans{
			TID:         td.TransactionID,
			SenderDID:   did,
			ReceiverDID: rdid,
			Amount:      req.TokenCount,
			TrasnType:   req.Type,
			TokenIDs:    wta,
			QuorumList:  cr.QuorumList,
			TokenTime:   float64(dif.Milliseconds()),
		}
		c.ec.ExplorerTransaction(etrans)
		c.log.Info("Transfer finished successfully", "duration", dif)
		resp.Status = true
		msg := fmt.Sprintf("Transfer finished successfully in %v", dif)
		resp.Message = msg
		return resp
	}
}
