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

func (c *Core) InitiateRBTSelfTransfer(reqID string, req *model.RBTSelfTransferRequest) {
	br := c.initiateRBTSelfTransfer(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

type Token struct {
	TokenID       string  `gorm:"column:token_id;primaryKey"`
	ParentTokenID string  `gorm:"column:parent_token_id"`
	TokenValue    float64 `gorm:"column:token_value"`
	DID           string  `gorm:"column:did"`
	TokenStatus   int     `gorm:"column:token_status;"`
}

func (c *Core) initiateRBTSelfTransfer(reqID string, req *model.RBTSelfTransferRequest) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(req.Sender)
	if !ok {
		resp.Message = "Invalid sender DID"
		return resp
	}

	// Get the list of all tokens that are locked - Reason: 24 hours after pledging elapsed
	// this method gets new pledges for the tokens
	wt := c.Up.GetSelfTransferTokens(did)
	var tokens []Token
	for i := range wt {
		c.W.S.Read(wallet.TokenStorage, &tokens, "did=? AND token_id=?", did, wt[i])
	}

	//wt, err := c.W.GetTokens(did, 1)

	// release the locked tokens before exit
	//defer c.W.ReleaseTokens(wt)

	for i := range wt {
		c.W.Pin(wt[i], wallet.OwnerRole, did)
	}

	wta := make([]string, 0)
	for i := range wt {
		wta = append(wta, wt[i])
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	tokenType := token.RBTTokenType
	if c.testNet {
		tokenType = token.TestTokenType
	}
	tis := make([]contract.TokenInfo, 0)
	for i := range wt {
		blk := c.W.GetLatestTokenBlock(wt[i], tokenType)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		bid, err := blk.GetBlockID(wt[i])
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		ti := contract.TokenInfo{
			Token:     wt[i],
			TokenType: tokenType,
			OwnerDID:  did,
			BlockID:   bid,
		}
		tis = append(tis, ti)
	}
	epoch := time.Now()
	sct := &contract.ContractType{
		Type:       contract.SCRBTDirectType,
		PledgeMode: contract.POWPledgeMode,
		TotalRBTs:  float64(len(wt)),
		EpochTime:  epoch.String(),
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			ReceiverDID: did,
			Comment:     req.Comment,
			TransTokens: tis,
			EpochTime:   epoch.String(),
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
		ReceiverPeerID: c.peerID,
		ContractBlock:  sc.GetBlock(),
	}
	td, _, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	td.Amount = float64(len(wt))
	td.TotalTime = float64(dif.Milliseconds())
	c.W.AddTransactionHistory(td)
	etrans := &ExplorerTrans{
		TID:         td.TransactionID,
		SenderDID:   did,
		ReceiverDID: did,
		Amount:      float64(len(wt)),
		TrasnType:   req.Type,
		TokenIDs:    wta,
		QuorumList:  cr.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
	}
	c.ec.ExplorerTransaction(etrans)
	c.log.Info("Self Transfer finished successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("Self Transfer finished successfully in %v", dif)
	resp.Message = msg
	return resp
}
