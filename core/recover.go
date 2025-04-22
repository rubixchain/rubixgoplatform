package core

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	signModule "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type SendRecoverRequest struct {
	Address   string                  `json:"peer_id"`
	Signature signModule.DIDSignature `json:"Signature"`
	Hash      string                  `json:"hash"`
}

func (c *Core) InitiateRecoverRBT(reqID string, req *model.RBTRecoverRequest) {
	br := c.initiateRecoverRBT(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateRecoverRBT(reqID string, req *model.RBTRecoverRequest) *model.BasicResponse {

	resp := &model.BasicResponse{
		Status: false,
	}

	if req.Sender == req.PinningNode {
		resp.Message = "Sender and Pinning node cannot be same"
		return resp
	}

	did := req.Sender
	pinningNodeDID := req.PinningNode
	pinningNodepeerid := c.w.GetPeerID(pinningNodeDID)
	if pinningNodepeerid == "" {
		c.log.Error("Peer ID not found", "did", pinningNodepeerid)
		resp.Message = "invalid address, Peer ID not found"
		return resp
	}

	_, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	p, err := c.getPeer(req.PinningNode)
	if err != nil {
		resp.Message = "Failed to get pinning peer, " + err.Error()
		return resp
	}
	defer p.Close()
	var hashResponse model.BasicResponse
	err = p.SendJSONRequest("GET", APIRequestSigningHash, nil, nil, &hashResponse, true)
	if err != nil {
		c.log.Error("Unable to send Recover Token Request to the pinned node", "err", err)
		return &hashResponse
	}
	if !hashResponse.Status {
		c.log.Error("Failed to get hash for signing from the pinned node")
		return &hashResponse
	}
	hashForSign := hashResponse.Message
	var signFunc *signModule.DIDLite
	if req.Password == "" {
		signFunc = signModule.InitDIDLiteWithPassword(did, c.didDir, "mypassword")
	} else {
		signFunc = signModule.InitDIDLiteWithPassword(did, c.didDir, req.Password)
	}
	pvtSign, err := signFunc.PvtSign([]byte(hashForSign))
	if err != nil {
		c.log.Error("Failed to sign while recovering RBT")
		resp.Message = "Failed to sign, " + err.Error()
		return resp
	}
	signature := signModule.DIDSignature{
		Pixels:    []byte{},
		Signature: pvtSign,
	}
	sr := SendRecoverRequest{
		Address:   req.Sender,
		Signature: signature,
		Hash:      hashForSign,
	}
	var br model.BasicResponse
	err = p.SendJSONRequest("POST", APIRecoverPinnedRBT, nil, &sr, &br, true)
	if err != nil {
		c.log.Error("Unable to send Recover Token Request to the pinned node", "err", err)
		return &br
	}
	if !br.Status {
		c.log.Error("Failed to recover RBT: ", br.Message)
		return &br
	}

	retrieved, ok := br.Result.([]interface{})
	if !ok {
		c.log.Debug("Failed to retrieve slice from interface")
	}
	// Convert []interface{} to []TokenInfo
	var tokenInfos []contract.TokenInfo
	for _, item := range retrieved {
		if m, ok := item.(map[string]interface{}); ok {
			tokenInfo := mapToTokenInfo(m)
			tokenInfos = append(tokenInfos, tokenInfo)
		}
	}

	for _, tokenInfo := range tokenInfos {
		token := tokenInfo.Token
		tokenType := tokenInfo.TokenType
		tr := TCBSyncRequest{
			Token:     token,
			TokenType: tokenType,
			BlockID:   "",
		}

		var trep TCBSyncReply

		err = p.SendJSONRequest("POST", APISyncTokenChain, nil, &tr, &trep, false)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
		}
		if !trep.Status {
			c.log.Error("Failed to sync token chain block", "msg", trep.Message)
		}

		for _, bb := range trep.TCBlock {
			blk := block.InitBlock(bb, nil)
			if blk == nil {
				c.log.Error("Failed to add token chain block, invalid block, sync failed", "err", err)
			}
			err = c.w.AddTokenBlock(token, blk)
			if err != nil {
				c.log.Error("Failed to add token chain block, syncing failed", "err", err)
			}
		}
		var pt string
		if c.TokenType(PartString) == tokenType {
			gb := c.w.GetGenesisTokenBlock(token, tokenType)
			if gb == nil {
				c.log.Error("failed to get genesis block for token ", token)
			} else {
				pt, _, err = gb.GetParentDetials(token)
				if err != nil {
					c.log.Error("failed to get parent details for token ", token, "err : ", err)
					pt = "" // Ensure pt is reset to empty string if an error occurs
				}
			}
			_, err = c.syncParentToken(p, pt)
			if err != nil {
				c.log.Error("Failed to sync parent token chain while recovering token", err)

			}

		}

		tokenDetails := wallet.Token{
			TokenID:       token,
			ParentTokenID: pt,
			TokenValue:    tokenInfo.TokenValue,
			DID:           tokenInfo.OwnerDID,
			TokenStatus:   wallet.TokenIsFree,
		}
		c.w.CreateToken(&tokenDetails)
	}

	c.log.Info("Tokens recovered successfully")
	resp.Status = true
	msg := "Tokens recovered successfully"
	resp.Message = msg
	return resp
}

func (c *Core) recoverPinnedToken(req *ensweb.Request) *ensweb.Result {
	var sr SendRecoverRequest
	err := c.l.ParseJSON(req, &sr)
	crep := model.BasicResponse{
		Status: false,
	}
	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Failed to parse json request"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	p, err := c.getPeer(sr.Address)
	if err != nil {
		c.log.Error("failed to get peer", "err", err)
		crep.Message = "failed to get peer"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	defer p.Close()
	_, recoverNodeDID, _ := util.ParseAddress(sr.Address)
	recoveredTokens, err := c.w.GetAllPinnedTokens(recoverNodeDID)
	if err != nil {
		c.log.Error("Failed to get the pinned tokens of did :", recoverNodeDID, "err", err)
		crep.Message = "No tokens have been pinned for the DID :" + recoverNodeDID
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	signFunc := signModule.InitDIDLite(recoverNodeDID, c.didDir, nil)
	verified, err := signFunc.PvtVerify([]byte(sr.Hash), sr.Signature.Signature)
	if !verified {
		c.log.Error("Failed to verify signature of sender, Unable to recover tokens", "err", err)
		crep.Message = "Failed to verify signature of sender, Unable to recover tokens"
		return c.l.RenderJSON(req, &crep, http.StatusOK)
	}
	tis := make([]contract.TokenInfo, 0)
	for i := range recoveredTokens {
		tts := "rbt"
		if recoveredTokens[i].TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(recoveredTokens[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			crep.Message = "failed to get latest block, invalid token chain"
		}
		bid, err := blk.GetBlockID(recoveredTokens[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			crep.Message = "failed to get block id, " + err.Error()
		}
		ti := contract.TokenInfo{
			Token:      recoveredTokens[i].TokenID,
			TokenType:  tt,
			TokenValue: recoveredTokens[i].TokenValue,
			OwnerDID:   recoveredTokens[i].DID,
			BlockID:    bid,
		}
		tis = append(tis, ti)
	}

	crep.Status = true
	crep.Message = "Token Recovered Succesfully"
	crep.Result = tis
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func (c *Core) requestSigningHash(req *ensweb.Request) *ensweb.Result {
	c.log.Info("Request for Sign Hash received")
	crep := model.BasicResponse{
		Status: false,
	}
	hashForSign := uuid.New().String()
	crep.Status = true
	crep.Message = hashForSign
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func mapToTokenInfo(m map[string]interface{}) contract.TokenInfo {
	tokenType, err := m["tokenType"].(json.Number)
	if !err {
		fmt.Println("invalid type for tokenType :", err)
	}
	tokenTypeInt64, err1 := tokenType.Int64()
	if err1 != nil {
		fmt.Println("failed to convert tokenType to int64 :", err1)
	}
	toknValue, err2 := m["tokenValue"].(json.Number)
	if !err2 {
		fmt.Println("invalid type for tokenValue :", err2)
	}
	tokenValueFloat64, err3 := toknValue.Float64()
	if err3 != nil {
		fmt.Println("failed to convert tokenValue to float64:", err3)
	}
	return contract.TokenInfo{
		Token:      m["token"].(string),
		TokenType:  int(tokenTypeInt64),
		TokenValue: tokenValueFloat64,
		OwnerDID:   m["ownerDID"].(string),
		BlockID:    m["blockID"].(string),
	}
}
