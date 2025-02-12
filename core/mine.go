package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

//0. Free, 1. Locked, 2. Mined, 3. Partially Used

func (c *Core) mineRBT(reqID string, did string) *model.BasicResponse {
	st := time.Now()
	miningEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	} // Needed? TODO-Mining

	//Read table, collect tokens with CurrentBlockNumber - TransferBlockNumber >= 5 in tableData for the given did and credit status as free
	// var tableData PledgeHistory
	tableData := []wallet.PledgeHistory{}
	err = c.s.Read(wallet.PledgeHistoryTable, &tableData, "current_block_number-transfer_block_number>=?", 5)
	if err != nil {
		resp.Message = "No rows containing token block count> 5" + err.Error()
		return resp
	}
	//fetch from network, the number of credits required for mining
	creditsNeeded := 100
	//from the selected rows, calculate the total credits. and check if it matches requirements. If yes, collect data to send to mining validators.
	//And lock the credits/table being used. If any error, release locked credits.
	creditCount := 0
	toSendData := make([]model.ToSend, len(tableData))
	for _, td := range tableData {
		if creditCount+td.TokenCredit > creditsNeeded {
			td.TokenCredit = creditsNeeded - creditCount
		}
		creditCount += td.TokenCredit
		m := model.ToSend{
			TokenID:     td.TransferTokenID,
			TxnID:       td.TransactionID,
			TokenType:   td.TransferTokenType,
			BlockID:     string(td.TransferBlockNumber) + td.TransferBlockID,
			CreditsUsed: td.TokenCredit,
		}
		//  Update credit status as locked in table.
		toSendData = append(toSendData, m)
	}

	//Create Mining Contract and Add signature?

	// contractType := getMiningContractType(reqID, did, toSendData, creditsNeeded)
	// sc := contract.CreateNewMiningContract(contractType)
	// sc := contract.CreateNewContract(contractType)

	// err = sc.UpdateSignature(dc)
	// if err != nil {
	// 	c.log.Error(err.Error())
	// 	resp.Message = err.Error()
	// 	return resp
	// }
	//Create consensus request - calculate total pledge tokens required = 1RBT
	cr := getMiningConsensusRequest(2, c.peerID, miningEpoch, toSendData, did, "", "")

	td, _, pds, err := c.initiateMiningConsensus(cr, dc)

	fmt.Println(td, pds)
	// err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &toSendData, &br, true)

	//Post mining send details to explorer
	return nil
}

