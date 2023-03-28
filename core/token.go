package core

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

type TokenPublish struct {
	Token string `json:"token"`
}

type TCBSyncRequest struct {
	Token     string `json:"token"`
	TokenType int    `json:"token_type"`
	BlockID   string `json:"block_id"`
}

type TCBSyncReply struct {
	Status      bool     `json:"status"`
	Message     string   `json:"message"`
	NextBlockID string   `json:"next_block_id"`
	TCBlock     [][]byte `json:"tc_block"`
}

func (c *Core) SetupToken() {
	c.l.AddRoute(APISyncTokenChain, "POST", c.syncTokenChain)
}

func (c *Core) GetAccountInfo(did string) (model.DIDAccountInfo, error) {
	wt, err := c.w.GetAllWholeTokens(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		return model.DIDAccountInfo{}, fmt.Errorf("failed to get tokens")
	}
	info := model.DIDAccountInfo{
		DID: did,
	}
	for _, t := range wt {
		switch t.TokenStatus {
		case wallet.TokenIsFree:
			info.WholeRBT++
		case wallet.TokenIsLocked:
			info.LockedWholeRBT++
		case wallet.TokenIsPledged:
			info.PledgedWholeRBT++
		}
	}
	return info, nil
}

func (c *Core) GenerateTestTokens(reqID string, num int, did string) {
	err := c.generateTestTokens(reqID, num, did)
	br := model.BasicResponse{
		Status:  true,
		Message: "DID registered successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) generateTestTokens(reqID string, num int, did string) error {
	if !c.testNet {
		return fmt.Errorf("generate test token is available in test net")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}

	for i := 0; i < num; i++ {

		rt := &rac.RacType{
			Type:        rac.RacTestTokenType,
			DID:         did,
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
		}

		r, err := rac.CreateRac(rt)
		if err != nil {
			c.log.Error("Failed to create rac block", "err", err)
			return fmt.Errorf("failed to create rac block")
		}

		// Assuming bo block token creation
		// ha, err := r[0].GetHash()
		// if err != nil {
		// 	c.log.Error("Failed to calculate rac hash", "err", err)
		// 	return err
		// }
		// sig, err := dc.PvtSign([]byte(ha))
		// if err != nil {
		// 	c.log.Error("Failed to get rac signature", "err", err)
		// 	return err
		// }
		err = r[0].UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update rac signature", "err", err)
			return err
		}

		tb := r[0].GetBlock()
		if tb == nil {
			c.log.Error("Failed to get rac block")
			return err
		}
		tk := util.HexToStr(tb)
		nb := bytes.NewBuffer([]byte(tk))
		id, err := c.w.Add(nb, did, wallet.OwnerRole)
		if err != nil {
			c.log.Error("Failed to add token to network", "err", err)
			return err
		}
		gb := &block.GenesisBlock{
			Type: block.TokenGeneratedType,
			Info: []block.GenesisTokenInfo{
				{Token: id},
			},
		}
		ti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     id,
					TokenType: token.TestTokenType,
				},
			},
		}

		tcb := &block.TokenChainBlock{
			TokenType:       token.TestTokenType,
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      did,
			GenesisBlock:    gb,
			TransInfo:       ti,
		}

		ctcb := make(map[string]*block.Block)
		ctcb[id] = nil

		blk := block.CreateNewBlock(ctcb, tcb)

		if blk == nil {
			c.log.Error("Failed to create new token chain block")
			return fmt.Errorf("failed to create new token chain block")
		}
		err = blk.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update did signature", "err", err)
			return fmt.Errorf("failed to update did signature")
		}
		t := &wallet.Token{
			TokenID:     id,
			DID:         did,
			TokenValue:  1,
			TokenStatus: wallet.TokenIsFree,
		}
		err = c.w.CreateTokenBlock(blk, token.TestTokenType)
		if err != nil {
			c.log.Error("Failed to add token chain", "err", err)
			return err
		}
		err = c.w.CreateToken(t)
		if err != nil {
			c.log.Error("Failed to create token", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) syncTokenChain(req *ensweb.Request) *ensweb.Result {
	var tr TCBSyncRequest
	err := c.l.ParseJSON(req, &tr)
	if err != nil {
		return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: "Failed to parse request"}, http.StatusOK)
	}
	blks, nextID, err := c.w.GetAllTokenBlocks(tr.Token, tr.TokenType, tr.BlockID)
	if err != nil {
		return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: err.Error()}, http.StatusOK)
	}
	return c.l.RenderJSON(req, &TCBSyncReply{Status: true, Message: "Got all blocks", TCBlock: blks, NextBlockID: nextID}, http.StatusOK)
}

func (c *Core) syncTokenChainFrom(p *ipfsport.Peer, pblkID string, token string, tokenType int) error {
	// p, err := c.getPeer(address)
	// if err != nil {
	// 	c.log.Error("Failed to get peer", "err", err)
	// 	return err
	// }
	// defer p.Close()
	var err error
	blk := c.w.GetLatestTokenBlock(token, tokenType)
	blkID := ""
	if blk != nil {
		blkID, err = blk.GetBlockID(token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return err
		}
		if blkID == pblkID {
			return nil
		}
	}
	tr := TCBSyncRequest{
		Token:     token,
		TokenType: tokenType,
		BlockID:   blkID,
	}
	for {
		var trep TCBSyncReply
		err = p.SendJSONRequest("POST", APISyncTokenChain, nil, &tr, &trep, false)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			return err
		}
		if !trep.Status {
			c.log.Error("Failed to sync token chain block", "msg", trep.Message)
			return fmt.Errorf(trep.Message)
		}
		for _, bb := range trep.TCBlock {
			blk := block.InitBlock(bb, nil)
			if blk == nil {
				c.log.Error("Failed to add token chain block, invalid block, sync failed", "err", err)
				return fmt.Errorf("failed to add token chain block, invalid block, sync failed")
			}
			err = c.w.AddTokenBlock(token, tokenType, blk)
			if err != nil {
				c.log.Error("Failed to add token chain block, syncing failed", "err", err)
				return err
			}
		}
		if trep.NextBlockID == "" {
			break
		}
		tr.BlockID = trep.NextBlockID
	}
	return nil
}

// func (c *Core) tokenStatusCallback(peerID string, topic string, data []byte) {
// 	// c.log.Debug("Recevied token status request")
// 	// var tp TokenPublish
// 	// err := json.Unmarshal(data, &tp)
// 	// if err != nil {
// 	// 	return
// 	// }
// 	// c.log.Debug("Token recevied", "token", tp.Token)
// }
