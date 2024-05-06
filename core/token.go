package core

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
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

func (c *Core) GetAllTokens(did string, tt string) (*model.TokenResponse, error) {
	tr := &model.TokenResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got all tokens",
		},
	}
	switch tt {
	case model.RBTType:
		tkns, err := c.w.GetAllTokens(did)
		if err != nil {
			return tr, nil
		}
		tr.TokenDetials = make([]model.TokenDetial, 0)
		for _, t := range tkns {
			td := model.TokenDetial{
				Token:  t.TokenID,
				Status: t.TokenStatus,
			}
			tr.TokenDetials = append(tr.TokenDetials, td)
		}
	case model.DTType:
		tkns, err := c.w.GetAllDataTokens(did)
		if err != nil {
			return tr, nil
		}
		tr.TokenDetials = make([]model.TokenDetial, 0)
		for _, t := range tkns {
			td := model.TokenDetial{
				Token:  t.TokenID,
				Status: t.TokenStatus,
			}
			tr.TokenDetials = append(tr.TokenDetials, td)
		}
	case model.NFTType:
		tkns := c.w.GetAllNFT(did)
		if tkns == nil {
			return tr, nil
		}
		tr.TokenDetials = make([]model.TokenDetial, 0)
		for _, t := range tkns {
			td := model.TokenDetial{
				Token:  t.TokenID,
				Status: t.TokenStatus,
			}
			tr.TokenDetials = append(tr.TokenDetials, td)
		}
	default:
		tr.BasicResponse.Status = false
		tr.BasicResponse.Message = "Invalid token type"
	}
	return tr, nil
}

