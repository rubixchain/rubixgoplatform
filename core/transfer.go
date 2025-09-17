package core

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	RubixTxnTopic string = "rubix_txns"
)

type ConsensusReturns struct {
	TxnDetails    *model.TransactionDetails
	PledgeDetails *PledgeDetails
	Msg           string
}

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
		if req.TokenCount < MinDecimalValue(MaxDecimalPlaces) {
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

		reqTokens, remainingAmount, err := c.GetRequiredTokens(senderDID, req.TokenCount, RBTTransferMode)
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
			wt, err := c.GetTokens(dc, senderDID, remainingAmount, RBTTransferMode)
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
			// Nodes running old version of rubixgoplatform will not have their TransactionID column of Tokens's table populated
			// And hence should be skipped from Self Transfer
			if token.TransactionID == "" {
				continue
			}
			tokenTransactionDetail, err := c.w.GetTransactionDetailsbyTransactionId(token.TransactionID)
			if err != nil {
				return nil, fmt.Errorf("failed to get transaction details for trx hash: %v, err: %v", token.TransactionID, err)
			}

			if time.Now().Unix()-tokenTransactionDetail.Epoch > int64(pledgePeriodInSeconds) {
				if err := c.w.LockToken(&token); err != nil {
					return nil, fmt.Errorf("failed to lock tokens %v, exiting selfTransfer routine with error: %v", token.TokenID, err.Error())
				}

				tokensForTransfer = append(tokensForTransfer, token)
			}
		}

		if len(tokensForTransfer) > 0 {
			c.log.Debug("Tokens acquired for self transfer")
		}
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
		rpeerid = c.w.GetPeerID(receiverdid)
		if rpeerid == "" {
			// Check if DID is present in the DIDTable as the receiver might be part of the current node
			didDetails, err := c.w.GetDID(receiverdid)
			if err != nil {
				if strings.Contains(err.Error(), "no records found") {
					c.log.Error("receiver Peer ID not found", "did", receiverdid)
					resp.Message = "invalid address, receiver Peer ID not found"
					//return resp
				} else {
					c.log.Error(fmt.Sprintf("Error occurred while fetching DID info from DIDTable for DID: %v, err: %v", receiverdid, err))
					resp.Message = fmt.Sprintf("Error occurred while fetching DID info from DIDTable for DID: %v, err: %v", receiverdid, err)
					return resp
				}
			}

			if didDetails == nil {
				receiverPeerInfo, err := c.GetPeerDIDInfo(receiverdid)
				if err != nil {
					c.log.Error("receiver Peer ID not found in network", "did", receiverdid)
					resp.Message = "invalid address, receiver Peer ID not found"
					return resp
				}
				rpeerid = receiverPeerInfo.PeerID
			} else {
				// Set the receiverPeerID to self Peer ID
				rpeerid = c.peerID
			}
		} else {
			p, err := c.getPeer(req.Receiver)
			if err != nil {
				resp.Message = "Failed to get receiver peer, " + err.Error()
				return resp
			}
			if p != nil {
				p.Close()
			}
		}
	}
	wta := make([]string, 0)
	for i := range tokensForTxn {
		wta = append(wta, tokensForTxn[i].TokenID)
	}

	tis := make([]contract.TokenInfo, 0)
	tokenListForExplorer := []Token{}
	// transTokensSyncInfo := make(map[string]GenesisAndLatestBlocks, len(tokensForTxn))

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
			TokenValue: floatPrecision(tokensForTxn[i].TokenValue, MaxDecimalPlaces),
			OwnerDID:   tokensForTxn[i].DID,
			BlockID:    bid,
		}
		tis = append(tis, ti)
		tokenListForExplorer = append(tokenListForExplorer, Token{TokenHash: ti.Token, TokenValue: ti.TokenValue})

		// genesis := c.w.GetGenesisTokenBlock(ti.Token, ti.TokenType)
		// genesisNLatestBlocks := GenesisAndLatestBlocks{
		// 	GenesisBlock: genesis.GetBlock(),
		// }
		// if c.TokenType(PartString) == ti.TokenType {
		// 	// get parent token id
		// 	parentToken, _, err := genesis.GetParentDetials(ti.Token)
		// 	if err != nil {
		// 		c.log.Error("failed to fetch parent token detials", "err", err, "token", ti.Token)
		// 		resp.Message = fmt.Sprintf("failed to fetch parent token detials, err : %v, token : %v", err, ti.Token)
		// 		return resp
		// 	}
		// 	// get parent token type
		// 	b, err := c.getFromIPFS(parentToken)
		// 	if err != nil {
		// 		c.log.Error("failed to get parent token details from ipfs", "err", err, "parent token", parentToken)
		// 		resp.Message = fmt.Sprintf("failed to get parent token details from ipfs", "err", err, "parent token", parentToken)
		// 		return resp
		// 	}
		// 	_, iswholeToken, _ := token.CheckWholeToken(string(b), c.testNet)

		// 	parentTokenType := token.RBTTokenType
		// 	if !iswholeToken {
		// 		blk := util.StrToHex(string(b))
		// 		rb, err := rac.InitRacBlock(blk, nil)
		// 		if err != nil {
		// 			c.log.Error("invalid token, invalid rac block of parent token", "err", err)
		// 			resp.Message = "failed to get parent token info for token " + ti.Token
		// 			return resp
		// 		}
		// 		parentTokenType = rac.RacType2TokenType(rb.GetRacType())
		// 	}

		// 	// get parent genesis and latest blocks
		// 	parentGenesis := c.w.GetGenesisTokenBlock(parentToken, parentTokenType)
		// 	parentLatest := c.w.GetLatestTokenBlock(parentToken, parentTokenType)
		// 	genesisNLatestBlocks.ParentGenesisBlock = parentGenesis.GetBlock()
		// 	genesisNLatestBlocks.ParentLatestBlock = parentLatest.GetBlock()
		// } else {
		// 	if genesis != blk { // TODO : try using block id or number to compare
		// 		genesisNLatestBlocks.LatestBlock = blk.GetBlock()
		// 	}
		// }
	}

	//check if sender has previous block pledged quorums' details
	for _, tokeninfo := range tis {
		b := c.w.GetLatestTokenBlock(tokeninfo.Token, tokeninfo.TokenType)
		//check if the transaction in prev block involved any quorums
		switch b.GetTransType() {
		case block.TokenGeneratedType:
			continue
		case block.TokenBurntType:
			c.log.Error("token is burnt, can't transfer anymore; token:", tokeninfo.Token)
			resp.Message = "token is burnt, can't transfer anymore"
			return resp
		case block.TokenTransferredType:
			//fetch all the pledged quorums, if the transaction involved quorums
			prevQuorums, _ := b.GetSigner()

			for _, prevQuorum := range prevQuorums {
				//check if the sender has prev pledged quorum's did type; if not, fetch it from the prev sender
				fmt.Println("Checking if the sender has previous block pledged quorum's did type")
				prevQuorumInfo, err := c.GetPeerDIDInfo(prevQuorum)
				if err != nil {
					if strings.Contains(err.Error(), "retry") {
						c.AddPeerDetails(*prevQuorumInfo)
					}
				}
				if prevQuorumInfo == nil || *prevQuorumInfo.DIDType == -1 {
					//if a signle pledged quorum is also not found, we can assume that other pledged quorums will also be not found,
					//and request prev sender to share details of all the pledged quorums, and thus breaking the for loop
					break
				}

			}
		}
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
	resultChan := make(chan *model.BasicResponse, 1)

	// Start the transaction in a goroutine
	go func() {
		td, _, pds, consError := c.initiateConsensus(cr, sc, dc)
		if consError != nil {
			resp.Message = fmt.Sprintf("Consensus failed " + consError.Error())
			resp.Status = false
			resultChan <- resp
			return
		}
		et := time.Now()
		dif := et.Sub(st)
		if isSelfRBTTransfer {
			var amt float64 = 0
			for _, tknInfo := range tis {
				amt += tknInfo.TokenValue
			}
			td.Amount = amt
		} else {
			td.Amount = req.TokenCount
		}
		td.TotalTime = float64(dif.Milliseconds())

		if td.TotalTime < 0.00 {
			td.TotalTime = 0.00
		}

		if err := c.w.AddTransactionHistory(td); err != nil {
			errMsg := fmt.Sprintf("Error occured while adding transaction details: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg

			return
		}
		etrans := &ExplorerRBTTrans{
			TokenHashes:    wta,
			TransactionID:  td.TransactionID,
			BlockHash:      strings.Split(td.BlockID, "-")[1],
			Network:        req.Type,
			SenderDID:      senderDID,
			ReceiverDID:    receiverdid,
			Amount:         req.TokenCount,
			QuorumList:     extractQuorumDID(cr.QuorumList),
			PledgeInfo:     PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
			TransTokenList: tokenListForExplorer,
			Comments:       req.Comment,
		}

		c.log.Info("Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
		resp.Status = true
		msg := fmt.Sprintf("Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
		resp.Message = msg
		if strings.Contains(resp.Message, "with transaction id") {
			if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
				resp.Result = txID
			}
		}
		c.ec.ExplorerRBTTransaction(etrans)

		// Send final transaction completion response if not already timed out
		select {
		case resultChan <- resp:
			// Successfully sent to resultChan
		default:
			// If no one is listening (already timed out), just log and exit
			c.log.Debug("Transaction completed but resultChan is not being read anymore")
		}
	}()

	select {
	case result := <-resultChan:
		// Transaction completed within 40s or failed
		c.log.Debug("transaction completed before 20 secs")
		return result

	case <-time.After(20 * time.Second):
		// Timeout occurred, return Transaction ID only
		c.log.Debug("transaction still processing with txn id ", cr.TransactionID)

		msg := fmt.Sprintf("Transaction is still processing, with transaction id %v ", cr.TransactionID)
		resp.Message = msg
		if strings.Contains(resp.Message, "with transaction id") {
			if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
				resp.Result = txID
			}
		}
		resp.Status = true
		return resp
	}
}

//Functions to initiate PinRBT

func (c *Core) InitiatePinRBT(reqID string, req *model.RBTPinRequest) {
	br := c.initiatePinRBT(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiatePinRBT(reqID string, req *model.RBTPinRequest) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}

	if req.Sender == req.PinningNode {
		resp.Message = "Sender and receiver cannot be same"
		return resp
	}
	did := req.Sender
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	pinningNodeDID := req.PinningNode
	pinningNodepeerid := c.w.GetPeerID(pinningNodeDID)
	if pinningNodepeerid == "" {
		c.log.Error("Peer ID not found", "did", pinningNodeDID)
		resp.Message = "invalid address, Peer ID not found"
		return resp
	}

	// Handle the case where TokenCount is 0
	if req.TokenCount == 0 {
		reqTokens, err := c.w.GetAllFreeToken(did)
		if err != nil {
			c.w.ReleaseTokens(reqTokens)
			c.log.Error("Failed to get tokens", "err", err)
			resp.Message = "Insufficient tokens or tokens are locked or " + err.Error()
			return resp
		}

		tokensForTxn := make([]wallet.Token, 0)
		if len(reqTokens) != 0 {
			tokensForTxn = append(tokensForTxn, reqTokens...)
		}

		return c.completePinning(st, reqID, req, did, pinningNodeDID, pinningNodepeerid, tokensForTxn, resp, dc)
	}

	if req.TokenCount < MinDecimalValue(MaxDecimalPlaces) {
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
	accountBalance, err := c.GetAccountInfo(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked or " + err.Error()
		return resp
	} else {
		if req.TokenCount > accountBalance.RBTAmount {
			c.log.Error(fmt.Sprint("The requested amount not available for pinning ", req.TokenCount, " Token value available for pinning : ", accountBalance.RBTAmount))
			resp.Message = fmt.Sprint("The requested amount not available for pinning ", req.TokenCount, " Token value available for pinning : ", accountBalance.RBTAmount)
			return resp
		}
	}

	tokensForTxn := make([]wallet.Token, 0)

	reqTokens, remainingAmount, err := c.GetRequiredTokens(did, req.TokenCount, PinningServiceMode)
	if err != nil {
		c.w.ReleaseTokens(reqTokens)
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked or " + err.Error()
		return resp
	}
	if len(reqTokens) != 0 {
		tokensForTxn = append(tokensForTxn, reqTokens...)
	}

	if remainingAmount > 0 {
		wt, err := c.GetTokens(dc, did, remainingAmount, PinningServiceMode)
		if err != nil {
			c.log.Error("Failed to get tokens", "err", err)
			resp.Message = "Insufficient tokens or tokens are locked"
			return resp
		}
		if len(wt) != 0 {
			tokensForTxn = append(tokensForTxn, wt...)
		}
	}

	return c.completePinning(st, reqID, req, did, pinningNodeDID, pinningNodepeerid, tokensForTxn, resp, dc)
}

func (c *Core) completePinning(st time.Time, reqID string, req *model.RBTPinRequest, did, pinningNodeDID, pinningNodepeerid string, tokensForTxn []wallet.Token, resp *model.BasicResponse, dc did.DIDCrypto) *model.BasicResponse {
	var sumOfTokensForTxn float64
	for _, tokenForTxn := range tokensForTxn {
		sumOfTokensForTxn = sumOfTokensForTxn + tokenForTxn.TokenValue
		sumOfTokensForTxn = floatPrecision(sumOfTokensForTxn, MaxDecimalPlaces)
	}
	// release the locked tokens before exit
	defer c.w.ReleaseTokens(tokensForTxn)

	for i := range tokensForTxn {
		c.w.Pin(tokensForTxn[i].TokenID, wallet.PinningRole, did, "TID-Not Generated", req.Sender, req.PinningNode, tokensForTxn[i].TokenValue)
	}
	p, err := c.getPeer(req.PinningNode)
	if err != nil {
		resp.Message = "Failed to get pinning peer, " + err.Error()
		return resp
	}
	defer p.Close()

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
		//OwnerDID will be the same as the sender, so that ownership is not changed.
		ti := contract.TokenInfo{
			Token:      tokensForTxn[i].TokenID,
			TokenType:  tt,
			TokenValue: floatPrecision(tokensForTxn[i].TokenValue, MaxDecimalPlaces),
			OwnerDID:   did,
			BlockID:    bid,
		}

		tis = append(tis, ti)
	}
	sct := &contract.ContractType{
		Type:       contract.SCRBTDirectType,
		PledgeMode: contract.PeriodicPledgeMode,
		TotalRBTs:  req.TokenCount,
		TransInfo: &contract.TransInfo{
			SenderDID:      did,
			PinningNodeDID: pinningNodeDID,
			Comment:        req.Comment,
			TransTokens:    tis,
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
		ReqID:             uuid.New().String(),
		Type:              req.Type,
		SenderPeerID:      c.peerID,
		PinningNodePeerID: pinningNodepeerid,
		ContractBlock:     sc.GetBlock(),
		Mode:              PinningServiceMode,
	}
	td, _, pds, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	td.Amount = req.TokenCount
	td.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(td)
	// etrans := &ExplorerTrans{
	// 	TID:         td.TransactionID,
	// 	SenderDID:   did,
	// 	ReceiverDID: pinningNodeDID,
	// 	Amount:      req.TokenCount,
	// 	TrasnType:   req.Type,
	// 	TokenIDs:    wta,
	// 	QuorumList:  cr.QuorumList,
	// 	TokenTime:   float64(dif.Milliseconds()),
	// } Remove comments
	etrans := &ExplorerRBTTrans{
		TokenHashes:   wta,
		TransactionID: td.TransactionID,
		BlockHash:     strings.Split(td.BlockID, "-")[1],
		Network:       req.Type,
		SenderDID:     did,
		ReceiverDID:   pinningNodeDID,
		Amount:        req.TokenCount,
		QuorumList:    extractQuorumDID(cr.QuorumList),
		PledgeInfo:    PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		Comments:      req.Comment,
	}
	c.ec.ExplorerRBTTransaction(etrans)
	c.log.Info("Pinning finished successfully", "duration", dif, " trnxid", td.TransactionID)
	resp.Status = true
	msg := fmt.Sprintf("Pinning finished successfully in %v with trnxid %v", dif, td.TransactionID)
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

// SubscribeTxnSetup subscribes to the topic rubix_txns
func (c *Core) SubscribeTxnSetup() {
	topic := RubixTxnTopic
	err := c.ps.SubscribeTopic(topic, c.TxnCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe NFT", topic)
		return
	}
	c.log.Info("Subscribing " + topic + " is successful")
}

// // TxnCallBack receives the published events with topic rubix_txns and processes them
// func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {
// 	var newEvent model.PubSubTxnInfo
// 	err := json.Unmarshal(data, &newEvent)
// 	if err != nil {
// 		c.log.Error("Failed to parse published event", "err", err)
// 		return
// 	}
// 	c.log.Info("Recieved Update on txn " + newEvent.TxnID)

// 	receiverDid := newEvent.ReceiverDID
// 	// tokenType := newEvent.TokenType

// 	// initiate block
// 	txnBlock := block.InitBlock(newEvent.TxnBlock, nil)
// 	currentOwner := txnBlock.GetOwner()

// 	// get tokens list from block, do sanity check, and add block to the token chain(s)
// 	tokensList := txnBlock.GetTransTokens()
// 	switch newEvent.TxnMode {
// 	case RBTTransferMode, FTTransferMode:
// 		// sanity check of all tokens in list
// 		for _, tokenId := range tokensList {

// 			// skip txn sanity check, if txn type is token generated type
// 			if newEvent.TxnType == block.TokenGeneratedType {
// 				if currentOwner != newEvent.PublisherDID {
// 					c.log.Error("txn callback: publisher DID is not same as the owner of token extract from its previous token block")
// 					continue
// 				}

// 				// add token to sqlite db
// 				err := c.AddTokenToRespectiveTable(tokenId, newEvent.TxnID, newEvent.PublisherDID, newEvent.TxnMode, txnBlock)
// 				if err != nil {
// 					errMsg := fmt.Sprintf("failed to add token : %v, to tokensTable, error : %v ", tokenId, err)
// 					c.log.Error(errMsg)
// 				}
// 				// add block to the token chain
// 				c.w.AddFullNodeTokenBlock(tokenId, txnBlock)
// 				continue
// 			}

// 			// currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
// 			// if err != nil {
// 			// 	errMsg := fmt.Sprintf("failed to get current block number for token chain : %v", tokenId)
// 			// 	c.log.Error(errMsg)
// 			// 	return
// 			// }

// 			// // fetch latest block from LDB and check if the publisher is the token owner in the last block
// 			// tokenType := txnBlock.GetTokenType(tokenId)
// 			// latestTokenBlock := c.w.GetLatestTokenBlock(tokenId, tokenType)
// 			// if latestTokenBlock == nil {
// 			// 	// TODO : sync the token chain if not there
// 			// 	c.log.Error(fmt.Sprintf("failed to get the latest block for token: %v", tokenId))
// 			// 	continue
// 			// }

// 			// // make sure there is no missing blocks, by comparing prev and current block numbers
// 			// latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
// 			// if err != nil {
// 			// 	errMsg := fmt.Sprintf("failed to get latest block number for token chain : %v", tokenId)
// 			// 	c.log.Error(errMsg)
// 			// 	return
// 			// }

// 			// // check if the fullnode is updated with latest blocks
// 			// if latestBlockNumber+1 != currentBlockNumber {
// 			// 	errMsg := fmt.Sprintf("missing block for token chain : %v, latest block number is : %v and received block number is : %v", tokenId, latestBlockNumber, currentBlockNumber)
// 			// 	c.log.Error(errMsg)
// 			// 	// TODO : handle all tokens with missing blocks
// 			// 	return
// 			// }

// 			// previousOwner := latestTokenBlock.GetOwner()

// 			// // Sanity check
// 			// if previousOwner != newEvent.PublisherDID {
// 			// 	c.log.Error("txn callback: publisher DID is not same as the owner of token extracted from its previous token block")
// 			// 	return
// 			// }
// 			// if receiverDid != "" {
// 			// 	// Sanity check: In case of transfer NFT, it is always expected that receiver DID
// 			// 	// will always be same as the onwer (extracted from the latest NFT block)
// 			// 	if currentOwner != receiverDid {
// 			// 		c.log.Error("txn callback: reciever DID is not same as the owner of NFT extracted from received token block")
// 			// 		return
// 			// 	}
// 			// }

// 			// add token to sqlite db
// 			err := c.AddTokenToRespectiveTable(tokenId, newEvent.TxnID, receiverDid, newEvent.TxnMode, txnBlock)
// 			if err != nil {
// 				errMsg := fmt.Sprintf("failed to add token : %v, to tokensTable, error : %v ", tokenId, err)
// 				c.log.Error(errMsg)
// 				continue
// 			}
// 			// add block to the token chain
// 			err = c.w.AddFullNodeTokenBlock(tokenId, txnBlock)
// 			if err != nil {
// 				errMsg := fmt.Sprintf("failed to add block to token chain: %v, err : %v", tokenId, err)
// 				c.log.Error(errMsg)
// 				continue
// 			}
// 			c.log.Info("Transaction added to token-chain")
// 		}

// 	case SmartContractDeployMode, SmartContractExecuteMode, NFTDeployMode, NFTExecuteMode:
// 		tokenId := tokensList[0]

// 		// add token to sqlite db
// 		err := c.AddTokenToRespectiveTable(tokenId, newEvent.TxnID, txnBlock.GetDeployerDID(), newEvent.TxnMode, txnBlock)
// 		if err != nil {
// 			errMsg := fmt.Sprintf("failed to add token : %v, to tokensTable, error : %v ", tokenId, err)
// 			c.log.Error(errMsg)
// 		}

// 		// skip txn sanity check, if txn type is token generated type
// 		if newEvent.TxnType == block.TokenGeneratedType {
// 			if currentOwner != newEvent.PublisherDID {
// 				c.log.Error("txn callback: publisher DID is not same as the owner of token extract from its previous token block")
// 				return
// 			}
// 			// add block to the token chain
// 			c.w.AddFullNodeTokenBlock(tokenId, txnBlock)
// 		} else {

// 			// fetch latest block from LDB and check if the publisher is the token owner in the last block
// 			tokenType := txnBlock.GetTokenType(tokenId)
// 			latestTokenBlock := c.w.GetLatestTokenBlock(tokenId, tokenType)
// 			if latestTokenBlock == nil {
// 				// TODO : sync the token chain if not there
// 				c.log.Error(fmt.Sprintf("failed to get the latest block for token: %v", tokenId))
// 				return
// 			}

// 			// make sure there is no missing blocks, by comparing prev and current block numbers
// 			latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
// 			if err != nil {
// 				errMsg := fmt.Sprintf("failed to get latest block number for token chain : %v", tokenId)
// 				c.log.Error(errMsg)
// 			}
// 			currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
// 			if err != nil {
// 				errMsg := fmt.Sprintf("failed to get current block number for token chain : %v", tokenId)
// 				c.log.Error(errMsg)
// 			}

// 			// check if the fullnode is updated with latest blocks
// 			if latestBlockNumber+1 != currentBlockNumber {
// 				errMsg := fmt.Sprintf("missing block for token chain : %v, latest block number is : %v and received block number is : %v", tokenId, latestBlockNumber, currentBlockNumber)
// 				c.log.Error(errMsg)
// 				// TODO : handle all tokens with missing blocks
// 				return
// 			} else {
// 				previousOwner := latestTokenBlock.GetOwner()

// 				// Sanity check
// 				if previousOwner != newEvent.PublisherDID {
// 					c.log.Error("txn callback: publisher DID is not same as the owner of token extract from its previous token block")
// 					return
// 				}

// 				// add block to the token chain
// 				err = c.w.AddFullNodeTokenBlock(tokenId, txnBlock)
// 				if err != nil {
// 					errMsg := fmt.Sprintf("failed to add block to token chain: %v, err : %v", tokenId, err)
// 					c.log.Error(errMsg)
// 					return
// 				}
// 				c.log.Info("Transaction added to token-chain")
// 			}
// 		}
// 	}
// }

// TxnCallBack receives the published events with topic rubix_txns and processes them
func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {
	logger := c.log.With("peer_id", peerID, "topic", topic)

	// Parse incoming event
	event, err := c.parseTransactionEvent(data, logger)
	if err != nil {
		logger.Error("Failed to parse transaction event", "error", err)
		return
	}

	logger = logger.With("txn_id", event.TxnID, "txn_mode", event.TxnMode, "txn_type", event.TxnType)
	logger.Info("Received transaction update")

	// Initialize and validate transaction block
	txnBlock, err := c.initializeTransactionBlock(event, logger)
	if err != nil {
		logger.Error("Failed to initialize transaction block", "error", err)
		return
	}

	// Process transaction based on mode
	if err := c.processTransactionByMode(event, txnBlock, logger); err != nil {
		logger.Error("Failed to process transaction", "error", err)
		return
	}

	logger.Info("Transaction processed successfully")
}

// parseTransactionEvent extracts and validates the transaction event from raw data
func (c *Core) parseTransactionEvent(data []byte, logger logger.Logger) (*model.PubSubTxnInfo, error) {
	var event model.PubSubTxnInfo
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction event: %w", err)
	}

	// Validate required fields
	// if event.TxnID == "" {
	// 	return nil, fmt.Errorf("transaction ID is required")
	// }
	if event.PublisherDID == "" {
		return nil, fmt.Errorf("publisher DID is required")
	}

	return &event, nil
}

// initializeTransactionBlock creates and validates the transaction block
func (c *Core) initializeTransactionBlock(event *model.PubSubTxnInfo, logger logger.Logger) (*block.Block, error) {
	txnBlock := block.InitBlock(event.TxnBlock, nil)
	if txnBlock == nil {
		return nil, fmt.Errorf("failed to initialize transaction block")
	}

	return txnBlock, nil
}

// processTransactionByMode routes transaction processing based on transaction mode
func (c *Core) processTransactionByMode(event *model.PubSubTxnInfo, txnBlock *block.Block, logger logger.Logger) error {
	switch event.TxnMode {
	case RBTTransferMode, FTTransferMode:
		return c.processTransferTransaction(event, txnBlock, logger)
	case SmartContractDeployMode, SmartContractExecuteMode, NFTDeployMode, NFTExecuteMode:
		return c.processContractTransaction(event, txnBlock, logger)
	default:
		return fmt.Errorf("unsupported transaction mode: %v", event.TxnMode)
	}
}

// processTransferTransaction handles RBT and FT transfer transactions
func (c *Core) processTransferTransaction(event *model.PubSubTxnInfo, txnBlock *block.Block, logger logger.Logger) error {
	tokensList := txnBlock.GetTransTokens()

	var errors []error
	successCount := 0

	for _, tokenId := range tokensList {
		if err := c.processTransferToken(event, txnBlock, tokenId, logger); err != nil {
			logger.Error("Failed to process transfer token", "token_id", tokenId, "error", err)
			errors = append(errors, fmt.Errorf("token %s: %w", tokenId, err))
			continue
		}
		successCount++
	}

	c.log.Info("Transfer transaction processing completed",
		"total_tokens", len(tokensList),
		"successful", successCount,
		"failed", len(errors))

	// Return error if any tokens failed (could be adjusted based on business requirements)
	if len(errors) > 0 {
		return fmt.Errorf("failed to process %d tokens: %v", len(errors), errors)
	}

	return nil
}

// processTransferToken handles individual token processing for transfers
func (c *Core) processTransferToken(event *model.PubSubTxnInfo, txnBlock *block.Block, tokenId string, logger logger.Logger) error {
	currentOwner := txnBlock.GetOwner()

	// Handle token generation type transactions
	if event.TxnType == block.TokenGeneratedType {
		return c.processTokenGeneration(event, txnBlock, tokenId, currentOwner, logger)
	}

	// For regular transfers, perform validation (currently commented out in original)
	// TODO: Implement proper validation logic when ready

	// Add token to database
	if err := c.AddTokenToRespectiveTable(tokenId, event.TxnID, event.ReceiverDID, event.TxnMode, txnBlock); err != nil {
		return fmt.Errorf("failed to add token to database: %w", err)
	}

	// Add block to token chain
	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add block to token chain: %w", err)
	}

	return nil
}

// processTokenGeneration handles token generation transactions
func (c *Core) processTokenGeneration(event *model.PubSubTxnInfo, txnBlock *block.Block, tokenId string, currentOwner string, logger logger.Logger) error {
	// Validate publisher is the owner
	if currentOwner != event.PublisherDID {
		return fmt.Errorf("publisher DID mismatch: expected %s, got %s", event.PublisherDID, currentOwner)
	}

	// Add token to database
	if err := c.AddTokenToRespectiveTable(tokenId, event.TxnID, event.PublisherDID, event.TxnMode, txnBlock); err != nil {
		return fmt.Errorf("failed to add generated token to database: %w", err)
	}

	// Add block to token chain
	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add generated token block to chain: %w", err)
	}

	return nil
}

