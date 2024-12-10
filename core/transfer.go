package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
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

func (c *Core) initiateRBTTransfer(reqID string, req *model.RBTTransferRequest) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}

	if req.Sender == req.Receiver {
		resp.Message = "Sender and receiver cannot be same"
		return resp
	}

	if !strings.Contains(req.Sender, ".") || !strings.Contains(req.Receiver, ".") {
		resp.Message = "Sender and receiver address should be of the format PeerID.DID"
		return resp
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
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}

	accountBalance, err := c.GetAccountInfo(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked or " + err.Error()
		return resp
	} else {
		if req.TokenCount > accountBalance.RBTAmount {
			c.log.Error(fmt.Sprint("Insufficient balance, account balance is ", accountBalance.RBTAmount, " trnx value is ", req.TokenCount))
			resp.Message = fmt.Sprint("Insufficient balance, account balance is ", accountBalance.RBTAmount, " trnx value is ", req.TokenCount)
			return resp
		}
	}

	tokensForTxn := make([]wallet.Token, 0)

	reqTokens, remainingAmount, err := c.GetRequiredTokens(did, req.TokenCount)
	if err != nil {
		c.w.ReleaseTokens(reqTokens)
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked or " + err.Error()
		return resp
	}
	if len(reqTokens) != 0 {
		tokensForTxn = append(tokensForTxn, reqTokens...)
	}
	//check if ther is enough tokens to do transfer
	// Get the required tokens from the DID bank
	// this method locks the token needs to be released or
	// removed once it done with the transfer
	if remainingAmount > 0 {
		wt, err := c.GetTokens(dc, did, remainingAmount)
		if err != nil {
			c.log.Error("Failed to get tokens", "err", err)
			resp.Message = "Insufficient tokens or tokens are locked"
			return resp
		}
		if len(wt) != 0 {
			tokensForTxn = append(tokensForTxn, wt...)
		}
	}

	var sumOfTokensForTxn float64
	for _, tokenForTxn := range tokensForTxn {
		sumOfTokensForTxn = sumOfTokensForTxn + tokenForTxn.TokenValue
		sumOfTokensForTxn = floatPrecision(sumOfTokensForTxn, MaxDecimalPlaces)
	}

	if sumOfTokensForTxn != req.TokenCount {
		c.log.Error(fmt.Sprint("Sum of Selected Tokens sum : ", sumOfTokensForTxn, " is not equal to trnx value : ", req.TokenCount))
		resp.Message = fmt.Sprint("Sum of Selected Tokens sum : ", sumOfTokensForTxn, " is not equal to trnx value : ", req.TokenCount)
		return resp
	}

	// release the locked tokens before exit
	defer c.w.ReleaseTokens(tokensForTxn)

	c.log.Info("*** Starting to pin tokens for transfer, token count :", len(tokensForTxn))
	for i := range tokensForTxn {
		c.w.Pin(tokensForTxn[i].TokenID, wallet.OwnerRole, did, "TID-Not Generated", req.Sender, req.Receiver, tokensForTxn[i].TokenValue)
	}
	c.log.Info("*** Sender token pinning completed")

	// Get the receiver & do sanity check
	p, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer p.Close()
	wta := make([]string, 0)
	for i := range tokensForTxn {
		wta = append(wta, tokensForTxn[i].TokenID)
	}

	tis := make([]contract.TokenInfo, 0)
	tokenListForExplorer := []Token{}
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
		tokenListForExplorer = append(tokenListForExplorer, Token{TokenHash: ti.Token, TokenValue: ti.TokenValue})
	}
	sct := &contract.ContractType{
		Type:       contract.SCRBTDirectType,
		PledgeMode: contract.POWPledgeMode,
		TotalRBTs:  req.TokenCount,
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			ReceiverDID: rdid,
			Comment:     req.Comment,
			TransTokens: tis,
		},
		ReqID: reqID,
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
		ReceiverPeerID: rpeerid,
		ContractBlock:  sc.GetBlock(),
	}
	c.log.Info("*** Initating consensus")
	td, _, pds, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}
	c.log.Info("*** Consensus completed")
	et := time.Now()
	dif := et.Sub(st)
	td.Amount = req.TokenCount
	td.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(td)
	etrans := &ExplorerRBTTrans{
		TokenHashes:    wta,
		TransactionID:  td.TransactionID,
		BlockHash:      strings.Split(td.BlockID, "-")[1],
		Network:        req.Type,
		SenderDID:      did,
		ReceiverDID:    rdid,
		Amount:         req.TokenCount,
		QuorumList:     extractQuorumDID(cr.QuorumList),
		PledgeInfo:     PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		TransTokenList: tokenListForExplorer,
		Comments:       req.Comment,
	}

	c.log.Info("*** Sending to explorer")
	c.ec.ExplorerRBTTransaction(etrans)
	c.log.Info("***  Sent to explorer")
	c.log.Info("Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
	resp.Status = true
	msg := fmt.Sprintf("Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
	resp.Message = msg
	return resp
}

func extractQuorumDID(quorumList []string) []string {
	var quorumListDID []string
	for _, quorum := range quorumList {
		parts := strings.Split(quorum, ".")
		if len(parts) > 1 {
			quorumListDID = append(quorumListDID, parts[1])
		} else {
			quorumListDID = append(quorumListDID, parts[0])
		}
	}
	return quorumListDID
}
