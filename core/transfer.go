package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
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
		c.log.Error("Failed to get did channels in InitiateRBTTransfer")
		return
	}
	dc.OutChan <- br
}

func gatherTokensForTransaction(c *Core, req *model.RBTTransferRequest, dc did.DIDCrypto, isSelfRBTTransfer bool) ([]wallet.Token, int, error) {
	c.log.Debug("********gathering tokens for transaction")
	var tokensForTransfer []wallet.Token
	var transferMode int = SpendableRBTTransferMode // Default mode

	senderDID := req.Sender

	if !isSelfRBTTransfer {
		if req.TokenCount < MinDecimalValue(MaxDecimalPlaces) {
			return nil, transferMode, fmt.Errorf("input transaction amount is less than minimum transaction amount")
		}

		decimalPlaces := strconv.FormatFloat(req.TokenCount, 'f', -1, 64)
		decimalPlacesStr := strings.Split(decimalPlaces, ".")
		if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > MaxDecimalPlaces {
			return nil, transferMode, fmt.Errorf("transaction amount exceeds %v decimal places", MaxDecimalPlaces)
		}

		accountBalance, err := c.GetAccountInfo(senderDID)
		if err != nil {
			return nil, transferMode, fmt.Errorf("failed to get account info: %v", err.Error())
		}

		totalBalance := accountBalance.SpendableRBT

		if req.TokenCount > totalBalance {
			return nil, transferMode, fmt.Errorf("insufficient balance, account balance is %v, transaction value is %v", totalBalance, req.TokenCount)
		}

		// if accountBalance.SpendableRBT == 0 && req.TokenCount <= accountBalance.RBTAmount {
		// 	// No spendable RBT, but enough free RBT tokens available
		// 	c.log.Info("Going ahead with free RBT tokens transfer")
		// 	transferMode = RBTTransferMode
		// 	reqTokens, remainingAmount, err := c.GetRequiredTokens(senderDID, req.TokenCount, RBTTransferMode)
		// 	if err != nil {
		// 		c.w.ReleaseTokens(reqTokens)
		// 		return nil, transferMode, fmt.Errorf("failed to get free RBT tokens: %v", err.Error())
		// 	}

		// 	if len(reqTokens) != 0 {
		// 		tokensForTransfer = append(tokensForTransfer, reqTokens...)
		// 	}

		// 	if remainingAmount > 0 {
		// 		wt, err := c.GetTokens(dc, senderDID, remainingAmount, RBTTransferMode)
		// 		if err != nil {
		// 			c.w.ReleaseTokens(tokensForTransfer)
		// 			return nil, transferMode, fmt.Errorf("failed to get additional free RBT tokens: %v", err.Error())
		// 		}
		// 		if len(wt) != 0 {
		// 			tokensForTransfer = append(tokensForTransfer, wt...)
		// 		}
		// 	}

		// 	var sumOfTokensForTxn float64
		// 	for _, tokenForTransfer := range tokensForTransfer {
		// 		sumOfTokensForTxn += tokenForTransfer.TokenValue
		// 		sumOfTokensForTxn = floatPrecision(sumOfTokensForTxn, MaxDecimalPlaces)
		// 	}

		// 	if sumOfTokensForTxn != req.TokenCount {
		// 		c.w.ReleaseTokens(tokensForTransfer)
		// 		return nil, transferMode, fmt.Errorf("sum of selected tokens %v does not equal transaction value %v", sumOfTokensForTxn, req.TokenCount)
		// 	}

		// 	return tokensForTransfer, transferMode, nil
		// }else

		// Use SpendableRBTTransferMode (already set as default)
		reqTokens, remainingAmount, err := c.GetRequiredTokens(senderDID, req.TokenCount, transferMode)
		if err != nil {
			c.w.ReleaseTokens(reqTokens, c.testNet)
			return nil, transferMode, fmt.Errorf("failed to get spendable tokens: %v", err.Error())
		}

		if len(reqTokens) != 0 {
			tokensForTransfer = append(tokensForTransfer, reqTokens...)
		}

		if remainingAmount > 0 {
			wt, err := c.GetTokens(dc, senderDID, remainingAmount, SpendableRBTTransferMode)
			if err != nil {
				c.w.ReleaseTokens(tokensForTransfer, c.testNet)
				return nil, transferMode, fmt.Errorf("failed to get additional spendable tokens: %v", err.Error())
			}
			if len(wt) != 0 {
				tokensForTransfer = append(tokensForTransfer, wt...)
			}
		}

		var sumOfTokensForTxn float64
		for _, tokenForTransfer := range tokensForTransfer {
			sumOfTokensForTxn += tokenForTransfer.TokenValue
			sumOfTokensForTxn = floatPrecision(sumOfTokensForTxn, MaxDecimalPlaces)
		}

		if sumOfTokensForTxn != req.TokenCount {
			c.w.ReleaseTokens(tokensForTransfer, c.testNet)
			return nil, transferMode, fmt.Errorf("sum of selected tokens %v does not equal transaction value %v", sumOfTokensForTxn, req.TokenCount)
		}

		return tokensForTransfer, transferMode, nil
		// } else {
		// 	// req.TokenCount > spendableRBT but <= totalBalance
		// 	return nil, transferMode, fmt.Errorf("transaction amount %v exceeds spendable RBT %v; locked tokens cannot be used", req.TokenCount, accountBalance.SpendableRBT)
		// }
	} else {
		// Self-transfer logic remains unchanged
		tokensOwnedBySender, err := c.w.GetFreeTokens(senderDID)
		if err != nil {
			if strings.Contains(err.Error(), "no records found") {
				return []wallet.Token{}, transferMode, nil
			}
			return nil, transferMode, fmt.Errorf("failed to get free tokens of owner, error: %v", err.Error())
		}

		for _, token := range tokensOwnedBySender {
			if token.TransactionID == "" {
				continue
			}
			tokenTransactionDetail, err := c.w.GetTransactionDetailsbyTransactionId(token.TransactionID)
			if err != nil {
				return nil, transferMode, fmt.Errorf("failed to get transaction details for trx hash: %v, err: %v", token.TransactionID, err)
			}

			if time.Now().Unix()-tokenTransactionDetail.Epoch > int64(pledgePeriodInSeconds) {
				if err := c.w.LockToken(&token); err != nil {
					return nil, transferMode, fmt.Errorf("failed to lock tokens %v, exiting selfTransfer routine with error: %v", token.TokenID, err.Error())
				}
				tokensForTransfer = append(tokensForTransfer, token)
			}
		}

		if len(tokensForTransfer) > 0 {
			c.log.Debug("Tokens acquired for self transfer")
		}
		return tokensForTransfer, transferMode, nil
	}
}