func (c *Core) GetAccountInfo(did string) (model.DIDAccountInfo, error) {
	wt, err := c.w.GetAllTokens(did)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens", "err", err)
		return model.DIDAccountInfo{}, fmt.Errorf("failed to get tokens")
	}
	info := model.DIDAccountInfo{
		DID: did,
	}
	for _, t := range wt {
		switch t.TokenStatus {
		case wallet.TokenIsFree:
			info.RBTAmount = info.RBTAmount + t.TokenValue
			info.RBTAmount = floatPrecision(info.RBTAmount, MaxDecimalPlaces)
		case wallet.TokenIsLocked:
			info.LockedRBT = info.LockedRBT + t.TokenValue
			info.LockedRBT = floatPrecision(info.LockedRBT, MaxDecimalPlaces)
		case wallet.TokenIsPledged:
			info.PledgedRBT = info.PledgedRBT + t.TokenValue
			info.PledgedRBT = floatPrecision(info.PledgedRBT, MaxDecimalPlaces)
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
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      did,
			GenesisBlock:    gb,
			TransInfo:       ti,
			TokenValue:      floatPrecision(1.0, MaxDecimalPlaces),
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
		err = c.w.CreateTokenBlock(blk)
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

func (c *Core) getFromIPFS(path string) ([]byte, error) {
	rpt, err := c.ipfs.Cat(path)
	if err != nil {
		c.log.Error("failed to get from ipfs", "err", err, "path", path)
		return nil, err
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(rpt)
	b := buf.Bytes()
	return b, nil
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

func (c *Core) GetRequiredTokens(did string, txnAmount float64) ([]wallet.Token, float64, error) {
	requiredTokens := make([]wallet.Token, 0)
	var remainingAmount float64
	wholeValue := int(txnAmount)
	//fv := float64(txnAmount)
	decimalValue := txnAmount - float64(wholeValue)
	decimalValue = floatPrecision(decimalValue, MaxDecimalPlaces)
	reqAmt := floatPrecision(txnAmount, MaxDecimalPlaces)
	//check if whole value exists
	if wholeValue != 0 {
		//extract the whole amount part that is the integer value of txn amount
		//serach for the required whole amount
		wholeTokens, remWhole, err := c.w.GetWholeTokens(did, wholeValue)
		if err != nil && err.Error() != "no records found" {
			c.w.ReleaseTokens(wholeTokens)
			c.log.Error("failed to search for whole tokens", "err", err)
			return nil, 0.0, err
		}

		//if whole tokens are found add thgem to the variable required Tokens
		if len(wholeTokens) != 0 {
			c.log.Debug("found whole tokens in wallet adding them to required tokens list")
			requiredTokens = append(requiredTokens, wholeTokens...)
			//wholeValue = wholeValue - len(requiredTokens)
			reqAmt = reqAmt - float64(len(wholeTokens))
			reqAmt = floatPrecision(reqAmt, MaxDecimalPlaces)
		}

		if (len(wholeTokens) != 0 && remWhole > 0) || (len(wholeTokens) != 0 && remWhole == 0) {
			if reqAmt == 0 {
				return requiredTokens, remainingAmount, nil
			}
			c.log.Debug("No more whole token left in wallet , rest of needed amt ", reqAmt)
			allPartTokens, err := c.w.GetAllPartTokens(did)
			if err != nil {
				// In GetAllPartTokens, we first check if there are any part tokens present in
				// TokensTable. Now there could be a situation, where there aren't any part tokens
				// and it Should not error out, but proceed further. The "no records found" error string
				// is usually received from the Read() method the db.
				// Hence, in this case, we simply return with whatever values requiredTokens and reqAmt holds
				if strings.Contains(err.Error(), "no records found") {
					return requiredTokens, reqAmt, nil
				}
				c.w.ReleaseTokens(wholeTokens)
				c.log.Error("failed to lock part tokens", "err", err)
				return nil, 0.0, err
			}
			var sum float64
			for _, partToken := range allPartTokens {
				sum = sum + partToken.TokenValue
				sum = floatPrecision(sum, MaxDecimalPlaces)
			}
			if sum < reqAmt {
				c.w.ReleaseTokens(wholeTokens)
				c.log.Error("There are no Whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
				return nil, 0.0, fmt.Errorf("there are no whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
			}
			// Create a slice to store the indices of elements to be removed
			var indicesToRemove []int
			// Iterate through allPartTokens
			defer c.w.ReleaseTokens(allPartTokens)
			for i, partToken := range allPartTokens {
				// Subtract the partToken value from the txnAmount
				// If the transaction amount is less than the partToken.TokenValue, skip
				if reqAmt < partToken.TokenValue {
					continue
				}
				reqAmt -= partToken.TokenValue
				reqAmt = floatPrecision(reqAmt, MaxDecimalPlaces)
				// Add the partToken to the requiredTokens
				requiredTokens = append(requiredTokens, partToken)
				// Store the index of the element to be removed
				indicesToRemove = append(indicesToRemove, i)
				// Check if txnAmount goes negative
				if reqAmt == 0 {
					break
				}
			}
			// Remove elements from allPartTokens using copy
			for i, idx := range indicesToRemove {
				copy(allPartTokens[idx-i:], allPartTokens[idx-i+1:])
			}
			allPartTokens = allPartTokens[:len(allPartTokens)-len(indicesToRemove)]
			c.w.ReleaseTokens(allPartTokens)

			if reqAmt > 0 {
				// Add the remaining amount to the remainingAmount variable
				remainingAmount += reqAmt
				remainingAmount = floatPrecision(remainingAmount, MaxDecimalPlaces)
			}
		}

		//if no parts found anf remWhole is also not 0
		if len(wholeTokens) == 0 && remWhole > 0 {
			c.log.Debug("No whole tokens found. proceeding to get part tokens for txn")

			allPartTokens, err := c.w.GetAllPartTokens(did)
			if err != nil && err.Error() != "no records found" {
				c.log.Error("failed to search for part tokens", "err", err)
				return nil, 0.0, err
			}
			if len(allPartTokens) == 0 {
				c.log.Error("No part Tokens found , This wallet is empty", "err", err)
				return nil, 0.0, err
			}
			var sum float64
			for _, partToken := range allPartTokens {
				sum = sum + partToken.TokenValue
			}
			if sum < txnAmount {
				c.log.Error("There are no Whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
				return nil, 0.0, fmt.Errorf("there are no whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
			}
			// Create a slice to store the indices of elements to be removed
			var indicesToRemove []int
			// Iterate through allPartTokens
			defer c.w.ReleaseTokens(allPartTokens)
			for i, partToken := range allPartTokens {
				// Subtract the partToken value from the txnAmount
				// If the transaction amount is less than the partToken.TokenValue, skip
				if txnAmount < partToken.TokenValue {
					continue
				}
				txnAmount -= partToken.TokenValue
				txnAmount = floatPrecision(txnAmount, MaxDecimalPlaces)
				// Add the partToken to the requiredTokens
				requiredTokens = append(requiredTokens, partToken)
				// Store the index of the element to be removed
				indicesToRemove = append(indicesToRemove, i)
				// Check if txnAmount goes negative
				if txnAmount == 0 {
					break
				}
			}
			// Remove elements from allPartTokens using copy
			for i, idx := range indicesToRemove {
				copy(allPartTokens[idx-i:], allPartTokens[idx-i+1:])
			}
			allPartTokens = allPartTokens[:len(allPartTokens)-len(indicesToRemove)]
			c.w.ReleaseTokens(allPartTokens)
			if txnAmount > 0 {
				// Add the remaining amount to the remainingAmount variable
				remainingAmount += txnAmount
				remainingAmount = floatPrecision(remainingAmount, MaxDecimalPlaces)
			}

		}
	} else {
		return make([]wallet.Token, 0), reqAmt, nil
	}
	defer c.w.ReleaseTokens(requiredTokens)
	remainingAmount = floatPrecision(remainingAmount, MaxDecimalPlaces)
	return requiredTokens, remainingAmount, nil
}
