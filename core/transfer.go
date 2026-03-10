package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
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

func (c *Core) InitiateTransfer(reqID string, req *models.TransferRequest) {
	br := c.initiateTransfer(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateTransfer(reqID string, req *models.TransferRequest) *model.BasicResponse {

	resp := &model.BasicResponse{
		Status: false,
	}

	initiatorDID := req.Initiator
	nextOwnerDID := req.Owner

	dc, err := c.SetupDID(reqID, initiatorDID)
	if err != nil {
		resp.Message = "Failed to setup DID: " + err.Error()
		return resp
	}

	var errFetch bool

	collectedRBTs, errFetch := c.fetchTokens(
		req.HasRBT(),
		c.w.GetRBTTokens,
		"InitiateTransfer: Failed to get RBT for transfer: ",
		resp,
	)
	if !errFetch {
		return resp
	}

	collectedFTs, errFetch := c.fetchTokens(
		req.HasFT(),
		c.w.GetFTTokens,
		"InitiateTransfer: Failed to get FT for transfer: ",
		resp,
	)
	if !errFetch {
		return resp
	}

	collectedNFTs, errFetch := c.fetchTokens(
		req.HasNFT(),
		c.w.GetNFTTokens,
		"InitiateTransfer: Failed to get NFT for transfer: ",
		resp,
	)
	if !errFetch {
		return resp
	}

	collectedSmartContracts, errFetch := c.fetchTokens(
		req.HasSmartContract(),
		c.w.GetSmartContractTokens,
		"InitiateTransfer: Failed to get SC for execution: ",
		resp,
	)
	if !errFetch {
		return resp
	}

	// Need to get the quorumAddress from the db
	p, err := c.getPeer(quorumAddress)
	if err != nil {
		return err
	}
	defer p.Close()

	// Data need to be properly uipdated here
	request := models.PledgeTokenRequest{
		TransactionValue : 0.0      
	}
	var response models.PledgeTokenResponse

	// Make API call
	err = p.SendJSONRequest("POST", APIReqPledgeToken, nil, &request, &response, true)
	if err != nil {
		return err
	}

	// Check response
	if !response.Status {
		return fmt.Errorf("failed: %s", response.Message)
	}

	// Whatever implemented above are placeholder functions. These needs to be changed with proper functions which fetches the required tokens according to the set conditions
	//TODO
	// Need to form the TransactionTokens struct here from the info collected above.
	// The steps would be to get the token and the previous transaction id for each of the tokens. We will get that from []models.Token, just need to write a function to fetch those
	// Implement functions which fetches the required tokens according to the owner did from db
	// Then we need to create a function to make this TransactionInfo struct with all the informatiosn we have.

	// The api calls with the quorum

	//Concern:
	// The concern we have at the moment is if there is a smart contract execution and nft execution happening in the same transaction
	// then we have a problem. We only have a single data field in TransactionInfo.
	pledgeTokens := response.PledgeTokens
	// need to form the QuorumInfo struct {} with the pledgeToken information
	transactionInfo := models.TransactionInfo{
		Initiator: initiatorDID,
		Owner:     nextOwnerDID,
		Epoch:     int(st.Unix()),
		Network:   "",
		Tokens:    nil,
	}

	return resp
}

// This helper function will help us fetch different types of token from the db
// Here we are passing the GETFunctions we already have for the different types of tokens.
// This needs to be updated. The existing functions are used as placeholders for now.
func (c *Core) fetchTokens(
	enabled bool,
	getTokens func() ([]models.Token, error),
	errMsg string,
	resp *model.BasicResponse,
) ([]models.Token, bool) {

	if !enabled {
		return nil, true
	}

	tokens, err := getTokens()
	if err != nil {
		msg := errMsg + err.Error()
		c.log.Error(msg)
		resp.Message = msg
		return nil, false
	}

	return tokens, true
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
	return parts.CollectRBTTokens(dc, c.w, req.TokenCount, c.testnet, c.log, c.publishTxn)
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

func getConsensusRequest(consensusRequestType int, senderPeerID string, receiverPeerID string,
	contractBlock []byte, transactionEpoch int, isSelfTransfer bool,
) *ConensusRequest {
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

	// Track overall transaction performance
	var txErr error
	defer func() {
		c.TrackOperation("tx.rbt_transfer.total", map[string]interface{}{
			"sender":   req.Sender,
			"receiver": req.Receiver,
			"amount":   req.TokenCount,
			"type":     req.Type,
		})(txErr)
	}()

	resp := &model.BasicResponse{
		Status: false,
	}

	senderDID := req.Sender
	receiverdid := req.Receiver

	// This flag indicates if the call is made for Self Transfer or general token transfer
	isSelfRBTTransfer := senderDID == receiverdid

	// Track validation phase
	defer c.TrackOperation("tx.rbt_transfer.validation", map[string]interface{}{
		"token_count": req.TokenCount,
	})(nil)

	dc, err := c.SetupDID(reqID, senderDID)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		txErr = err
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

	// for i := range tokensForTxn {
	// tokenIdBuffer := bytes.NewBufferString(tokensForTxn[i].TokenID)
	// tokenIdHash, err := c.w.Add(tokenIdBuffer, senderDID, wallet.OwnerRole)
	// if err != nil {
	// 	c.log.Error("Failed to add commited tokens to ipfs", "err", err)
	// 	resp.Message = "Failed to add commited tokens to ipfs , err : " + err.Error()
	// 	return resp
	// }
	// c.w.Pin(tokensForTxn[i].TokenID, wallet.OwnerRole, senderDID, "TID-Not Generated", req.Sender, req.Receiver, tokensForTxn[i].TokenValue)
	// }

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

		genesisBlock := c.w.GetGenesisTokenBlock(tokensForTxn[i].TokenID, tt)
		if genesisBlock == nil {
			c.log.Error("failed to get genesis block, invalid token chain")
			resp.Message = "failed to get genesis block, invalid token chain"
			return resp
		}

		if err := c.ValidateTokenNetworkID(genesisBlock, tokensForTxn[i].TokenID); err != nil {
			c.log.Error("failed to validate token network ID", "err", err)
			resp.Message = "failed to validate token network ID, " + err.Error()
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
				if prevQuorumInfo == nil {
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
	// resultChan := make(chan *model.BasicResponse, 1)

	// to distinguish between transaction types
	cr.OperationType = req.OperationType

	// Start the transaction in a goroutine
	// go func() {
	td, _, _, consError := c.initiateConsensus(cr, sc, dc)
	if consError != nil {
		resp.Message = fmt.Sprintf("Consensus failed " + consError.Error())
		resp.Status = false
		// resultChan <- resp
		return resp
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
		return resp
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

	// Send final transaction completion response if not already timed out
	// select {
	// case resultChan <- resp:
	// 	// Successfully sent to resultChan
	// default:
	// 	// If no one is listening (already timed out), just log and exit
	// 	c.log.Debug("Transaction completed but resultChan is not being read anymore")
	// }
	// }()

	// select {
	// case result := <-resultChan:
	// 	// Transaction completed within 40s or failed
	// 	c.log.Debug("transaction completed before 20 secs")
	// 	return result

	// case <-time.After(20 * time.Second):
	// 	// Timeout occurred, return Transaction ID only
	// 	c.log.Debug("transaction still processing with txn id ", cr.TransactionID)

	// 	msg := fmt.Sprintf("Transaction is still processing, with transaction id %v ", cr.TransactionID)
	// 	resp.Message = msg
	// 	if strings.Contains(resp.Message, "with transaction id") {
	// 		if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
	// 			resp.Result = txID
	// 		}
	// 	}
	// 	resp.Status = true
	// 	return resp
	// }

	// c.log.Debug("transaction completed with txn id ", cr.TransactionID)

	// msg = fmt.Sprintf("Transaction completed with transaction id %v ", cr.TransactionID)
	// resp.Message = msg
	// if strings.Contains(resp.Message, "with transaction id") {
	// 	if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
	// 		resp.Result = txID
	// 	}
	// }

	resp.Status = true
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

// TODO(parts): revert the changes made to match the current function signature of
// new parts logic
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

	reqTokens, err := parts.CollectRBTTokens(dc, c.w, req.TokenCount, c.testnet, c.log, c.publishTxn)
	if err != nil {
		c.w.ReleaseTokens(reqTokens)
		c.log.Error("Failed to get tokens", "err", err)
		resp.Message = "Insufficient tokens or tokens are locked or " + err.Error()
		return resp
	}
	if len(reqTokens) != 0 {
		tokensForTxn = append(tokensForTxn, reqTokens...)
	}

	//TODO(parts): uncomment the following
	// if remainingAmount > 0 {
	// 	wt, err := c.GetTokens(dc, did, remainingAmount, PinningServiceMode)
	// 	if err != nil {
	// 		c.log.Error("Failed to get tokens", "err", err)
	// 		resp.Message = "Insufficient tokens or tokens are locked"
	// 		return resp
	// 	}
	// 	if len(wt) != 0 {
	// 		tokensForTxn = append(tokensForTxn, wt...)
	// 	}
	// }

	//TODO(parts): revert the following
	//return c.completePinning(st, reqID, req, did, pinningNodeDID, pinningNodepeerid, tokensForTxn, resp, dc)
	return c.completePinning(st, reqID, req, did, pinningNodeDID, pinningNodepeerid, nil, resp, dc)
}

func (c *Core) completePinning(st time.Time, reqID string, req *model.RBTPinRequest, did, pinningNodeDID, pinningNodepeerid string, tokensForTxn []wallet.Token, resp *model.BasicResponse, dc did.DIDCrypto) *model.BasicResponse {
	var sumOfTokensForTxn float64
	for _, tokenForTxn := range tokensForTxn {
		sumOfTokensForTxn = sumOfTokensForTxn + tokenForTxn.TokenValue
		sumOfTokensForTxn = floatPrecision(sumOfTokensForTxn, MaxDecimalPlaces)
	}
	// release the locked tokens before exit
	defer c.w.ReleaseTokens(tokensForTxn)

	// for i := range tokensForTxn {
	// tokenIdBuffer := bytes.NewBufferString(tokensForTxn[i].TokenID)
	// tokenIdHash, err := c.w.Add(tokenIdBuffer, did, wallet.OwnerRole)
	// if err != nil {
	// 	c.log.Error("Failed to add commited tokens to ipfs", "err", err)
	// 	resp.Message = "Failed to add commited tokens to ipfs , err : " + err.Error()
	// 	return resp
	// }
	// 	c.w.Pin(tokensForTxn[i].TokenID, wallet.PinningRole, did, "TID-Not Generated", req.Sender, req.PinningNode, tokensForTxn[i].TokenValue)
	// }
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
	td, _, _, err := c.initiateConsensus(cr, sc, dc)
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
