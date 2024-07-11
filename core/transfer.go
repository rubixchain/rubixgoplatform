package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/unpledge"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
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


func gatherTokensForTransaction(c *Core, req *model.RBTTransferRequest, dc did.DIDCrypto, isSelfRBTTransfer bool) ([]wallet.Token, error) {
	var tokensForTransfer []wallet.Token

	senderDID := req.Sender

	if !isSelfRBTTransfer {
		if req.TokenCount < MinTrnxAmt {
			return nil, fmt.Errorf("input transaction amount is less than minimum transaction amount")
		}
	
		decimalPlaces := strconv.FormatFloat(req.TokenCount, 'f', -1, 64)
		decimalPlacesStr := strings.Split(decimalPlaces, ".")
		if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > MaxDecimalPlaces {
			return nil, fmt.Errorf("transaction amount exceeds %v decimal places", MaxDecimalPlaces)
		}
	
		accountBalance, err := c.GetAccountInfo(senderDID)
		if err != nil {
			return nil, fmt.Errorf("insufficient tokens or tokens are locked or %v", err.Error())
		} else {
			if req.TokenCount > accountBalance.RBTAmount { 
				return nil, fmt.Errorf("insufficient balance, account balance is %v, trnx value is %v", accountBalance.RBTAmount, req.TokenCount)
			}
		}
	
		reqTokens, remainingAmount, err := c.GetRequiredTokens(senderDID, req.TokenCount)
		if err != nil {
			c.w.ReleaseTokens(reqTokens)
			return nil, fmt.Errorf("insufficient tokens or tokens are locked or %v", err.Error())
		}

		if len(reqTokens) != 0 {
			tokensForTransfer = append(tokensForTransfer, reqTokens...)
		}
		//check if ther is enough tokens to do transfer
		// Get the required tokens from the DID bank
		// this method locks the token needs to be released or
		// removed once it done with the transfer
		if remainingAmount > 0 {
			wt, err := c.GetTokens(dc, senderDID, remainingAmount)
			if err != nil {
				return nil, fmt.Errorf("insufficient tokens or tokens are locked or %v", err.Error())
			}
			if len(wt) != 0 {
				tokensForTransfer = append(tokensForTransfer, wt...)
			}
		}
	
		var sumOfTokensForTxn float64
		for _, tokenForTransfer := range tokensForTransfer {
			sumOfTokensForTxn = sumOfTokensForTxn + tokenForTransfer.TokenValue
			sumOfTokensForTxn = floatPrecision(sumOfTokensForTxn, MaxDecimalPlaces)
		}
	
		if sumOfTokensForTxn != req.TokenCount {
			return nil, fmt.Errorf("sum of Selected Tokens sum : %v is not equal to trnx value : %v", sumOfTokensForTxn, req.TokenCount)
		}

		return tokensForTransfer, nil
	} else {
		// Get all free tokens
		tokensOwnedBySender, err := c.w.GetFreeTokens(senderDID)
		if err != nil {
			if strings.Contains(err.Error(), "no records found") {
				return []wallet.Token{}, nil
			}
			return nil, fmt.Errorf("failed to get free tokens of owner, error: %v", err.Error())
		}
		
		// Get the transaction epoch for every token and chec
		for _, token := range tokensOwnedBySender {
			tokenTransactionDetail, err := c.w.GetTransactionDetailsbyTransactionId(token.TransactionID)
			if err != nil {
				return nil, fmt.Errorf("failed to get transaction details for trx hash: %v, err: %v", token.TransactionID, err)
			}

			if time.Now().Unix() - tokenTransactionDetail.Epoch > int64(unpledge.PledgePeriodInSeconds) {
				if err := c.w.LockToken(&token); err != nil {
					return nil, fmt.Errorf("failed to lock tokens %v, exiting selfTransfer routine with error: %v", token.TokenID, err.Error())
				}

				tokensForTransfer = append(tokensForTransfer, token)
			}
		}

		c.log.Debug("Tokens acquired for self transfer")
		return tokensForTransfer, nil
	}
}

func getContractType(reqID string, req *model.RBTTransferRequest, transTokenInfo []contract.TokenInfo, isSelfRBTTransfer bool) *contract.ContractType {
	if !isSelfRBTTransfer {
		return &contract.ContractType{
			Type:       contract.SCRBTDirectType,
			PledgeMode: contract.PeriodicPledgeMode,
			TotalRBTs:  req.TokenCount,
			TransInfo: &contract.TransInfo{
				SenderDID:   req.Sender,
				ReceiverDID: req.Receiver,
				Comment:     req.Comment,
				TransTokens: transTokenInfo,
			},
			ReqID: reqID,
		}
	} else {
		// Calculate the total value of self transfer RBT tokens
		var totalRBTValue float64
		for _, tokenInfo := range transTokenInfo {
			totalRBTValue += tokenInfo.TokenValue
		}

		return &contract.ContractType{
			Type:       contract.SCRBTDirectType,
			PledgeMode: contract.PeriodicPledgeMode,
			TotalRBTs:  totalRBTValue,
			TransInfo: &contract.TransInfo{
				SenderDID:   req.Sender,
				ReceiverDID: req.Receiver,
				Comment:     "Self Transfer at " + time.Now().String(),
				TransTokens: transTokenInfo,
			},
			ReqID: reqID,
		}
	}
}