func (c *Core) getUniqueTransTokensFromLatestBlocks(tokensForTxn []wallet.Token) ([]string, error) {
	uniqueTransTokens := make([]string, 0)
	// Create a map for quick lookup of tokensForTxn TokenIDs
	tokensForTxnMap := make(map[string]bool)
	for _, token := range tokensForTxn {
		tokensForTxnMap[token.TokenID] = true
	}

	for _, token := range tokensForTxn {
		// Determine token type
		tts := "rbt"
		if token.TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)

		// Get the latest token block
		blk := c.w.GetLatestTokenBlock(token.TokenID, tt)
		if blk == nil {
			return nil, fmt.Errorf("failed to get latest block for token %v, invalid token chain", token.TokenID)
		}
		// blk.GetTokenBlock()
		// Get trans tokens from the block
		transTokens := blk.GetTransTokens()
		if transTokens == nil {
			continue // Skip if no trans tokens
		}

		// Check each trans token
		for _, transToken := range transTokens {
			// Only add if transToken is not in tokensForTxn
			if !tokensForTxnMap[transToken] {
				// Avoid duplicates in output
				found := false
				for _, addedToken := range uniqueTransTokens {
					if addedToken == transToken {
						found = true
						break
					}
				}
				if !found {
					uniqueTransTokens = append(uniqueTransTokens, transToken)
				}
			}
		}
	}

	return uniqueTransTokens, nil
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

// create a seperate function for the transfer of spendable RBTs
// func (c *Core) initiateSpendableRBTTransfer(reqID string, req *model.RBTTransferRequest) *model.BasicResponse {