// processContractTransaction handles smart contract and NFT transactions
func (c *Core) processContractTransaction(event *model.PubSubTxnInfo, txnBlock *block.Block, logger logger.Logger) error {
	tokensList := txnBlock.GetTransTokens()
	if len(tokensList) == 0 {
		return fmt.Errorf("no tokens found in contract transaction")
	}

	tokenId := tokensList[0]

	// Add token to database first
	if err := c.AddTokenToRespectiveTable(tokenId, event.TxnID, txnBlock.GetDeployerDID(), event.TxnMode, txnBlock); err != nil {
		return fmt.Errorf("failed to add contract token to database: %w", err)
	}

	// Handle token generation vs regular contract execution
	if event.TxnType == block.TokenGeneratedType {
		return c.processContractTokenGeneration(event, txnBlock, tokenId, logger)
	}

	return c.processRegularContractTransaction(event, txnBlock, tokenId, logger)
}

// processContractTokenGeneration handles contract token generation
func (c *Core) processContractTokenGeneration(event *model.PubSubTxnInfo, txnBlock *block.Block, tokenId string, logger logger.Logger) error {
	currentOwner := txnBlock.GetOwner()
	if currentOwner != event.PublisherDID {
		return fmt.Errorf("publisher DID mismatch for contract token generation: expected %s, got %s", event.PublisherDID, currentOwner)
	}

	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add contract generation block to chain: %w", err)
	}

	return nil
}

