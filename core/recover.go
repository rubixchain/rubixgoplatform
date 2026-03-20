package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	signModule "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
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
	// Convert []interface{} to []ContractTokenInfo
	var tokenInfos []ContractTokenInfo
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

		// TODO(phase07): implement DB-based token chain sync (block-based sync removed)
		c.log.Info("[STUB] skipping block-based token chain sync for token", "token", token)

		// TODO(phase07): implement parent token sync for part tokens using DB

		tokenDetails := &models.Token{
			TokenID:       token,
			ParentTokenID: pgtype.Text{String: "", Valid: false},
			TokenValue:    tokenInfo.TokenValue,
			DID:           tokenInfo.OwnerDID,
			TokenStatus:   int16(constants.TokenStatus_Free),
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		c.w.CreateToken(tokenDetails)
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
	tis := make([]ContractTokenInfo, 0)
	for i := range recoveredTokens {
		tts := "rbt"
		if recoveredTokens[i].TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)
		// TODO(phase07): implement DB-based block ID lookup for recovered tokens
		bid := "" // BlockID unavailable without block package
		ti := ContractTokenInfo{
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

func mapToTokenInfo(m map[string]interface{}) ContractTokenInfo {
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
	return ContractTokenInfo{
		Token:      m["token"].(string),
		TokenType:  int(tokenTypeInt64),
		TokenValue: tokenValueFloat64,
		OwnerDID:   m["ownerDID"].(string),
		BlockID:    m["blockID"].(string),
	}
}
