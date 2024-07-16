package core

// import (
// 	"fmt"
// 	"net/http"
// 	"time"

// 	"github.com/rubixchain/rubixgoplatform/contract"
// 	"github.com/rubixchain/rubixgoplatform/core/model"
// 	"github.com/rubixchain/rubixgoplatform/core/unpledge"
// 	"github.com/rubixchain/rubixgoplatform/core/wallet"
// 	"github.com/rubixchain/rubixgoplatform/did"
// 	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"

// 	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
// )

// func (c *Core) selfTransferService() {
// 	c.l.AddRoute(APISelfTransfer, "POST", c.APISelfTransfer)
// }

// func (c *Core) didResponse(req *ensweb.Request, reqID string) *ensweb.Result {
// 	dc := c.GetWebReq(reqID)
// 	ch := <-dc.OutChan
// 	time.Sleep(time.Millisecond * 10)
// 	sr, ok := ch.(*did.SignResponse)
// 	if ok {
// 		return c.l.RenderJSON(req, sr, http.StatusOK)
// 	}
// 	br, ok := ch.(*model.BasicResponse)
// 	if ok {
// 		c.RemoveWebReq(reqID)
// 		br.Status = true
// 		return c.l.RenderJSON(req, br, http.StatusOK)
// 	}
// 	return c.l.RenderJSON(req, &model.BasicResponse{Status: false, Message: "Invalid response"}, http.StatusOK)
// }

// func (c *Core) APISelfTransfer(req *ensweb.Request) *ensweb.Result {
// 	resp := model.BasicResponse{
// 		Status: false,
// 	}

// 	if req.ID == "" {
// 		c.log.Debug("Request ID for API Self Transfer not set")
// 		req.ID = uuid.New().String()
// 	}

// 	var selfTransferReq model.SelfTransferRequest
// 	err := c.l.ParseJSON(req, &selfTransferReq)
// 	if err != nil {
// 		return c.l.RenderJSON(req, resp, http.StatusOK)
// 	}

// 	c.AddWebReq(req)
// 	go c.initSelfTransfer(req.ID, &selfTransferReq)
// 	return c.didResponse(req, req.ID)
// }

// func (c *Core) initSelfTransfer(reqID string, req *model.SelfTransferRequest) {
// 	br := c.selfTransfer(reqID, req)
// 	dc := c.GetWebReq(reqID)
// 	if dc == nil {
// 		c.log.Error("Failed to get did channels")
// 		return
// 	}
// 	dc.OutChan <- br
// }


// func (c *Core) selfTransfer(reqID string, rbtTransferRequest *model.RBTTransferRequest) *model.BasicResponse {
// 	st := time.Now()

// 	response := &model.BasicResponse{
// 		Status: false,
// 	}

// 	owner := rbtTransferRequest.Sender

// 	c.log.Debug("Initiating Self Trasfer for DID: ", owner)

// 	// Get all free tokens
// 	freeTokensOfOwner, err := c.w.GetFreeTokens(owner)
// 	if err != nil {
// 		response.Message = "failed to get free tokens of owner, error: " + err.Error()
// 		return response
// 	}
	
// 	// Get the transaction epoch for every token and chec
// 	var selfTransferredTokens []wallet.Token = make([]wallet.Token, 0)

// 	for _, token := range freeTokensOfOwner {
// 		tokenTransactionDetail, err := c.w.GetTransactionDetailsbyTransactionId(token.TransactionID)
// 		if err != nil {
// 			return response
// 		}

// 		if time.Now().Unix() - tokenTransactionDetail.Epoch > int64(unpledge.PledgePeriodInSeconds) {
// 			if err := c.w.LockToken(&token); err != nil {
// 				errMsg := fmt.Sprintf("selfTransfer: failed to lock tokens %v, exiting selfTransfer routine with error: %v", token.TokenID, err.Error())
// 				c.log.Error(errMsg)
// 				response.Message = errMsg
// 				return response
// 			}

// 			selfTransferredTokens = append(selfTransferredTokens, token)
// 		}
// 	}
	

// 	// --------------------------------

// 	// Get details of Tokens
// 	var ownedTokens []wallet.Token
// 	for _, token := range tokens {
// 		walletToken, err := c.w.ReadToken(token)
// 		if err != nil {
// 			response.Message = err.Error()
// 			return response
// 		}
// 		ownedTokens = append(ownedTokens, *walletToken)
// 	}
// 	defer c.w.ReleaseTokens(ownedTokens)

// 	c.log.Debug("1")
// 	// Lock the free tokens for processing
// 	for _, ownedToken := range ownedTokens {
// 		if err := c.w.LockToken(&ownedToken); err != nil {
// 			errMsg := fmt.Sprintf("selfTransfer: failed to lock tokens %v, exiting selfTransfer routine with error: %v", ownedToken.TokenID, err.Error())
// 			c.log.Error(errMsg)
// 			response.Message = errMsg
// 			return response
// 		}
// 	}