func (c *Core) initiateMiningConsensus(cr *MiningConensusRequest, dc did.DIDCrypto) (*model.TransactionDetails, map[string]map[string]float64, *PledgeDetails, error) {
	cs := ConsensusStatus{
		Credit: CreditScore{
			Credit: make([]CreditSignature, 0),
		},
		P: make(map[string]*ipfsport.Peer),
		Result: ConsensusResult{
			RunningCount:          0,
			SuccessCount:          0,
			FailedCount:           0,
			PledgingDIDSignStatus: false,
		},
	}
	reqPledgeTokens := float64(1)
	pd := PledgeDetails{
		TransferAmount:         reqPledgeTokens,
		RemPledgeTokens:        floatPrecision(reqPledgeTokens/2, MaxDecimalPlaces),
		NumPledgedTokens:       0,
		PledgedTokens:          make(map[string][]string),
		PledgedTokenChainBlock: make(map[string]interface{}),
		TokenList:              make([]Token, 0),
	}

	//getting last character from TID
	byteData, err := json.Marshal(cr.Tokens)
	if err != nil {
		c.log.Error("Failed to pledge token", "err", err)
		return nil, nil, nil, err
	}
	tid := util.HexToStr(util.CalculateHash(byteData, "SHA3-256"))
	lastCharTID := string(tid[len(tid)-1])
	cr.MiningTransactionID = tid

	//Select Quorums - for now - type 2
	ql := c.qm.GetQuorum(2, lastCharTID, c.peerID)

	c.qlock.Lock()
	c.quorumRequest[cr.ReqID] = &cs
	c.pd[cr.ReqID] = &pd
	c.qlock.Unlock()
	defer func() {
		c.qlock.Lock()
		delete(c.quorumRequest, cr.ReqID)
		delete(c.pd, cr.ReqID)
		c.qlock.Unlock()
	}()

	for _, a := range ql {
		go c.connectMiningQuorum(cr, a)
	}

	// TODO-Mining- figure out its use
	// loop := true
	// var err error
	// err = nil
	// for {
	// 	time.Sleep(time.Second)
	// 	c.qlock.Lock()
	// 	cs, ok := c.quorumRequest[cr.ReqID]
	// 	if !ok {
	// 		loop = false
	// 		err = fmt.Errorf("invalid request")
	// 	} else {
	// 		if cs.Result.SuccessCount >= MinConsensusRequired {
	// 			loop = false
	// 		} else if cs.Result.RunningCount == 0 {
	// 			loop = false
	// 			err = fmt.Errorf("consensus failed, retry transaction after sometimes")
	// 			c.log.Error("Consensus failed, retry transaction after sometimes")
	// 		}
	// 	}
	// 	c.qlock.Unlock()
	// 	if !loop {
	// 		break
	// 	}
	// }

	// nb, err := c.pledgeQuorumToken(cr, sc, tid, dc)
	// if err != nil {
	// 	c.log.Error("Failed to pledge token", "err", err)
	// 	return nil, nil, nil, err
	// }

	// ti := sc.GetTransTokenInfo()

	// TODO-Mining- figure out its use
	c.qlock.Lock()
	pds := c.pd[cr.ReqID]
	c.qlock.Unlock()
	pl := make(map[string]map[string]float64)
	for _, d := range cr.QuorumList {
		ds := strings.Split(d, ".")
		if len(ds) == 2 {
			ss, ok := pds.PledgedTokens[ds[1]]
			if ok {
				m := make(map[string]float64)
				for i := range ss {
					m[ss[i]] = 1
				}
				pl[ds[1]] = m
			}
		}
	}

	switch cr.Mode {

	case MiningMode:

		//Once everything is veryfied in quorumConsensus,
		//and quorums have pledged their tokens
		//quorums create the block while pledging and each quorum puts their signature on the genesis block.
		//A block is created, genesis block, in pledging. and quorums sign on the block here, and return it to the miner.
		//This will be sent to the explorer.
		//in pledge finality the quorum pledge block gets added.
		//update credit status
		//Add new token in tokens table.

		// 	rp, err := c.getPeer(cr.ReceiverPeerID+"."+sc.GetReceiverDID(), "")
		// 	if err != nil {
		// 		c.log.Error("Receiver not connected", "err", err)
		// 		return nil, nil, nil, err
		// 	}
		// 	defer rp.Close()
		// 	sr := SendTokenRequest{
		// 		Address:            cr.SenderPeerID + "." + sc.GetSenderDID(),
		// 		TokenInfo:          ti,
		// 		TokenChainBlock:    nb.GetBlock(),
		// 		QuorumList:         cr.QuorumList,
		// 		TransactionEpoch:   cr.TransactionEpoch,
		// 		PinningServiceMode: false,
		// 	}
		// 	var br model.BasicResponse
		// 	err = rp.SendJSONRequest("POST", APISendReceiverToken, nil, &sr, &br, true)
		// 	if err != nil {
		// 		c.log.Error("Unable to send tokens to receiver", "err", err)
		// 		return nil, nil, nil, err
		// 	}
		// 	if !br.Status {
		// 		c.log.Error("Unable to send tokens to receiver", "msg", br.Message)
		// 		return nil, nil, nil, fmt.Errorf("unable to send tokens to receiver, " + br.Message)
		// 	}
		// 	// br.Result will contain the new token state after sending tokens to receiver as a response to APISendReceiverToken
		// 	newtokenhashresult, ok := br.Result.([]interface{})
		// 	if !ok {
		// 		c.log.Error("Type assertion to string failed")
		// 		return nil, nil, nil, fmt.Errorf("Type assertion to string failed")
		// 	}
		// 	var newtokenhashes []string
		// 	for i, newTokenHash := range newtokenhashresult {
		// 		statehash, ok := newTokenHash.(string)
		// 		if !ok {
		// 			c.log.Error("Type assertion to string failed at index", i)
		// 			return nil, nil, nil, fmt.Errorf("Type assertion to string failed at index", i)
		// 		}
		// 		newtokenhashes = append(newtokenhashes, statehash)
		// 	}
		// 	fmt.Println("trans token initiate consensus : ", ti[0].Token) //TODO
		// 	//trigger pledge finality to the quorum and also adding the new tokenstate hash details for transferred tokens to quorum
		// 	pledgeFinalityError := c.quorumPledgeFinality(cr, nb, newtokenhashes, tid, weekCount)
		// 	if pledgeFinalityError != nil {
		// 		c.log.Error("Pledge finlaity not achieved", "err", err)
		// 		return nil, nil, nil, pledgeFinalityError
		// 	}
		// 	err = c.w.TokensTransferred(sc.GetSenderDID(), ti, nb, rp.IsLocal(), sr.PinningServiceMode)
		// 	if err != nil {
		// 		c.log.Error("Failed to transfer tokens", "err", err)
		// 		return nil, nil, nil, err
		// 	}
		// 	for _, t := range ti {
		// 		c.w.UnPin(t.Token, wallet.PrevSenderRole, sc.GetSenderDID())
		// 	}
		// 	//call ipfs repo gc after unpinnning
		// 	c.ipfsRepoGc()
		// 	nbid, err := nb.GetBlockID(ti[0].Token)
		// 	if err != nil {
		// 		c.log.Error("Failed to get block id", "err", err)
		// 		return nil, nil, nil, err
		// 	}
		// 	td := model.TransactionDetails{
		// 		TransactionID:   tid,
		// 		TransactionType: nb.GetTransType(),
		// 		BlockID:         nbid,
		// 		Mode:            wallet.SendMode,
		// 		SenderDID:       sc.GetSenderDID(),
		// 		ReceiverDID:     sc.GetReceiverDID(),
		// 		Comment:         sc.GetComment(),
		// 		DateTime:        time.Now(),
		// 		Status:          true,
		// 		Epoch:           int64(cr.TransactionEpoch),
		// 	}
		// 	err = c.initiateUnpledgingProcess(cr, td.TransactionID, td.Epoch)
		// 	if err != nil {
		// 		c.log.Error("Failed to store transactiond details with quorum ", "err", err)
		// 		return nil, nil, nil, err
		// 	}
		// 	return &td, pl, pds, nil
		// default:
		// 	err := fmt.Errorf("invalid consensus request mode: %v", cr.Mode)
		// 	c.log.Error(err.Error())
		// return nil, nil, nil, err
	}

	return nil, nil, nil, nil
}