// processRegularContractTransaction handles regular contract execution
func (c *Core) processRegularContractTransaction(event *model.PubSubTxnInfo, txnBlock *block.Block, tokenId string, logger logger.Logger) error {
	// Validate chain continuity
	if err := c.validateChainContinuity(tokenId, txnBlock, logger); err != nil {
		return fmt.Errorf("chain validation failed: %w", err)
	}

	// Validate ownership
	if err := c.validatePublisherTokenOwnership(event, tokenId, txnBlock, logger); err != nil {
		return fmt.Errorf("ownership validation failed: %w", err)
	}

	// Add block to chain
	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add contract block to chain: %w", err)
	}

	return nil
}

// validateChainContinuity ensures no missing blocks in the token chain
func (c *Core) validateChainContinuity(tokenId string, txnBlock *block.Block, logger logger.Logger) error {
	tokenType := txnBlock.GetTokenType(tokenId)
	latestTokenBlock := c.w.GetLatestTokenBlock(tokenId, tokenType)

	if latestTokenBlock == nil {
		return fmt.Errorf("latest token block not found for token %s", tokenId)
	}

	latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get latest block number: %w", err)
	}

	currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get current block number: %w", err)
	}

	if latestBlockNumber+1 != currentBlockNumber {
		return fmt.Errorf("missing block detected: latest=%d, current=%d", latestBlockNumber, currentBlockNumber)
	}

	return nil
}

// validateTokenOwnership verifies the publisher owns the token
func (c *Core) validatePublisherTokenOwnership(event *model.PubSubTxnInfo, tokenId string, txnBlock *block.Block, logger logger.Logger) error {
	tokenType := txnBlock.GetTokenType(tokenId)
	latestTokenBlock := c.w.GetLatestTokenBlock(tokenId, tokenType)

	if latestTokenBlock == nil {
		return fmt.Errorf("cannot validate ownership: latest token block not found")
	}

	previousOwner := latestTokenBlock.GetOwner()
	if previousOwner != event.PublisherDID {
		return fmt.Errorf("publisher DID mismatch: expected %s, got %s", event.PublisherDID, previousOwner)
	}

	return nil
}

func (c *Core) publishTxn(newEvent *model.PubSubTxnInfo) error {
	topic := RubixTxnTopic
	if c.ps != nil {
		err := c.ps.Publish(topic, newEvent)
		if err != nil {
			c.log.Error("Failed to publish new txn", "err", err)
			return err
		}
		c.log.Info("New state published on topic " + topic)
	}
	return nil
}