func getConsensusRequest(consensusRequestType int, senderPeerID string, receiverPeerID string, contractBlock []byte, transactionEpoch int, isSelfTransfer bool) *ConensusRequest {
	var consensusRequest *ConensusRequest = &ConensusRequest{
		ReqID:            uuid.New().String(),
		Type:             consensusRequestType,
		SenderPeerID:     senderPeerID,
		ReceiverPeerID:   receiverPeerID,
		ContractBlock:    contractBlock,
		TransactionEpoch: transactionEpoch,
	}
	
	if isSelfTransfer {
		consensusRequest.Mode = SelfTransferMode
	}

	return consensusRequest
}

func (c *Core) initiateRBTTransfer(reqID string, req *model.RBTTransferRequest) *model.BasicResponse {
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}

	senderDID := req.Sender
	receiverdid := req.Receiver

	// This flag indicates if the call is made for Self Transfer or general token transfer
	isSelfRBTTransfer := senderDID == receiverdid

	dc, err := c.SetupDID(reqID, senderDID)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}

	tokensForTxn, err := gatherTokensForTransaction(c, req, dc, isSelfRBTTransfer)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	// In case of self transfer
	if len(tokensForTxn) == 0 && isSelfRBTTransfer {
		resp.Status = true
		resp.Message = "No tokens present for self transfer"
		return resp
	}

	// release the locked tokens before exit
	defer c.w.ReleaseTokens(tokensForTxn)

	for i := range tokensForTxn {
		c.w.Pin(tokensForTxn[i].TokenID, wallet.OwnerRole, senderDID, "TID-Not Generated", req.Sender, req.Receiver, tokensForTxn[i].TokenValue)
	}

	// Get the receiver & do sanity check
	var rpeerid string = ""
	if !isSelfRBTTransfer {
		rpeerid := c.w.GetPeerID(receiverdid)
		if rpeerid == "" {
			c.log.Error("Peer ID not found", "did", receiverdid)
			resp.Message = "invalid address, Peer ID not found"
			return resp
		}

		p, err := c.getPeer(req.Receiver, senderDID)
		if err != nil {
			resp.Message = "Failed to get receiver peer, " + err.Error()
			return resp
		}
		defer p.Close()
	}

	wta := make([]string, 0)
	for i := range tokensForTxn {
		wta = append(wta, tokensForTxn[i].TokenID)
	}

	tis := make([]contract.TokenInfo, 0)
	for i := range tokensForTxn {
		tts := "rbt"
		if tokensForTxn[i].TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(tokensForTxn[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}

		bid, err := blk.GetBlockID(tokensForTxn[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		ti := contract.TokenInfo{
			Token:      tokensForTxn[i].TokenID,
			TokenType:  tt,
			TokenValue: tokensForTxn[i].TokenValue,
			OwnerDID:   tokensForTxn[i].DID,
			BlockID:    bid,
		}
		tis = append(tis, ti)
	}

	contractType := getContractType(reqID, req, tis, isSelfRBTTransfer)
	sc := contract.CreateNewContract(contractType)

	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	
	cr := getConsensusRequest(req.Type, c.peerID, rpeerid, sc.GetBlock(), txEpoch, isSelfRBTTransfer)

	td, _, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	td.Amount = req.TokenCount
	td.TotalTime = float64(dif.Milliseconds())

	if err := c.w.AddTransactionHistory(td); err != nil {
		errMsg := fmt.Sprintf("Error occured while adding transaction details: %v", err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	/* blockHash, err := extractHash(td.BlockID)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	} */
	etrans := &ExplorerTrans{
		TID:         td.TransactionID,
		SenderDID:   senderDID,
		ReceiverDID: receiverdid,
		Amount:      req.TokenCount,
		TrasnType:   req.Type,
		TokenIDs:    wta,
		QuorumList:  cr.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
		//BlockHash:   blockHash,
	}
	c.ec.ExplorerTransaction(etrans)
	c.log.Info("Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
	resp.Status = true
	msg := fmt.Sprintf("Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
	resp.Message = msg
	return resp
}

func extractHash(input string) (string, error) {
	values := strings.Split(input, "-")
	if len(values) != 2 {
		return "", fmt.Errorf("invalid format: %s", input)
	}
	return values[1], nil
}
