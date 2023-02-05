package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
)

type TokenPublish struct {
	Token string `json:"token"`
}

type TCBSyncRequest struct {
	Token   string `json:"token"`
	BlockID string `json:"block_id"`
}

type TCBSyncReply struct {
	Status      bool     `json:"status"`
	Message     string   `json:"message"`
	NextBlockID string   `json:"next_block_id"`
	TCBlock     [][]byte `json:"tc_block"`
}

func (c *Core) getTokens(did string, amount float64) ([]string, []string, bool) {
	return nil, nil, true
}

func (c *Core) removeTokens(did string, wholeTokens []string, partTokens []string) error {
	// ::TODO:: remove the tokens from the bank
	return nil
}

func (c *Core) releaseTokens(did string, wholeTokens []string, partTokens []string) error {
	// ::TODO:: releae the tokens which is lokced for the transaction
	return nil
}

func (c *Core) SetupToken() {
	c.l.AddRoute(APISyncTokenChain, "POST", c.syncTokenChain)
}

func (c *Core) GetAccountInfo(did string) (model.DIDAccountInfo, error) {
	wt, err := c.w.GetAllWholeTokens(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		return model.DIDAccountInfo{}, fmt.Errorf("Failed to get tokens")
	}
	pt, err := c.w.GetAllPartTokens(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		return model.DIDAccountInfo{}, fmt.Errorf("Failed to get tokens")
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
	for _, t := range pt {
		switch t.TokenStatus {
		case wallet.TokenIsFree:
			info.PartRBT++
		case wallet.TokenIsLocked:
			info.LockedPartRBT++
		case wallet.TokenIsPledged:
			info.PledgedPartRBT++
		}
	}
	return info, nil
}

func (c *Core) GenerateTestTokens(reqID string, num int, did string) error {
	if !c.testNet {
		return fmt.Errorf("This operation only avialable in test net")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}

	for i := 0; i < num; i++ {
		m := make(map[string]string)
		m["timeStamp"] = time.Now().String()
		mb, err := json.Marshal(m)
		if err != nil {
			c.log.Error("Failed to do json marshal (timestamp)", "err", err)
			return fmt.Errorf("failed to do json marshal")
		}

		rt := &rac.RacType{
			Type:         rac.RacTestTokenType,
			DID:          did,
			TotalSupply:  1,
			CreatorInput: string(mb),
		}

		r, err := rac.CreateRac(rt)
		if err != nil {
			c.log.Error("Failed to create rac block", "err", err)
			return fmt.Errorf("failed to create rac block")
		}

		ha, err := r[0].GetHash()
		if err != nil {
			c.log.Error("Failed to calculate rac hash", "err", err)
			return err
		}
		sig, err := dc.PvtSign([]byte(ha))
		if err != nil {
			c.log.Error("Failed to get rac signature", "err", err)
			return err
		}
		r[0].UpdateSignature(util.HexToStr(sig))

		tb := r[0].GetBlock()
		if tb == nil {
			c.log.Error("Failed to get rac block")
			return err
		}
		tk := util.HexToStr(tb)
		nb := bytes.NewBuffer([]byte(tk))
		id, err := c.ipfs.Add(nb)
		if err != nil {
			c.log.Error("Failed to add token to network", "err", err)
			return err
		}

		tcb := &block.TokenChainBlock{
			TransactionType: wallet.TokenGeneratedType,
			TokenOwner:      did,
			TokenID:         id,
			Comment:         "Token generated at " + time.Now().String(),
		}

		ctcb := make(map[string]*block.Block)
		ctcb[id] = nil

		blk := block.CreateNewBlock(ctcb, tcb)

		if blk == nil {
			c.log.Error("Failed to create new token chain block")
			return fmt.Errorf("Failed to create new token chain block")
		}

		hash, err := blk.GetHash()
		if err != nil {
			c.log.Error("Invalid new token chain block, missing block hash")
			return fmt.Errorf("Invalid new token chain block, missing block hash")
		}

		bid, err := blk.GetBlockID(id)

		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return fmt.Errorf("Failed to get block id")
		}

		sig, err = dc.PvtSign([]byte(hash))
		if err != nil {
			c.log.Error("Failed to get did signature", "err", err)
			return fmt.Errorf("Failed to get did signature")
		}

		err = blk.UpdateSignature(util.HexToStr(sig))
		if err != nil {
			c.log.Error("Failed to update did signature", "err", err)
			return fmt.Errorf("Failed to update did signature")
		}

		t := &wallet.Token{
			TokenID:      id,
			TokenDetials: tk,
			DID:          did,
			TokenChainID: bid,
			TokenStatus:  wallet.TokenIsFree,
		}
		err = c.w.AddTokenBlock(id, blk)
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
	blks, nextID, err := c.w.GetAllTokenBlocks(tr.Token, tr.BlockID)
	if err != nil {
		return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: err.Error()}, http.StatusOK)
	}
	return c.l.RenderJSON(req, &TCBSyncReply{Status: true, Message: "Got all blocks", TCBlock: blks, NextBlockID: nextID}, http.StatusOK)
}

func (c *Core) syncTokenChainFrom(address string, cb *block.Block, token string) error {
	p, err := c.getPeer(address)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return err
	}
	defer p.Close()
	blk, err := c.w.GetLatestTokenBlock(token)
	if err != nil {
		c.log.Error("Failed to token chain block", "err", err)
		return err
	}
	blkID := ""
	if blk != nil {
		blkID, err = blk.GetBlockID(token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return err
		}
		pblkID, err := cb.GetPrevBlockID(token)
		if err != nil {
			c.log.Error("Failed to get previous block id", "err", err)
			return err
		}
		if blkID == pblkID {
			c.log.Debug("Blokcs are already synced", "blkid", blkID)
			return nil
		}
	}
	tr := TCBSyncRequest{
		Token:   token,
		BlockID: blkID,
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
			blk := block.InitBlock(block.TokenBlockType, bb, nil)
			if blk == nil {
				c.log.Error("Failed to add token chain block, invalid block, sync failed", "err", err)
				return fmt.Errorf("failed to add token chain block, invalid block, sync failed")
			}
			c.log.Debug("Got tokens", "map", blk.GetBlockMap())
			err = c.w.AddTokenBlock(token, blk)
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

func (c *Core) tokenStatusCallback(peerID string, topic string, data []byte) {
	// c.log.Debug("Recevied token status request")
	// var tp TokenPublish
	// err := json.Unmarshal(data, &tp)
	// if err != nil {
	// 	return
	// }
	// c.log.Debug("Token recevied", "token", tp.Token)
}
