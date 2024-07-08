package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
}

func (c *Core) InitiateRecoverRBT(reqID string, req *model.RBTRecoverRequest) {
	br := c.initiateRecoverRBT(reqID, req)
	fmt.Println("Request ID in InitiateRecoverRBT:", reqID)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateRecoverRBT(reqID string, req *model.RBTRecoverRequest) *model.BasicResponse {

	fmt.Println("Initiate Recover RBT Called ")
	resp := &model.BasicResponse{
		Status: false,
	}

	if req.Sender == req.PinningNode {
		resp.Message = "Sender and Pinning node cannot be same"
		return resp
	}

	if !strings.Contains(req.Sender, ".") || !strings.Contains(req.PinningNode, ".") {
		resp.Message = "Sender and Pinning Node address should be of the format PeerID.DID"
		return resp
	}
	_, did, ok := util.ParseAddress(req.Sender)
	if !ok {
		resp.Message = "Invalid sender DID"
		return resp
	}

	pinningNodepeerid, pinningNodeDID, ok := util.ParseAddress(req.PinningNode)
	fmt.Println("Pinning node:", pinningNodeDID, pinningNodepeerid)
	if !ok {
		resp.Message = "Invalid pinning DID"
		return resp
	}

	if req.TokenCount < MinTrnxAmt {
		resp.Message = "Input transaction amount is less than minimum transaction amount"
		return resp
	}

	decimalPlaces := strconv.FormatFloat(req.TokenCount, 'f', -1, 64)
	decimalPlacesStr := strings.Split(decimalPlaces, ".")
	if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > MaxDecimalPlaces {
		c.log.Error("Transaction amount exceeds %d decimal places.\n", MaxDecimalPlaces)
		resp.Message = fmt.Sprintf("Transaction amount exceeds %d decimal places.\n", MaxDecimalPlaces)
		return resp
	}

	dc, err := c.SetupDID(reqID, did)
	fmt.Println("dc", dc)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	p, err := c.getPeer(req.PinningNode)
	fmt.Println("P in getPeer :", p)
	if err != nil {
		resp.Message = "Failed to get pinning peer, " + err.Error()
		return resp
	}
	defer p.Close()
	var signFunc *signModule.DIDLite
	if req.Password == "" {
		signFunc = signModule.InitDIDLiteWithPassword(did, c.didDir, "mypassword")
	} else {
		signFunc = signModule.InitDIDLiteWithPassword(did, c.didDir, req.Password)
	}
	//signFunc := signModule.InitDIDLiteWithPassword(did, c.didDir, "mypassword")
	fmt.Println("c.didDIR", c.didDir)
	pvtSign, err := signFunc.PvtSign([]byte(did))
	fmt.Println("DID in initiateRecoverRBT", did)
	fmt.Println("PVTSIGN in initiate Recover RBT", pvtSign)
	fmt.Println("Error in initiateRecoverRBT", err)
	signature := signModule.DIDSignature{
		Pixels:    []byte{},
		Signature: pvtSign,
	}
	sr := SendRecoverRequest{
		Address:   req.Sender,
		Signature: signature,
	}
	var br model.BasicResponse
	err = p.SendJSONRequest("POST", APIRecoverPinnedRBT, nil, &sr, &br, true)
	if err != nil {
		c.log.Error("Unable to send tokens to receiver", "err", err)
		return nil
	}

	fmt.Println(br.Message)
	fmt.Println(br.Result)
	fmt.Println(br.Status)

	fmt.Printf("Type: %T, Value: %v\n", br.Result, br.Result)

	//tokenlist := br.Result
	retrieved, ok := br.Result.([]interface{})
	if !ok {
		fmt.Println("Failed to retrieve slice from interface")
	}
	// Convert []interface{} to []TokenInfo
	var tokenInfos []contract.TokenInfo
	for _, item := range retrieved {
		if m, ok := item.(map[string]interface{}); ok {
			tokenInfo := mapToTokenInfo(m)
			tokenInfos = append(tokenInfos, tokenInfo)
		}
	}

	// Print the retrieved data
	fmt.Println("Successfully retrieved TokenInfo slice from interface:")
	for i, tokenInfo := range tokenInfos {
		fmt.Printf("TokenInfo %d: %+v\n", i, tokenInfo)
		fmt.Printf("Block ID: %s\n", tokenInfo.BlockID)
		fmt.Printf("Owner DID: %s\n", tokenInfo.OwnerDID)
		fmt.Printf("Token: %s\n", tokenInfo.Token)
		fmt.Printf("Token Type: %d\n", tokenInfo.TokenType)
		fmt.Printf("Token Value: %f\n", tokenInfo.TokenValue)

		token := tokenInfo.Token
		tokenType := tokenInfo.TokenType
		blockID := tokenInfo.BlockID

		// Perform your desired operation with the extracted fields
		// For example, just print them
		fmt.Printf("Token: %s, TokenType: %d, BlockID: %s\n", token, tokenType, blockID)

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

		fmt.Println("The response from the json request ", trep)
		for _, bb := range trep.TCBlock {
			blk := block.InitBlock(bb, nil)
			if blk == nil {
				c.log.Error("Failed to add token chain block, invalid block, sync failed", "err", err)
			}
			fmt.Println("The block is ", blk)
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
			err = c.syncParentToken(p, pt)
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
	pinNodeDid := c.l.GetQuerry(req, "did")
	fmt.Println("DID in recoverPinnedToken :", pinNodeDid)
	var sr SendRecoverRequest
	err := c.l.ParseJSON(req, &sr)
	crep := model.BasicResponse{
		Status: false,
	}
	fmt.Println("SR in recover Pinned Token :", sr)
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
	fmt.Println("recoverDID in recoverPinnedToken :", recoverNodeDID)
	fmt.Println("Recovered Tokens in the Pinned Node :", recoveredTokens)
	fmt.Println("Err in recoveredTokens:", err)

	signFunc := signModule.InitDIDLite(recoverNodeDID, c.didDir, nil)
	fmt.Println("The directory in recoverPinnedToken", c.didDir)
	verified, err := signFunc.PvtVerify([]byte(recoverNodeDID), sr.Signature.Signature)
	fmt.Println("Verified in recoverPinnedToken", verified)
	fmt.Println("Err in recoverPinnedToken", err)
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

	fmt.Println("TIS is :", tis)
	crep.Status = true
	crep.Message = "Token Recovered Succesfully"
	crep.Result = tis
	return c.l.RenderJSON(req, &crep, http.StatusOK)
}

func mapToTokenInfo(m map[string]interface{}) contract.TokenInfo {
	tokenType, err := m["tokenType"].(json.Number)
	if err {
		fmt.Println("The error in tokenType is :", err)
	}
	tokenTypeInt64, err1 := tokenType.Int64()
	if err1 != nil {
		fmt.Println("The error in tokenTypeInt is :", err1)
	}

	fmt.Println("TokenType :", tokenType)
	fmt.Println("TokenType Int64 :", tokenTypeInt64)
	fmt.Println("Token Type int :", int(tokenTypeInt64))

	toknValue, err2 := m["tokenValue"].(json.Number)
	if err2 {
		fmt.Println("The error in tokenValue is :", err2)
	}
	tokenValueFloat64, err3 := toknValue.Float64()
	if err3 != nil {
		fmt.Println("The error in tokenValueFloat64 is :", err3)
	}
	fmt.Println("Token value :", toknValue)
	fmt.Println("Token value float64", tokenValueFloat64)

	return contract.TokenInfo{
		Token:      m["token"].(string),
		TokenType:  int(tokenTypeInt64),
		TokenValue: tokenValueFloat64,
		OwnerDID:   m["ownerDID"].(string),
		BlockID:    m["blockID"].(string),
	}
}