func (c *Core) connectMiningQuorum(cr *MiningConensusRequest, addr string) {
	qt := AlphaQuorumType
	c.startConsensus(reqID, qt)
	var p *ipfsport.Peer
	var err error
	p, err = c.getPeer(addr, cr.MinerDID)

	if err != nil {
		c.log.Error(fmt.Sprintf("Failed to get peer connection while connecting to quorum address %v, err: %v", addr, err))
		c.finishConsensus(cr.ReqID, qt, nil, false, "", nil, nil)
		return
	} //TODO Mining - Alpha Quorum Type?

	err = c.initPledgeQuorumToken(cr, p, qt)

	if err != nil {
		if strings.Contains(err.Error(), "don't have enough balance to pledge") {
			c.log.Error("Quorum failed to pledge token")
			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
			return
		}
		c.log.Error("Failed to pledge token", "err", err)
		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
		return
	}
	//
	// var cresp ConensusReply
	// err = p.SendJSONRequest("POST", APIQuorumConsensus, nil, cr, &cresp, true, 10*time.Minute)
	//
	//	if err != nil {
	//		c.log.Error("Failed to get consensus", "err", err)
	//		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//		return
	//	}
	//
	//	if strings.Contains(cresp.Message, "parent token is not in burnt stage") {
	//		ptPrefix := "pt: "
	//		issueTypePrefix := "issueType: "
	//		// Find the starting indexes of pt and issueType values
	//		ptStart := strings.Index(cresp.Message, ptPrefix) + len(ptPrefix)
	//		issueTypeStart := strings.Index(cresp.Message, issueTypePrefix) + len(issueTypePrefix)
	//		// Extracting the substrings from the message
	//		pt := cresp.Message[ptStart : strings.Index(cresp.Message[ptStart:], ",")+ptStart]
	//		issueType := cresp.Message[issueTypeStart:]
	//		c.log.Debug("String: pt is ", pt, " issuetype is ", issueType)
	//		c.log.Debug("sc.GetSenderDID()", sc.GetSenderDID(), "pt", pt)
	//		orphanChildTokenList, err1 := c.w.GetChildToken(sc.GetSenderDID(), pt)
	//		if err1 != nil {
	//			c.log.Error("Consensus failed due to orphan child token ", "err", err1)
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//		issueTypeInt, err2 := strconv.Atoi(issueType)
	//		c.log.Debug("issue type in int is ", issueTypeInt)
	//		if err2 != nil {
	//			c.log.Error("Consensus failed due to orphan child token, issueType string conversion", "err", err2)
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//		c.log.Debug("Orphan token list ", orphanChildTokenList)
	//		if issueTypeInt == ParentTokenNotBurned {
	//			for _, orphanChild := range orphanChildTokenList {
	//				orphanChild.TokenStatus = wallet.TokenIsOrphaned
	//				c.log.Debug("Orphan token list status updated", orphanChild)
	//				c.w.UpdateToken(&orphanChild)
	//			}
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//	}
	//
	//	if strings.Contains(cresp.Message, "failed to sync tokenchain") {
	//		tokenPrefix := "Token: "
	//		issueTypePrefix := "issueType: "
	//		// Find the starting indexes of pt and issueType values
	//		ptStart := strings.Index(cresp.Message, tokenPrefix) + len(tokenPrefix)
	//		issueTypeStart := strings.Index(cresp.Message, issueTypePrefix) + len(issueTypePrefix)
	//		// Extracting the substrings from the message
	//		token := cresp.Message[ptStart : strings.Index(cresp.Message[ptStart:], ",")+ptStart]
	//		issueType := cresp.Message[issueTypeStart:]
	//		c.log.Debug("String: token is ", token, " issuetype is ", issueType)
	//		issueTypeInt, err1 := strconv.Atoi(issueType)
	//		if err1 != nil {
	//			c.log.Error("Consensus failed due to token chain sync issue, issueType string conversion", "err", err1)
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//		c.log.Debug("issue type in int is ", issueTypeInt)
	//		syncIssueTokenDetails, err2 := c.w.ReadToken(token)
	//		if err2 != nil {
	//			c.log.Error("Consensus failed due to tokenchain sync issue ", "err", err2)
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//		c.log.Debug("sync issue token details ", syncIssueTokenDetails)
	//		if issueTypeInt == TokenChainNotSynced {
	//			syncIssueTokenDetails.TokenStatus = wallet.TokenChainSyncIssue
	//			c.log.Debug("sync issue token details status updated", syncIssueTokenDetails)
	//			c.w.UpdateToken(syncIssueTokenDetails)
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//	}
	//
	//	if strings.Contains(cresp.Message, "Token state is exhausted, Token is being Double spent.") {
	//		tokenPrefix := "Token : "
	//		tStart := strings.Index(cresp.Message, tokenPrefix) + len(tokenPrefix)
	//		var token string
	//		if tStart >= len(tokenPrefix) {
	//			token = cresp.Message[tStart:]
	//			c.log.Debug("Token is being Double spent. Token is ", token)
	//		}
	//		doubleSpendTokenDetails, err2 := c.w.ReadToken(token)
	//		if err2 != nil {
	//			c.log.Error("Consensus failed due to token being double spent ", "err", err2)
	//			c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//			return
	//		}
	//		c.log.Debug("Double spend token details ", doubleSpendTokenDetails)
	//		doubleSpendTokenDetails.TokenStatus = wallet.TokenIsBeingDoubleSpent
	//		c.log.Debug("Double spend token details status updated", doubleSpendTokenDetails)
	//		c.w.UpdateToken(doubleSpendTokenDetails)
	//		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//		return
	//	}
	//
	//	if !cresp.Status {
	//		c.log.Error("Failed to get consensus", "msg", cresp.Message)
	//		c.finishConsensus(cr.ReqID, qt, p, false, "", nil, nil)
	//		return
	//	}
	//
	// c.finishConsensus(cr.ReqID, qt, p, true, cresp.Hash, cresp.ShareSig, cresp.PrivSig)
}

// func getMiningContractType(reqID string, requestingDID string, creditsInfo []model.ToSend, creditsNeeded int) *contract.MiningContractType {
// 	return &contract.MiningContractType{
// 		Type:       contract.SCMiningType,
// 		PledgeMode: contract.PeriodicPledgeMode,
// 		MiningInfo: &contract.MiningInfo{
// 			RequestingDID: requestingDID,
// 			CreditsInfo:   creditsInfo,
// 			CreditsNeeded: creditsNeeded,
// 		},
// 		ReqID: reqID,
// 	}
// }

func getMiningConsensusRequest(consensusRequestType int, minerPeerID string, miningEpoch int, tokens []model.ToSend, did string, password string, comment string) *MiningConensusRequest {
	var consensusRequest *MiningConensusRequest = &MiningConensusRequest{
		ReqID:       uuid.New().String(),
		Mode:        MiningMode,
		Type:        consensusRequestType,
		MinerPeerID: minerPeerID,
		MiningEpoch: miningEpoch,
		Tokens:      tokens,
		MinerDID:    did,
		Comment:     comment,
	}

	return consensusRequest
}