// }

func (c *Core) initiateRBTTransfer(reqID string, req *model.RBTTransferRequest) *model.BasicResponse {
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}

	senderDID := req.Sender
	receiverDID := req.Receiver

	// This flag indicates if the call is made for Self Transfer or general token transfer
	isSelfRBTTransfer := senderDID == receiverDID

	dc, err := c.SetupDID(reqID, senderDID)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}

	c.log.Debug("*****Setup DID is done for sender in initiateRBTTransfer function********")

	tokensForTxn, transferMode, err := gatherTokensForTransaction(c, req, dc, isSelfRBTTransfer)
	if err != nil {
		errMsg := fmt.Sprintf("failed to retrieve spendable RBTs for transfer, err: %v", err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	c.log.Debug("***Tokens gathered for transaction****")
	c.log.Debug("*****Transfer Mode is****", transferMode)

	// In case of self transfer
	if len(tokensForTxn) == 0 && isSelfRBTTransfer {
		resp.Status = true
		resp.Message = "No tokens present for self transfer"
		c.log.Error("No tokens present for self transfer")
		return resp
	}

	// release the locked tokens before exit
	defer c.w.ReleaseTokens(tokensForTxn, c.testNet)

	for i := range tokensForTxn {
		_, err := c.w.Pin(tokensForTxn[i].TokenID, wallet.OwnerRole, senderDID, "TID-Not Generated", req.Sender, req.Receiver, tokensForTxn[i].TokenValue)
		if err != nil {
			errMsg := fmt.Sprintf("failed to pin the token: %v, error: %v", tokensForTxn[i].TokenID, err)
			c.log.Error(errMsg)
		}
	}

	// Get the receiver & do sanity check
	var rpeerid string = ""
	if !isSelfRBTTransfer {
		rpeerid = c.w.GetPeerID(receiverDID)
		if rpeerid == "" {
			// Check if DID is present in the DIDTable as the receiver might be part of the current node
			didDetails, err := c.w.GetDID(receiverDID)
			if err != nil {
				if strings.Contains(err.Error(), "no records found") {
					c.log.Error("receiver Peer ID not found", "did", receiverDID)
					resp.Message = "invalid address, receiver Peer ID not found"
					//return resp
				} else {
					c.log.Error(fmt.Sprintf("Error occurred while fetching DID info from DIDTable for DID: %v, err: %v", receiverDID, err))
					resp.Message = fmt.Sprintf("Error occurred while fetching DID info from DIDTable for DID: %v, err: %v", receiverDID, err)
					return resp
				}
			}

			if didDetails == nil {
				receiverPeerInfo, err := c.GetPeerDIDInfo(receiverDID)
				if err != nil {
					c.log.Error("receiver Peer ID not found in network", "did", receiverDID)
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
	// wta := make([]string, 0)
	// for i := range tokensForTxn {
	// 	wta = append(wta, tokensForTxn[i].TokenID)
	// }

	tis := make([]contract.TokenInfo, 0)
	tokenListForExplorer := []Token{}
	selfTransferTokensMap := make(map[string]struct{})

	c.log.Debug("tokens for transaction : ", tokensForTxn)

	for i := range tokensForTxn {
		tts := "rbt"
		if tokensForTxn[i].TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(tokensForTxn[i].TokenID, tt)
		if blk == nil {
			errMsg := fmt.Sprintf("failed to get latest block for the token: %v", tokensForTxn[i].TokenID)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
		//sender verifies whether the previous block is a cvr stage-2 block or not
		if transferMode == SpendableRBTTransferMode {
			if tts == "part" {
				latestBlockNumber, err := blk.GetBlockNumber(tokensForTxn[i].TokenID)
				if err != nil {
					errMsg := fmt.Sprintf("failed to get block number for the token: %v", tokensForTxn[i].TokenID)
					c.log.Error(errMsg)
					resp.Message = errMsg
					return resp
				}
				if latestBlockNumber == 0 && blk.GetTransType() == block.TokenGeneratedType {
					parentToken, _, err := blk.GetParentDetials(tokensForTxn[i].TokenID)
					if err != nil {
						errMsg := fmt.Sprintf("failed to get parent token for the token: %v", tokensForTxn[i].TokenID)
						c.log.Error(errMsg)
						resp.Message = errMsg
						return resp
					}
					parentTokenValue, err := c.w.GetTokenValueByTokenID(parentToken, req.Sender)
					if err != nil {
						errMsg := fmt.Sprintf("failed to get parent token value for the token: %v , error: %v", tokensForTxn[i].TokenID, err)
						c.log.Error(errMsg)
						resp.Message = errMsg
						return resp
					}
					var tokenTypeStr string
					if parentTokenValue != 1.0 {
						tokenTypeStr = "part"
					} else {
						tokenTypeStr = "rbt"
					}
					parentTokenType := c.TokenType(tokenTypeStr)

					burntBlockOfParentToken := c.w.GetLatestTokenBlock(parentToken, parentTokenType)
					previousBlkIDOfBurntBlk, err := burntBlockOfParentToken.GetPrevBlockID(parentToken)
					if err != nil {
						errMsg := fmt.Sprintf("failed to get previous blockID of burnt block of the token : %v , error: %v", parentToken, err)
						c.log.Error(errMsg)
						resp.Message = errMsg
						return resp
					}
					blkPreviousTOBurntBlkBytes, err := c.w.GetTokenBlock(parentToken, parentTokenType, previousBlkIDOfBurntBlk)
					if err != nil {
						errMsg := fmt.Sprintf("failed to get a block which is previous to the burnt block of the token : %v , error: %v", parentToken, err)
						c.log.Error(errMsg)
						resp.Message = errMsg
						return resp
					}
					blkPreviousTOBurntBlk := block.InitBlock(blkPreviousTOBurntBlkBytes, nil)
					if blkPreviousTOBurntBlk.GetTransType() != block.OwnershipTransferredType {
						errmsg := fmt.Sprintf("block which is previous to the burnt block is not of type : %v", block.OwnershipTransferredType)
						c.log.Error(errmsg)
						resp.Message = errmsg
						return resp
					}

				}

			} else {
				if blk.GetTransType() != block.OwnershipTransferredType {
					errMsg := fmt.Sprintf("previous block is not of type : %v", block.OwnershipTransferredType)
					c.log.Error(errMsg)
					resp.Message = errMsg
					return resp
				}

			}

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

		// gather tokens for self-transfer : check if the transToken exists in the self-transfer map,
		// if exists delete the token from the self-transfer map, if does not exist
		// then fetch the list of all tokens from the latest block, and
		// add all the tokens to the self-transfer map except the trans-token
		c.log.Debug("*********dealing with trans-token :", tokensForTxn[i].TokenID)

		_, exists := selfTransferTokensMap[tokensForTxn[i].TokenID]
		if exists {
			c.log.Debug("&&&&&trans-token in self-trans map before deleting : ", selfTransferTokensMap[tokensForTxn[i].TokenID])
			delete(selfTransferTokensMap, tokensForTxn[i].TokenID)
			c.log.Debug("&&&&&&&trans-token in self-trans map after deleting : ", selfTransferTokensMap[tokensForTxn[i].TokenID])
		} else {
			transTokensInBlock := blk.GetTransTokens()
			c.log.Debug("***********trans tokens in last block :", transTokensInBlock)
			for _, token := range transTokensInBlock {
				if token != tokensForTxn[i].TokenID {
					selfTransferTokensMap[token] = struct{}{}
				}
			}
		}

	}

	//check if sender has previous block pledged quorums' details
	//TODO: Reuse blk for GetLatestTokenBlock
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

	// preaparing the block for tokens to be transferred to receiver
	contractType := getContractType(reqID, req, tis, isSelfRBTTransfer)

	// c.log.Debug("***********Contract type for sender to receiver transaction******* ", contractType)

	c.log.Debug("*******creating the contract for sender to receiver transaction*******")

	sc := contract.CreateNewContract(contractType)

	// Starting CVR stage-1
	// TODO : handle cvr stage-0 : quorum's signature
	// And handle self-transfer of sender's remaining amount

	c.log.Debug("*****sender signining on the contract block*******")

	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	// create block in cvr stage-1
	transactionID := util.HexToStr(util.CalculateHash(sc.GetBlock(), "SHA3-256"))

	c.log.Debug("*****sender to receiver transactionID***", transactionID)
	tks := make([]block.TransTokens, 0)
	ctcb := make(map[string]*block.Block)

	for i := range tis {
		tt := block.TransTokens{
			Token:     tis[i].Token,
			TokenType: tis[i].TokenType,
		}
		tks = append(tks, tt)
		b := c.w.GetLatestTokenBlock(tis[i].Token, tis[i].TokenType)
		ctcb[tis[i].Token] = b
	}

	bti := &block.TransInfo{
		Comment: sc.GetComment(),
		TID:     transactionID,
		Tokens:  tks,
	}
	signData, senderNLSSShare, senderPrivSign, err := sc.GetHashSig(senderDID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch sender sign; err: %v", err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}
	senderSignType := dc.GetSignType()
	senderSign := &block.InitiatorSignature{
		NLSSShare:   senderNLSSShare,
		PrivateSign: senderPrivSign,
		DID:         senderDID,
		Hash:        signData,
		SignType:    senderSignType,
	}

	tcb := block.TokenChainBlock{
		TransactionType:    block.SpendableTokenTransferredType,
		TokenOwner:         sc.GetReceiverDID(),
		TransInfo:          bti,
		SmartContract:      sc.GetBlock(),
		InitiatorSignature: senderSign,
		Epoch:              txEpoch,
	}
	c.log.Debug("****creating new transblock for sender to receiver transaction in cvr-1*********")
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block - qrm init")
		resp.Message = "failed to create new token chain block - qrm init"
		return resp
	}

	sr := SendTokenRequest{
		Address:         c.peerID + "." + sc.GetSenderDID(),
		TokenInfo:       tis,
		TokenChainBlock: nb.GetBlock(),
		// QuorumList:         cr.QuorumList,
		TransactionEpoch:   txEpoch,
		PinningServiceMode: false,
		CVRStage:           wallet.CVRStage1_Sender_to_Receiver,
		// TransTokenSyncInfo: cr.TransTokenSyncInfo,
	}
	rp, err := c.getPeer(rpeerid + "." + sc.GetReceiverDID())
	if err != nil {
		c.log.Error("Receiver not connected", "err", err)
		resp.Message = "Receiver not connected" + err.Error()
		return resp
	}
	defer rp.Close()

	var br model.BasicResponse
	err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)
	if err != nil {

		c.log.Error("Unable to send tokens to receiver", "err", err)
		resp.Message = "Unable to send tokens to receiver: " + err.Error()
		return resp

	}
	if !br.Status {
		if strings.Contains(br.Message, "failed to sync tokenchain") {
			tokenPrefix := "Token: "
			issueTypePrefix := "issueType: "

			// Find the starting indexes of pt and issueType values
			ptStart := strings.Index(br.Message, tokenPrefix) + len(tokenPrefix)
			issueTypeStart := strings.Index(br.Message, issueTypePrefix) + len(issueTypePrefix)

			// Extracting the substrings from the message
			token := br.Message[ptStart : strings.Index(br.Message[ptStart:], ",")+ptStart]
			issueType := br.Message[issueTypeStart:]

			c.log.Debug("String: token is ", token, " issuetype is ", issueType)
			issueTypeInt, err1 := strconv.Atoi(issueType)
			if err1 != nil {
				errMsg := fmt.Sprintf("transfer failed due to token chain sync issue, issueType string conversion, err %v", err1)
				c.log.Error(errMsg)
				resp.Message = errMsg
				return resp
			}
			c.log.Debug("issue type in int is ", issueTypeInt)
			syncIssueTokenDetails, err2 := c.w.ReadToken(token)
			if err2 != nil {
				errMsg := fmt.Sprintf("transfer failed due to tokenchain sync issue, err %v", err2)
				c.log.Error(errMsg)
				resp.Message = errMsg
				return resp
			}
			c.log.Debug("sync issue token details ", syncIssueTokenDetails)
			if issueTypeInt == TokenChainNotSynced {
				syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
				c.log.Debug("sync issue token details status updated", syncIssueTokenDetails)
				c.w.UpdateToken(syncIssueTokenDetails)
				resp.Message = "Receiver failed to sync token chain"
				return resp
			}
		}
		c.log.Error("Unable to send tokens to receiver", "msg", br.Message)
		return resp
	}

	// update token status based on the situations:
	// 1. sender receiver on different ports
	// 2. sender receiver on same port
	if rpeerid != c.peerID {
		c.log.Debug("*********** updating token status for sender's transferred token")
		err = c.UpdateTransferredTokensInfo(tokensForTxn, wallet.TokenIsTransferred, transactionID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to update trans tokens in DB, err : %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
	} else {
		c.log.Debug("******sender peer and receiver's is same")
	}

	selfTransferResponse := c.CreateSelfTransferRBTContract(selfTransferTokensMap, dc, req, txEpoch)
	if !selfTransferResponse.Status {
		errMsg := fmt.Sprintf("self transfer contract creation failed, err : %v", selfTransferResponse.Message)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	selfTransferContract := selfTransferResponse.Result.(*contract.Contract)

	nbid, err := nb.GetBlockID(tis[0].Token)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get blockID for token %v, err: %v", tis[0].Token, err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)

	td := &model.TransactionDetails{
		TransactionID:   transactionID,
		TransactionType: nb.GetTransType(),
		BlockID:         nbid,
		Mode:            wallet.SendMode,
		SenderDID:       sc.GetSenderDID(),
		ReceiverDID:     sc.GetReceiverDID(),
		Comment:         sc.GetComment(),
		DateTime:        time.Now(),
		Status:          true,
		Epoch:           int64(txEpoch),
	}

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

	c.log.Debug("**adding txnd details to the transactionHistory table*****")

	if err := c.w.AddTransactionHistory(td); err != nil {
		errMsg := fmt.Sprintf("Error occured while adding transaction details: %v", err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	// if !c.testNet {
	// etrans := &ExplorerRBTTrans{
	// 	TokenHashes:   wta,
	// 	TransactionID: td.TransactionID,
	// 	BlockHash:     strings.Split(td.BlockID, "-")[1],
	// 	Network:       req.Type,
	// 	SenderDID:     senderDID,
	// 	ReceiverDID:   receiverDID,
	// 	Amount:        req.TokenCount,
	// 	// QuorumList:     extractQuorumDID(cr.QuorumList),
	// 	// PledgeInfo:     PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
	// 	TransTokenList: tokenListForExplorer,
	// 	Comments:       req.Comment,
	// }
	// 	c.log.Debug("************initiating explorer updation")
	// 	c.ec.ExplorerRBTTransaction(etrans)
	// 	c.log.Debug("************completed explorer updation")
	// }

	// Starting CVR stage-2
	cvrRequest := &wallet.PrePledgeRequest{
		DID:        senderDID,
		QuorumType: req.Type,
		// TxnID:             transactionID,
		// SelftransferTxnID: selfTransactionID,
		TransferMode:    SpendableRBTTransferMode,
		SCTransferBlock: sc.GetBlock(),
		TxnEpoch:        int64(txEpoch),
		ReqID:           reqID,
	}
	if selfTransferContract != nil {
		cvrRequest.SCSelfTransferBlock = selfTransferContract.GetBlock()
	}

	// cvr-2 go-routine
	go func(cvrReq *wallet.PrePledgeRequest) {
		resp := c.initiateCVRTwo(cvrReq)
		c.log.Debug("response from CVR-2 : ", resp)
	}(cvrRequest)

	// go c.initiateRBTCVRTwo(cvrRequest)

	// //pre-pledging related api will get call here
	// cr := getConsensusRequest(req.Type, c.peerID, rpeerid, sc.GetBlock(), txEpoch, isSelfRBTTransfer)

	// td, _, _, err = c.initiateConsensus(cr, sc, dc)
	// if err != nil {
	// 	c.log.Error("Consensus failed ", "err", err)
	// 	resp.Message = "Consensus failed " + err.Error()
	// 	return resp
	// }

	c.log.Info("Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
	resp.Status = true
	msg := fmt.Sprintf("Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
	resp.Message = msg
	return resp
}

// prepare self-transfer RBTs and create self-transfer contract block
func (c *Core) CreateSelfTransferRBTContract(selfTransferTokensMap map[string]struct{}, dc did.DIDCrypto, req *model.RBTTransferRequest, txnEpoch int) *model.BasicResponse {
	resp := &model.BasicResponse{
		Status: false,
	}

	// get all self-transfer tokens,
	// the final self-transfer token list is : selfTransferTokensList
	var selfTransactionID string
	var selfTransferBlock *block.Block
	var selfTransferContract *contract.Contract
	var selfTransferTokenCount float64
	selfTransferTokensList := make([]contract.TokenInfo, 0)
	lockedTokensForSelfTransfer := make([]string, 0)

	for selfTransferToken := range selfTransferTokensMap {

		// check if the token is spendable, if not then do not include in self-transfer
		selftransferTokenInfo, err := c.w.GetToken(selfTransferToken, wallet.TokenIsSpendable)
		if err != nil {
			// errMsg := fmt.Sprintf("failed to get token for self-transfer, token: %v, error: %v", selfTransferToken, err)
			// c.log.Error(errMsg)
			continue
			// c.w.UnlockLockedTokens(req.Sender, lockedTokensForSelfTransfer, c.testNet)
			// resp.Message = errMsg
			// return resp
		}
		if selftransferTokenInfo.DID != req.Sender {
			continue
		}

		tts := "rbt"
		if selftransferTokenInfo.TokenValue != 1 {
			tts = "part"
		}
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(selfTransferToken, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			c.w.UnlockLockedTokens(req.Sender, lockedTokensForSelfTransfer, c.testNet)
			return resp
		}
		bid, err := blk.GetBlockID(selfTransferToken)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			c.w.UnlockLockedTokens(req.Sender, lockedTokensForSelfTransfer, c.testNet)
			return resp
		}

		selftransferTokeninfo := contract.TokenInfo{
			Token:      selfTransferToken,
			TokenType:  tt,
			TokenValue: selftransferTokenInfo.TokenValue,
			OwnerDID:   req.Sender,
			BlockID:    bid,
		}
		selfTransferTokensList = append(selfTransferTokensList, selftransferTokeninfo)
		lockedTokensForSelfTransfer = append(lockedTokensForSelfTransfer, selfTransferToken)
		selfTransferTokenCount = floatPrecision(selfTransferTokenCount+selftransferTokenInfo.TokenValue, MaxDecimalPlaces)

		// pinning self-transfer tokens
		_, err = c.w.Pin(selftransferTokenInfo.TokenID, wallet.OwnerRole, req.Sender, "TID-Not Generated", req.Sender, req.Sender, selftransferTokenInfo.TokenValue)
		if err != nil {
			errMsg := fmt.Sprintf("failed to pin the token: %v, error: %v", selftransferTokenInfo.TokenID, err)
			c.log.Error(errMsg)
		}
	}

	c.log.Debug("************ self transfer token count : ", selfTransferTokenCount)

	// if there are no available tokens for self-transfer then no need of self-transfer
	if len(selfTransferTokensList) != 0 {
		c.log.Debug("********** self transfer tokens : ", selfTransferTokensList)
		//create new contract for self transfer
		selfTransferReq := &model.RBTTransferRequest{
			Sender:     req.Sender,
			Receiver:   req.Sender,
			TokenCount: selfTransferTokenCount,
			Type:       req.Type,
			Password:   req.Password,
		}
		selfTransferContractType := getContractType(reqID, selfTransferReq, selfTransferTokensList, false)

		// c.log.Debug("***********Contract type for self transaction in cvr-1******* ", selfTransferContractType)

		c.log.Debug("*******creating the contract for self transaction in cvr-1*******")

		selfTransferContract = contract.CreateNewContract(selfTransferContractType)

		c.log.Debug("******** sender signing on self-transfer txn id")

		err := selfTransferContract.UpdateSignature(dc)
		if err != nil {
			errMsg := fmt.Sprintf("failed to update the signature on the self transfer contract, error: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}

		// creating self transfer block in cvr stage-2
		selfTransactionID = util.HexToStr(util.CalculateHash(selfTransferContract.GetBlock(), "SHA3-256"))

		c.log.Debug("trasactionID for self transaction", selfTransactionID)

		selfTransferTokens := make([]block.TransTokens, 0)
		latestTokenChainBlock := make(map[string]*block.Block)

		for i := range selfTransferTokensList {
			selfTransTokens := block.TransTokens{
				Token:     selfTransferTokensList[i].Token,
				TokenType: selfTransferTokensList[i].TokenType,
			}
			selfTransferTokens = append(selfTransferTokens, selfTransTokens)
			latestBlock := c.w.GetLatestTokenBlock(selfTransferTokensList[i].Token, selfTransferTokensList[i].TokenType)
			latestTokenChainBlock[selfTransferTokensList[i].Token] = latestBlock
		}

		selfTransBlockTransInfo := &block.TransInfo{
			Comment: selfTransferContract.GetComment(),
			TID:     selfTransactionID,
			Tokens:  selfTransferTokens,
		}

		// sender signature on txn id
		signData, senderNLSSShare, senderPrivSign, err := selfTransferContract.GetHashSig(dc.GetDID())
		if err != nil {
			errMsg := fmt.Sprintf("failed to fetch sender sign; err: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
		senderSignType := dc.GetSignType()
		senderSign := &block.InitiatorSignature{
			NLSSShare:   senderNLSSShare,
			PrivateSign: senderPrivSign,
			DID:         dc.GetDID(),
			Hash:        signData,
			SignType:    senderSignType,
		}

		selfTransferTCB := block.TokenChainBlock{
			TransactionType:    block.SpendableTokenTransferredType,   // cvr stage-1
			TokenOwner:         selfTransferContract.GetReceiverDID(), //ReceiverDID is same as req.Sender because it is self transfer
			TransInfo:          selfTransBlockTransInfo,
			SmartContract:      selfTransferContract.GetBlock(),
			InitiatorSignature: senderSign,
			Epoch:              txnEpoch,
		}

		c.log.Debug("*******creating a new block for self transaction*****")
		if latestTokenChainBlock == nil {
			errMsg := fmt.Sprintf("failed to get prev block of tokens for self-transfer")
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}

		c.log.Debug(fmt.Sprintf("self transfer tcb : %v", selfTransferTCB))

		selfTransferBlock = block.CreateNewBlock(latestTokenChainBlock, &selfTransferTCB)
		if selfTransferBlock == nil {
			c.log.Error("Failed to create a selftransfer token chain block - qrm init")
			resp.Message = "Failed to create a selftransfer token chain block - qrm init"
			return resp
		}
		//update token type
		var ok bool
		for _, t := range selfTransferTokensList {

			selfTransferBlock, ok = selfTransferBlock.UpdateTokenType(t.Token, token.CVR_RBTTokenType+t.TokenType)
			if !ok {
				errMsg := fmt.Sprintf("failed to update token cvr-1 type for self transfer block, error: %v", err)
				c.log.Error(errMsg)
				resp.Message = errMsg
				return resp
			}

		}

		//update the levelDB and SqliteDB
		updatedTokenStateHashesAfterSelfTransfer, err := c.w.TokensReceived(req.Sender, selfTransferTokensList, selfTransferBlock, c.peerID, c.peerID, false, c.ipfs)
		if err != nil {
			errMsg := fmt.Sprintf("failed to add self-transfer token block and update status, error: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
		//TODO: Send these updatedTokenStateHashesAfterSelfTransfer to quorums for pinning
		c.log.Debug(fmt.Sprintf("Updated token state hashes after self transfer are:%v", updatedTokenStateHashesAfterSelfTransfer))

	}

	resp.Result = selfTransferContract

	resp.Status = true
	resp.Message = "self-ytransfer contract created"
	return resp
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
			c.w.ReleaseTokens(reqTokens, c.testNet)
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
		c.w.ReleaseTokens(reqTokens, c.testNet)
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
	defer c.w.ReleaseTokens(tokensForTxn, c.testNet)

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