// 	selfPeerId := c.GetPeerID()
// 	selfAddress := selfPeerId + "." + ownerDID
// 	for i := range ownedTokens {
// 		c.w.Pin(ownedTokens[i].TokenID, wallet.OwnerRole, ownerDID, "TID-Not Generated", selfAddress, selfAddress, ownedTokens[i].TokenValue)
// 	}

// 	c.log.Debug("2")
// 	// Setup DID
// 	dc, err := c.SetupDID(reqID, ownerDID)
// 	if err != nil {
// 		response.Message = "Failed to setup DID, " + err.Error()
// 		c.log.Error(fmt.Sprintf("selfTransfer: failed to setup DID, error: %v", err.Error()))
// 		return response
// 	}
	
// 	// Create a Contract for the self transfer
// 	tokenInfos := make([]contract.TokenInfo, 0)
// 	totalTokenValue := getTotalValueFromTokens(ownedTokens)

// 	c.log.Debug("3")
// 	for _, freeToken := range ownedTokens {
// 		tokenTypeString := "rbt"
// 		if freeToken.TokenValue != 1 {
// 			tokenTypeString = "part"
// 		}

// 		tokenType := c.TokenType(tokenTypeString)
// 		latestTokenBlock := c.w.GetLatestTokenBlock(freeToken.TokenID, tokenType)
// 		if latestTokenBlock == nil {
// 			c.log.Error("failed to get latest block, invalid token chain")
// 			response.Message = "failed to get latest block, invalid token chain"
// 			return response
// 		}

// 		latestBlockID, err := latestTokenBlock.GetBlockID(freeToken.TokenID)
// 		if err != nil {
// 			c.log.Error("failed to get block id", "err", err)
// 			response.Message = "failed to get block id, " + err.Error()
// 			return response
// 		}
// 		tokenInfo := contract.TokenInfo{
// 			Token:      freeToken.TokenID,
// 			TokenType:  tokenType,
// 			TokenValue: freeToken.TokenValue,
// 			OwnerDID:   freeToken.DID,
// 			BlockID:    latestBlockID,
// 		}
// 		tokenInfos = append(tokenInfos, tokenInfo)
// 	}

// 	selfTransferContractType := &contract.ContractType{
// 		Type:       contract.SCRBTDirectType,
// 		PledgeMode: contract.POWPledgeMode,
// 		TotalRBTs:  totalTokenValue,
// 		TransInfo: &contract.TransInfo{
// 			SenderDID:   ownerDID,
// 			ReceiverDID: ownerDID,
// 			Comment:     "Self transfer at " + time.Now().String(),
// 			TransTokens: tokenInfos,
// 		},
// 		ReqID: reqID,
// 	}

// 	selfTransferContract := contract.CreateNewContract(selfTransferContractType)

// 	err = selfTransferContract.UpdateSignature(dc)
// 	if err != nil {
// 		c.log.Error(err.Error())
// 		response.Message = err.Error()
// 		return response
// 	}

// 	c.log.Debug("5")
// 	consensusRequest := &ConensusRequest{
// 		ReqID:          uuid.New().String(),
// 		Type:           2, // TODO: need to be decided
// 		SenderPeerID:   selfPeerId,
// 		ReceiverPeerID: selfPeerId,
// 		ContractBlock:  selfTransferContract.GetBlock(),
// 	}

// 	c.log.Debug("6")
// 	transactionDetails, _, err := c.initiateConsensus(consensusRequest, selfTransferContract, dc)
// 	if err != nil {
// 		c.log.Error("Consensus failed ", "err", err)
// 		response.Message = "Consensus failed " + err.Error()
// 		return response
// 	}
// 	transactionDetails.Amount = totalTokenValue
// 	et := time.Now()
// 	dif := et.Sub(st)
// 	transactionDetails.TotalTime = float64(dif.Milliseconds())
	
// 	// if err := c.w.AddTransactionHistory(transactionDetails); err != nil {
// 	// 	errMsg := fmt.Sprintf("Error occured while adding transaction details: %v", err)
// 	// 	c.log.Error(errMsg)
// 	// 	response.Message = errMsg
// 	// 	return response
// 	// }

// 	c.log.Info("Self Transfer finished successfully", "duration", dif, " trnxid", transactionDetails.TransactionID)
// 	response.Status = true

// 	return response
// }

// // func getTotalValueFromTokens(tokens []wallet.Token) float64 {
// // 	var totatValue float64 = 0.0

// // 	for _, token := range tokens {
// // 		totatValue += token.TokenValue
// // 	}

// // 	return totatValue
// // }