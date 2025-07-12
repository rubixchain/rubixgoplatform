package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) CreateFTs(reqID string, did string, ftcount int, ftname string, wholeToken int, ftNumStartIndex int) {
	err := c.createFTs(reqID, ftname, ftcount, wholeToken, did, ftNumStartIndex)
	br := model.BasicResponse{
		Status:  true,
		Message: "FT created successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	channel := c.GetWebReq(reqID)
	if channel == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	channel.OutChan <- &br
}

func (c *Core) createFTs(reqID string, FTName string, numFTs int, numWholeTokens int, did string, ftNumStartIndex int) error {
	if did == "" {
		c.log.Error("DID is empty")
		return fmt.Errorf("DID is empty")
	}
	isAlphanumericDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !isAlphanumericDID || !strings.HasPrefix(did, "bafybmi") || len(did) != 59 {
		c.log.Error("Invalid FT creator's DID. Please provide valid DID")
		return fmt.Errorf("Invalid DID, Please provide valid DID")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil || dc == nil {
		c.log.Error("Failed to setup DID")
		return fmt.Errorf("DID crypto is not initialized, err: %v ", err)
	}

	var FT []wallet.FT

	c.s.Read(wallet.FTStorage, &FT, "ft_name=? AND creator_did=?", FTName, did)

	if len(FT) != 0 {
		c.log.Error("FT Name already exists")
		return fmt.Errorf("FT Name already exists")
	}

	// Validate input parameters

	switch {
	case numFTs <= 0:
		return fmt.Errorf("number of tokens to create must be greater than zero")
	case numWholeTokens <= 0:
		return fmt.Errorf("number of whole tokens must be a positive integer")
	case numFTs > int(numWholeTokens*1000):
		return fmt.Errorf("max allowed FT count is 1000 for 1 RBT")
	}

	// Fetch whole tokens using GetToken
	wholeTokens, err := c.GetTokens(dc, did, float64(numWholeTokens), 0)
	if err != nil || wholeTokens == nil {
		c.log.Error("Failed to fetch whole token for FT creation")
		return err
	}
	//TODO: Need to test and verify whether tokens are getiing unlocked if there is an error in creating FT.
	defer c.w.ReleaseTokens(wholeTokens, c.testNet)
	fractionalValue, err := c.GetPresiceFractionalValue(int(numWholeTokens), numFTs)
	if err != nil {
		c.log.Error("Failed to calculate FT token value", err)
		return err
	}

	newFTs := make([]wallet.FTToken, 0, numFTs)
	var newFTTokenIDs []string

	var parentTokenIDsArray []string
	for _, token := range wholeTokens {
		parentTokenIDsArray = append(parentTokenIDsArray, token.TokenID)
	}
	parentTokenIDs := strings.Join(parentTokenIDsArray, ",")
	for i := ftNumStartIndex; i < ftNumStartIndex+numFTs; i++ {
		racType := &rac.RacType{
			Type:        c.RACFTType(),
			DID:         did,
			TokenNumber: uint64(i),
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
			FTInfo: &rac.RacFTInfo{
				Parents: parentTokenIDs,
				FTNum:   i,
				FTName:  FTName,
				FTValue: fractionalValue,
			},
		}

		// Create the RAC block
		racBlocks, err := rac.CreateRac(racType)
		if err != nil {
			c.log.Error("Failed to create RAC block", "err", err)
			return err
		}

		if len(racBlocks) != 1 {
			return fmt.Errorf("failed to create RAC block")
		}

		// Update the signature of the RAC block
		err = racBlocks[0].UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update DID signature", "err", err)
			return err
		}

		ftnumString := strconv.Itoa(i)
		parts := []string{FTName, ftnumString, did}
		result := strings.Join(parts, " ")
		byteArray := []byte(result)
		ftBuffer := bytes.NewBuffer(byteArray)
		ftID, err := c.w.Add(ftBuffer, did, wallet.AddFunc)
		if err != nil {
			c.log.Error("Failed to create FT, Failed to add token to IPFS", "err", err)
			return err
		}
		c.log.Info("FT created: " + ftID + " FT Num: " + ftnumString)
		newFTTokenIDs = append(newFTTokenIDs, ftID)

		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     ftID,
					TokenType: c.TokenType(FTString),
				},
			},
			Comment: "FT generated at : " + time.Now().String() + " for FT Name : " + FTName,
		}
		tcb := &block.TokenChainBlock{
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      did,
			TransInfo:       bti,
			GenesisBlock: &block.GenesisBlock{
				Info: []block.GenesisTokenInfo{
					{
						Token:       ftID,
						ParentID:    parentTokenIDs,
						TokenNumber: i,
					},
				},
			},
			TokenValue: fractionalValue,
		}
		ctcb := make(map[string]*block.Block)
		ctcb[ftID] = nil
		block := block.CreateNewBlock(ctcb, tcb)
		if block == nil {
			return fmt.Errorf("failed to create new block")
		}
		err = block.UpdateSignature(dc)
		if err != nil {
			c.log.Error("FT creation failed, failed to update signature", "err", err)
			return err
		}
		err = c.w.AddTokenBlock(ftID, block)
		if err != nil {
			c.log.Error("Failed to create FT, failed to add token chain block", "err", err)
			return err
		}
		// Create the new token
		ft := &wallet.FTToken{
			TokenID:     ftID,
			FTName:      FTName,
			TokenStatus: wallet.TokenIsFree,
			TokenValue:  fractionalValue,
			DID:         did,
		}
		newFTs = append(newFTs, *ft)
	}

	for i := range wholeTokens {

		release := true
		defer c.relaseToken(&release, wholeTokens[i].TokenID)
		ptts := RBTString
		if wholeTokens[i].ParentTokenID != "" && wholeTokens[i].TokenValue < 1 {
			ptts = PartString
		}
		ptt := c.TokenType(ptts)

		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     wholeTokens[i].TokenID,
					TokenType: ptt,
				},
			},
			Comment: "Token burnt at : " + time.Now().String(),
		}
		tcb := &block.TokenChainBlock{
			TransactionType: block.TokenIsBurntForFT,
			TokenOwner:      did,
			TransInfo:       bti,
			TokenValue:      wholeTokens[i].TokenValue,
			ChildTokens:     newFTTokenIDs,
		}
		ctcb := make(map[string]*block.Block)
		ctcb[wholeTokens[i].TokenID] = c.w.GetLatestTokenBlock(wholeTokens[i].TokenID, ptt)
		block := block.CreateNewBlock(ctcb, tcb)
		if block == nil {
			return fmt.Errorf("failed to create new block")
		}
		err = block.UpdateSignature(dc)
		if err != nil {
			c.log.Error("FT creation failed, failed to update signature", "err", err)
			return err
		}
		err = c.w.AddTokenBlock(wholeTokens[i].TokenID, block)
		if err != nil {
			c.log.Error("FT creation failed, failed to add token block", "err", err)
			return err
		}
		wholeTokens[i].TokenStatus = wallet.TokenIsBurntForFT
		err = c.w.UpdateToken(&wholeTokens[i])
		if err != nil {
			c.log.Error("FT token creation failed, failed to update token status", "err", err)
			return err
		}
		release = false
	}

	for i := range newFTs {
		tt := c.TokenType(FTString)
		blk := c.w.GetGenesisTokenBlock(newFTs[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get gensis block for Parent DID updation, invalid token chain")
			return err
		}
		FTOwner := blk.GetOwner()
		ft := &newFTs[i]
		ft.CreatorDID = FTOwner
		err = c.w.CreateFT(ft)
		if err != nil {
			c.log.Error("Failed to write FT details in FT tokens table", "err", err)
			return err
		}
	}
	updateFTTableErr := c.updateFTTable()
	if updateFTTableErr != nil {
		c.log.Error("Failed to update FT table after FT creation", "err", err)
		return updateFTTableErr
	}
	return nil
}

func (c *Core) GetFTInfoByDID(did string) ([]model.FTInfo, error) {
	if !c.w.IsDIDExist(did) {
		c.log.Error("DID does not exist")
		return nil, fmt.Errorf("DID does not exist")
	}
	FT, err := c.w.GetFTsAndCountByStatus(did, wallet.TokenIsFree)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens FTs and Count", "err", err)
		return []model.FTInfo{}, fmt.Errorf("Failed to get tokens FTs and Count")
	}
	ftInfoMap := make(map[string]map[string]int)

	// Iterate through retrieved FTs and populate the map
	for _, t := range FT {
		if ftInfoMap[t.FTName] == nil {
			ftInfoMap[t.FTName] = make(map[string]int) // Initialize map for each FTName
		}
		ftInfoMap[t.FTName][t.CreatorDID] += t.FTCount // Increment count for the specific CreatorDID
	}
	info := make([]model.FTInfo, 0)
	for ftName, creatorCounts := range ftInfoMap {
		for creatorDID, count := range creatorCounts {
			info = append(info, model.FTInfo{
				FTName:     ftName,
				FTCount:    count,
				CreatorDID: creatorDID,
			})
		}
	}
	return info, nil
}

func (c *Core) GetSpendableFTInfoByDID(did string) ([]model.FTInfo, error) {
	if !c.w.IsDIDExist(did) {
		c.log.Error("DID does not exist")
		return nil, fmt.Errorf("DID does not exist")
	}
	FT, err := c.w.GetFTsAndCountByStatus(did, wallet.TokenIsSpendable)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens FTs and Count", "err", err)
		return []model.FTInfo{}, fmt.Errorf("Failed to get tokens FTs and Count")
	}
	ftInfoMap := make(map[string]map[string]int)

	// Iterate through retrieved FTs and populate the map
	for _, t := range FT {
		if ftInfoMap[t.FTName] == nil {
			ftInfoMap[t.FTName] = make(map[string]int) // Initialize map for each FTName
		}
		ftInfoMap[t.FTName][t.CreatorDID] += t.FTCount // Increment count for the specific CreatorDID
	}
	info := make([]model.FTInfo, 0)
	for ftName, creatorCounts := range ftInfoMap {
		for creatorDID, count := range creatorCounts {
			info = append(info, model.FTInfo{
				FTName:     ftName,
				FTCount:    count,
				CreatorDID: creatorDID,
			})
		}
	}
	return info, nil
}

func (c *Core) GetFreeAndSpendableFTInfoByDID(did string) ([]model.FTInfo, error) {
	if !c.w.IsDIDExist(did) {
		c.log.Error("DID does not exist")
		return nil, fmt.Errorf("DID does not exist")
	}
	FT, err := c.w.GetFTsAndCountByStatus(did, -1)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens FTs and Count", "err", err)
		return []model.FTInfo{}, fmt.Errorf("Failed to get tokens FTs and Count")
	}
	ftInfoMap := make(map[string]map[string]int)

	// Iterate through retrieved FTs and populate the map
	for _, t := range FT {
		if ftInfoMap[t.FTName] == nil {
			ftInfoMap[t.FTName] = make(map[string]int) // Initialize map for each FTName
		}
		ftInfoMap[t.FTName][t.CreatorDID] += t.FTCount // Increment count for the specific CreatorDID
	}
	info := make([]model.FTInfo, 0)
	for ftName, creatorCounts := range ftInfoMap {
		for creatorDID, count := range creatorCounts {
			info = append(info, model.FTInfo{
				FTName:     ftName,
				FTCount:    count,
				CreatorDID: creatorDID,
			})
		}
	}
	return info, nil
}

func (c *Core) InitiateFTTransfer(reqID string, req *model.TransferFTReq) {
	br := c.ftTransfer(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateFTTransfer(reqID string, req *model.TransferFTReq) *model.BasicResponse {
	st := time.Now()
	txEpoch := int(st.Unix())
	resp := &model.BasicResponse{
		Status: false,
	}
	if req.Sender == req.Receiver {
		c.log.Error("Sender and receiver cannot same")
		resp.Message = "Sender and receiver cannot be same"
		return resp
	}
	if req.Sender == "" || req.Receiver == "" {
		c.log.Error("Sender and receiver cannot be empty")
		resp.Message = "Sender and receiver cannot be empty"
		return resp
	}
	isAlphanumericSender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.Sender)
	isAlphanumericReceiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.Receiver)
	if !isAlphanumericSender || !isAlphanumericReceiver {
		c.log.Error("Invalid sender or receiver address. Please provide valid DID")
		resp.Message = "Invalid sender or receiver address. Please provide valid DID"
		return resp
	}
	if !strings.HasPrefix(req.Sender, "bafybmi") || len(req.Sender) != 59 || !strings.HasPrefix(req.Receiver, "bafybmi") || len(req.Receiver) != 59 {
		c.log.Error("Invalid sender or receiver DID")
		resp.Message = "Invalid sender or receiver DID"
		return resp
	}
	_, did, ok := util.ParseAddress(req.Sender)
	if !ok {
		c.log.Error("Failed to parse sender DID")
		resp.Message = "Invalid sender DID"
		return resp
	}

	rpeerid, rdid, ok := util.ParseAddress(req.Receiver)
	if !ok {
		c.log.Error("Failed to parse receiver DID")
		resp.Message = "Invalid receiver DID"
		return resp
	}
	if req.FTCount <= 0 {
		c.log.Error("Input transaction amount is less than minimum FT transaction amount")
		resp.Message = "Invalid FT count"
		return resp
	}
	if req.FTName == "" {
		c.log.Error("FT name cannot be empty")
		resp.Message = "FT name is required"
		return resp
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		c.log.Error("Failed to setup DID")
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	var creatorDID string
	if req.CreatorDID == "" {
		// Checking for same FTs with different creators
		info, err := c.GetFTInfoByDID(did)
		if err != nil || info == nil {
			c.log.Error("Failed to get FT info for transfer", "err", err)
			resp.Message = "Failed to get FT info for transfer"
			return resp
		}
		ftNameToCreators := make(map[string][]string)
		for _, ft := range info {
			ftNameToCreators[ft.FTName] = append(ftNameToCreators[ft.FTName], ft.CreatorDID)
		}
		for ftName, creators := range ftNameToCreators {
			if len(creators) > 1 {
				c.log.Error(fmt.Sprintf("There are same FTs '%s' with different creators.", ftName))
				for i, creator := range creators {
					c.log.Error(fmt.Sprintf("Creator DID %d: %s", i+1, creator))
				}
				c.log.Info("Use -creatorDID flag to specify the creator DID and can proceed for transfer")
				resp.Message = "There are same FTs with different creators, use -creatorDID flag to specify creatorDID"
				return resp
			}
		}
		if info != nil && len(info) > 0 {
			creatorDID = info[0].CreatorDID
		}
	}
	var AllFTs []wallet.FTToken
	if req.CreatorDID != "" {
		AllFTs, err = c.w.GetFreeFTsByNameAndCreatorDID(req.FTName, did, req.CreatorDID)
		creatorDID = req.CreatorDID
	} else {
		AllFTs, err = c.w.GetFreeFTsByNameAndDID(req.FTName, did)
	}
	AvailableFTCount := len(AllFTs)
	if err != nil {
		c.log.Error("Failed to get FTs", "err", err)
		resp.Message = "Insufficient FTs or FTs are locked or " + err.Error()
		return resp
	} else {
		if req.FTCount > AvailableFTCount {
			c.log.Error(fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount))
			resp.Message = fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount)
			return resp
		}
	}
	FTsForTxn := AllFTs[:req.FTCount]
	//TODO: Pinning of tokens

	// Fetching peer's peer id
	peerInfo, err := c.GetPeerDIDInfo(req.Receiver)
	if err != nil {
		if peerInfo == nil {
			c.log.Error("could not get peerId of receiver ", req.Receiver, "error", err)
			resp.Message = fmt.Sprintf("could not get peerId of receiver : %v, error: %v", req.Receiver, err)
			return resp
		}
		if strings.Contains(err.Error(), "retry") {
			c.AddPeerDetails(*peerInfo)
		}
	}
	if peerInfo.PeerID == "" {
		c.log.Error("failed to get peerId of receiver ", req.Receiver, "error", err)
		resp.Message = fmt.Sprintf("failed to get peerId of receiver : %v, error: %v", req.Receiver, err)
		return resp
	}

	receiverPeerID, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer receiverPeerID.Close()

	FTTokenIDs := make([]string, 0)
	for i := range FTsForTxn {
		FTTokenIDs = append(FTTokenIDs, FTsForTxn[i].TokenID)
	}
	TokenInfo := make([]contract.TokenInfo, 0)
	for i := range FTsForTxn {
		FTsForTxn[i].TokenStatus = wallet.TokenIsLocked
		lockFTErr := c.s.Update(wallet.FTTokenStorage, FTsForTxn, "ft_name=?", FTsForTxn[i].FTName)
		if lockFTErr != nil {
			c.log.Error("Failed to update FT token status", "err", lockFTErr)
			resp.Message = "Failed to update FT token status"
			return resp
		}
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(FTsForTxn[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		bid, err := blk.GetBlockID(FTsForTxn[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		ti := contract.TokenInfo{
			Token:      FTsForTxn[i].TokenID,
			TokenType:  tt,
			TokenValue: FTsForTxn[i].TokenValue,
			OwnerDID:   did,
			BlockID:    bid,
		}
		TokenInfo = append(TokenInfo, ti)
	}
	sct := &contract.ContractType{
		Type:       contract.SCFTType,
		PledgeMode: contract.PeriodicPledgeMode,
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			ReceiverDID: rdid,
			Comment:     req.Comment,
			TransTokens: TokenInfo,
		},
		ReqID: reqID,
	}
	FTData := model.FTInfo{
		FTName:  req.FTName,
		FTCount: req.FTCount,
	}
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	cr := &ConensusRequest{
		Mode:             FTTransferMode,
		ReqID:            uuid.New().String(),
		Type:             req.QuorumType,
		SenderPeerID:     c.peerID,
		ReceiverPeerID:   rpeerid,
		ContractBlock:    sc.GetBlock(),
		FTinfo:           FTData,
		TransactionEpoch: txEpoch,
	}

	resultChan := make(chan *model.BasicResponse, 1)

	// start transacion in go routine
	go func() {
		td, _, pds, FTconsErr := c.initiateConsensus(cr, sc, dc)
		if FTconsErr != nil {
			resp.Message = fmt.Sprintf("Consensus failed: %s", FTconsErr.Error())
			tokens := sc.GetTransTokenInfo()
			for _, token := range tokens {
				if token.Token == "" {
					continue
				}

				ftToken := &wallet.FTToken{}
				ReadFTErr := c.s.Read(wallet.FTTokenStorage, ftToken, "token_id=?", token.Token)
				if ReadFTErr != nil {
					c.log.Error("Failed to read FT token", "token", token.Token, "err", ReadFTErr)
					resp.Message = "Failed to read FT token"
					resp.Status = false
					resultChan <- resp
					return
				}

				if ftToken.TokenStatus == wallet.TokenIsLocked {
					ftToken.TokenStatus = wallet.TokenIsFree
					updateFTErr := c.s.Update(wallet.FTTokenStorage, ftToken, "token_id=?", token.Token)
					if updateFTErr != nil {
						c.log.Error("Failed to update FT token status", "token", token.Token, "err", updateFTErr)
						resp.Message = "Failed to update FT token status"
						resp.Status = false
						resultChan <- resp
						return
					}
				}
			}
			c.UpdateUserInfo([]string{did})
			resp.Status = false
			resultChan <- resp
			return
		}
		et := time.Now()
		dif := et.Sub(st)
		td.Amount = float64(req.FTCount)
		td.TotalTime = float64(dif.Milliseconds())
		if td.TotalTime < 0.00 {
			td.TotalTime = 0.00
		}
		if err := c.w.AddTransactionHistory(td); err != nil {
			errMsg := fmt.Sprintf("Error occured while adding FT transaction details: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return
		}
		AllTokens := make([]AllToken, len(FTsForTxn))
		for i := range FTsForTxn {
			tokenDetail := AllToken{}
			tokenDetail.TokenHash = FTsForTxn[i].TokenID
			tt := c.TokenType(FTString)
			blk := c.w.GetLatestTokenBlock(FTsForTxn[i].TokenID, tt)
			bid, _ := blk.GetBlockID(FTsForTxn[i].TokenID)

			blockNoPart := strings.Split(bid, "-")[0]
			// Convert the string part to an int
			blockNoInt, err := strconv.Atoi(blockNoPart)
			if err != nil {
				log.Printf("Error getting BlockID: %v", err)
				continue
			}
			tokenDetail.BlockNumber = blockNoInt
			tokenDetail.BlockHash = strings.Split(bid, "-")[1]

			AllTokens[i] = tokenDetail
		}

		eTrans := &ExplorerFTTrans{
			FTBlockHash:     AllTokens,
			CreatorDID:      creatorDID,
			SenderDID:       did,
			ReceiverDID:     rdid,
			FTName:          req.FTName,
			FTTransferCount: req.FTCount,
			Network:         req.QuorumType,
			FTSymbol:        "N/A",
			Comments:        req.Comment,
			TransactionID:   td.TransactionID,
			PledgeInfo:      PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
			QuorumList:      extractQuorumDID(cr.QuorumList),
			Amount:          FTsForTxn[0].TokenValue * float64(req.FTCount),
			FTTokenList:     FTTokenIDs,
		}
		c.log.Info("FT Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
		msg := fmt.Sprintf("FT Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
		resp.Status = true
		resp.Message = msg
		if strings.Contains(resp.Message, "with transaction id") {
			if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
				resp.Result = txID
			}
		}

		updateFTTableErr := c.updateFTTable()
		if updateFTTableErr != nil {
			c.log.Error("Failed to update FT table after transfer ", "err", updateFTTableErr)
			resp.Message = "Failed to update FT table after transfer"
			return
		}

		c.ec.ExplorerFTTransaction(eTrans)
		c.UpdateUserInfo([]string{did})
		// Send final transaction completion response if not already timed out
		select {
		case resultChan <- resp:
			// Successfully sent to resultChan
		default:
			// If no one is listening (already timed out), just log and exit
			c.log.Debug("FT Transaction completed but resultChan is not being read anymore")
		}

	}()
	select {
	case result := <-resultChan:
		// Transaction completed within 40s or failed
		c.log.Debug("FT transaction completed before 20 secs")
		return result

	case <-time.After(20 * time.Second):
		// Timeout occurred, return Transaction ID only
		c.log.Debug("FT transaction still processing with txn id ", cr.TransactionID)

		msg := fmt.Sprintf("FT Transaction is still processing, with transaction id %v ", cr.TransactionID)
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

func extractTransactionIDFromMessage(msg string) string {
	re := regexp.MustCompile(`[a-fA-F0-9]{64}`)
	return re.FindString(msg)
}

// FT transfer with CVR
func (c *Core) ftTransfer(reqID string, req *model.TransferFTReq) *model.BasicResponse {
	st := time.Now()
	txEpoch := int(st.Unix())
	resp := &model.BasicResponse{
		Status: false,
	}
	if req.Sender == req.Receiver {
		c.log.Error("Sender and receiver cannot same")
		resp.Message = "Sender and receiver cannot be same"
		return resp
	}
	if req.Sender == "" || req.Receiver == "" {
		c.log.Error("Sender and receiver cannot be empty")
		resp.Message = "Sender and receiver cannot be empty"
		return resp
	}
	isAlphanumericSender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.Sender)
	isAlphanumericReceiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.Receiver)
	if !isAlphanumericSender || !isAlphanumericReceiver {
		c.log.Error("Invalid sender or receiver address. Please provide valid DID")
		resp.Message = "Invalid sender or receiver address. Please provide valid DID"
		return resp
	}
	if !strings.HasPrefix(req.Sender, "bafybmi") || len(req.Sender) != 59 || !strings.HasPrefix(req.Receiver, "bafybmi") || len(req.Receiver) != 59 {
		c.log.Error("Invalid sender or receiver DID")
		resp.Message = "Invalid sender or receiver DID"
		return resp
	}
	_, senderDID, ok := util.ParseAddress(req.Sender)
	if !ok {
		c.log.Error("Failed to parse sender DID")
		resp.Message = "Invalid sender DID"
		return resp
	}

	_, rdid, ok := util.ParseAddress(req.Receiver)
	if !ok {
		c.log.Error("Failed to parse receiver DID")
		resp.Message = "Invalid receiver DID"
		return resp
	}
	if req.FTCount <= 0 {
		c.log.Error("Input transaction amount is less than minimum FT transaction amount")
		resp.Message = "Invalid FT count"
		return resp
	}
	if req.FTName == "" {
		c.log.Error("FT name cannot be empty")
		resp.Message = "FT name is required"
		return resp
	}
	dc, err := c.SetupDID(reqID, senderDID)
	if err != nil {
		c.log.Error("Failed to setup DID")
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	// var creatorDID string
	if req.CreatorDID == "" {
		// Checking for same FTs with different creators
		info, err := c.GetSpendableFTInfoByDID(senderDID)
		if err != nil || info == nil {
			c.log.Error("Failed to get FT info for transfer", "err", err)
			resp.Message = "Failed to get FT info for transfer"
			return resp
		}
		ftNameToCreators := make(map[string][]string)
		for _, ft := range info {
			ftNameToCreators[ft.FTName] = append(ftNameToCreators[ft.FTName], ft.CreatorDID)
		}
		for ftName, creators := range ftNameToCreators {
			if len(creators) > 1 {
				c.log.Error(fmt.Sprintf("There are same FTs '%s' with different creators.", ftName))
				for i, creator := range creators {
					c.log.Error(fmt.Sprintf("Creator DID %d: %s", i+1, creator))
				}
				c.log.Info("Use -creatorDID flag to specify the creator DID and can proceed for transfer")
				resp.Message = "There are same FTs with different creators, use -creatorDID flag to specify creatorDID"
				return resp
			}
		}
		// creatorDID = info[0].CreatorDID
	}
	var AllFTs []wallet.FTToken
	if req.CreatorDID != "" {
		AllFTs, err = c.w.GetSpendableFTsByNameAndCreatorDID(req.FTName, senderDID, req.CreatorDID)
		// creatorDID = req.CreatorDID
	} else {
		AllFTs, err = c.w.GetSpendableFTsByNameAndDID(req.FTName, senderDID)
	}
	AvailableFTCount := len(AllFTs)
	if err != nil {
		c.log.Error("Failed to get spendable FTs", "err", err)
		resp.Message = "Insufficient spendable FTs or FTs are locked or " + err.Error()
		return resp
	} else {
		if req.FTCount > AvailableFTCount {
			c.log.Error(fmt.Sprint("Insufficient balance, Available spendable FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount))
			resp.Message = fmt.Sprint("Insufficient balance, Available spendable FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount)
			return resp
		}
	}
	FTsForTxn := AllFTs[:req.FTCount]
	//TODO: Pinning of tokens

	// Fetching peer's peer id
	peerInfo, err := c.GetPeerDIDInfo(req.Receiver)
	if err != nil {
		if peerInfo == nil {
			c.log.Error("could not get peerId of receiver ", req.Receiver, "error", err)
			resp.Message = fmt.Sprintf("could not get peerId of receiver : %v, error: %v", req.Receiver, err)
			return resp
		}
		if strings.Contains(err.Error(), "retry") {
			c.AddPeerDetails(*peerInfo)
		}
	}
	if peerInfo.PeerID == "" {
		c.log.Error("failed to get peerId of receiver ", req.Receiver, "error", err)
		resp.Message = fmt.Sprintf("failed to get peerId of receiver : %v, error: %v", req.Receiver, err)
		return resp
	}

	c.log.Debug("******Receiver is:*********", req.Receiver)

	receiverPeerID, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer receiverPeerID.Close()

	FTTokenIDs := make([]string, 0)
	for i := range FTsForTxn {
		FTTokenIDs = append(FTTokenIDs, FTsForTxn[i].TokenID)
	}
	TokenInfo := make([]contract.TokenInfo, 0)
	selfTransferFTMap := make(map[string]struct{})

	for i := range FTsForTxn {
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(FTsForTxn[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		bid, err := blk.GetBlockID(FTsForTxn[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		ti := contract.TokenInfo{
			Token:      FTsForTxn[i].TokenID,
			TokenType:  tt,
			TokenValue: FTsForTxn[i].TokenValue,
			OwnerDID:   senderDID,
			BlockID:    bid,
		}
		TokenInfo = append(TokenInfo, ti)

		// gather tokens for self-transfer : check if the transToken exists in the self-transfer map,
		// if exists delete the token from the self-transfer map, if does not exist
		// then fetch the list of all tokens from the latest block, and
		// add all the tokens to the self-transfer map except the trans-token
		c.log.Debug("*********dealing with trans-token :", FTsForTxn[i].TokenID)

		_, exists := selfTransferFTMap[FTsForTxn[i].TokenID]
		if exists {
			c.log.Debug("&&&&& self-trans map before deleting : ", selfTransferFTMap)
			delete(selfTransferFTMap, FTsForTxn[i].TokenID)
			c.log.Debug("&&&&&&& self-trans map after deleting : ", selfTransferFTMap)
		} else {
			transTokensInBlock := blk.GetTransTokens()
			c.log.Debug("***********trans tokens in last block :", transTokensInBlock)
			for _, token := range transTokensInBlock {
				if token != FTsForTxn[i].TokenID {
					selfTransferFTMap[token] = struct{}{}
				}
			}
		}
	}
	sct := &contract.ContractType{
		Type:       contract.SCFTType,
		PledgeMode: contract.PeriodicPledgeMode,
		TransInfo: &contract.TransInfo{
			SenderDID:   senderDID,
			ReceiverDID: rdid,
			Comment:     req.Comment,
			TransTokens: TokenInfo,
		},
		ReqID: reqID,
	}
	// FTData := model.FTInfo{
	// 	FTName:  req.FTName,
	// 	FTCount: req.FTCount,
	// }
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	// exctract sender signature to add to block
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

	// create new cvr-1 block and send to receiver
	FTData := model.FTInfo{
		FTName:  req.FTName,
		FTCount: req.FTCount,
	}
	cvr1Resp := c.SendFTsToReceiver(sc, FTData, TokenInfo, FTsForTxn, txEpoch, receiverPeerID, senderSign)
	if !cvr1Resp.Status {
		errMsg := fmt.Sprintf("failed to send FTs to receiver, err : %v", cvr1Resp.Message)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	td := cvr1Resp.Result.(*model.TransactionDetails)

	// self transfer in cvr-1
	c.log.Debug("self transefr ft map ", selfTransferFTMap)
	selfTransferFTResponse := c.CreateSelfTransferFTContract(selfTransferFTMap, dc, req, txEpoch)
	if !selfTransferFTResponse.Status {
		errMsg := fmt.Sprintf("self transfer contract creation failed, err : %v", selfTransferFTResponse.Message)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	selfTransferFTContract := selfTransferFTResponse.Result.(*contract.Contract)

	et := time.Now()
	dif := et.Sub(st)
	td.Amount = float64(req.FTCount)
	td.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(td)

	// TODO :  Extra details regarding the FT need to added in the explorer
	// etrans := &ExplorerTrans{
	// 	TID:         td.TransactionID,
	// 	SenderDID:   senderDID,
	// 	ReceiverDID: rdid,
	// 	Amount:      float64(req.FTCount),
	// 	TrasnType:   req.QuorumType,
	// 	TokenIDs:    FTTokenIDs,
	// 	// QuorumList:  cr.QuorumList,
	// 	TokenTime: float64(dif.Milliseconds()),
	// }
	// explorerErr := c.ec.ExplorerTransaction(etrans)
	// if explorerErr != nil {
	// 	c.log.Error("Failed to send FT transaction to explorer ", "err", explorerErr)
	// }

	AllTokens := make([]AllToken, len(FTsForTxn))
	for i := range FTsForTxn {
		tokenDetail := AllToken{}
		tokenDetail.TokenHash = FTsForTxn[i].TokenID
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(FTsForTxn[i].TokenID, tt)
		bid, _ := blk.GetBlockID(FTsForTxn[i].TokenID)
		blockNoPart := strings.Split(bid, "-")[0]
		// Convert the string part to an int
		blockNoInt, err := strconv.Atoi(blockNoPart)
		if err != nil {
			log.Printf("Error getting BlockID: %v", err)
			continue
		}
		tokenDetail.BlockNumber = blockNoInt
		tokenDetail.BlockHash = strings.Split(bid, "-")[1]
		AllTokens[i] = tokenDetail
	}

	eTrans := &ExplorerFTTrans{
		FTBlockHash:     AllTokens,
		CreatorDID:      req.CreatorDID,
		SenderDID:       senderDID,
		ReceiverDID:     rdid,
		FTName:          req.FTName,
		FTTransferCount: req.FTCount,
		Network:         req.QuorumType,
		FTSymbol:        "N/A",
		Comments:        req.Comment,
		TransactionID:   td.TransactionID,
		// PledgeInfo:      PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		// QuorumList:      extractQuorumDID(cr.QuorumList),
		Amount:      FTsForTxn[0].TokenValue * float64(req.FTCount),
		FTTokenList: FTTokenIDs,
	}

	updateFTTableErr := c.updateFTTable()
	if updateFTTableErr != nil {
		c.log.Error("Failed to update FT table after transfer ", "err", updateFTTableErr)
		resp.Message = "Failed to update FT table after transfer"
		return resp
	}
	explorerErr := c.ec.ExplorerFTTransaction(eTrans)
	if explorerErr != nil {
		c.log.Error("Failed to send FT transaction to explorer ", "err", explorerErr)
	}

	// Starting CVR stage-2
	cvrRequest := &wallet.PrePledgeRequest{
		DID:             req.Sender,
		QuorumType:      req.QuorumType,
		TransferMode:    SpendableFTTransferMode,
		FTInfo:          FTData,
		SCTransferBlock: sc.GetBlock(),
		TxnEpoch:        int64(txEpoch),
		ReqID:           reqID,
	}
	if selfTransferFTContract != nil {
		cvrRequest.SCSelfTransferBlock = selfTransferFTContract.GetBlock()
	}

	// go-routine for cvr-2
	go func(cvrReq *wallet.PrePledgeRequest) {
		resp := c.initiateCVRTwo(cvrReq)
		c.log.Debug("response from CVR-2 : ", resp)
	}(cvrRequest)

	c.log.Info("FT Transfer in cvr-1 finished successfully", "duration", dif, " trnxid", td.TransactionID)
	msg := fmt.Sprintf("FT Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
	resp.Status = true
	resp.Message = msg
	return resp
}

// TODO : update received / transferred tokens
// create cvr-1 block and send FTs to receiver
func (c *Core) SendFTsToReceiver(sc *contract.Contract, ftData model.FTInfo, ftInfo []contract.TokenInfo, ftInfoForTable []wallet.FTToken, txEpoch int, receiverPeerID *ipfsport.Peer, senderSig *block.InitiatorSignature) *model.BasicResponse {
	resp := &model.BasicResponse{
		Status: false,
	}
	rpeerid := receiverPeerID.GetPeerID()
	// create block in cvr stage-1
	transactionID := util.HexToStr(util.CalculateHash(sc.GetBlock(), "SHA3-256"))

	c.log.Debug("*****sender to receiver transactionID***", transactionID)
	tks := make([]block.TransTokens, 0)
	ctcb := make(map[string]*block.Block)

	for i := range ftInfo {
		tt := block.TransTokens{
			Token:     ftInfo[i].Token,
			TokenType: ftInfo[i].TokenType,
		}
		tks = append(tks, tt)
		b := c.w.GetLatestTokenBlock(ftInfo[i].Token, ftInfo[i].TokenType)
		ctcb[ftInfo[i].Token] = b
	}

	bti := &block.TransInfo{
		Comment: sc.GetComment(),
		TID:     transactionID,
		Tokens:  tks,
	}

	tcb := block.TokenChainBlock{
		TransactionType:    block.SpendableTokenTransferredType,
		TokenOwner:         sc.GetReceiverDID(),
		TransInfo:          bti,
		SmartContract:      sc.GetBlock(),
		InitiatorSignature: senderSig,
		Epoch:              txEpoch,
	}
	c.log.Debug("****creating new FT transblock for sender to receiver transaction in cvr-1*********")
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new FT-token chain block")
		resp.Message = "failed to create new FT-token chain block"
		return resp
	}

	sr := SendFTRequest{
		Address:          c.peerID + "." + sc.GetSenderDID(),
		TokenInfo:        ftInfo,
		TokenChainBlock:  nb.GetBlock(),
		TransactionEpoch: txEpoch,
		FTInfo:           ftData,
		CVRStage:         wallet.CVRStage1_Sender_to_Receiver,
	}
	rp, err := c.getPeer(rpeerid + "." + sc.GetReceiverDID())
	if err != nil {
		c.log.Error("Receiver not connected", "err", err)
		resp.Message = "Receiver not connected" + err.Error()
		return resp
	}
	defer rp.Close()

	var br model.BasicResponse
	err = rp.SendJSONRequest("POST", APISendFTToken, nil, &sr, &br, true)
	if err != nil {
		c.log.Error("Unable to send FTs to receiver", "err", err)
		resp.Message = "Unable to send FTs to receiver: " + err.Error()
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
		resp.Message = fmt.Sprintf("Unable to send tokens to receiver, msg : %v", br.Message)
		return resp
	}

	// update token status based on the situations:
	// 1. sender receiver on different ports
	// 2. sender receiver on same port
	if rpeerid != c.peerID {
		c.log.Debug("*********** updating token status for sender's transferred FTs")
		err = c.UpdateTransferredFTsInfo(ftInfoForTable, wallet.TokenIsTransferred, transactionID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to update trans tokens in DB, err : %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
	} else {
		c.log.Debug("******sender peer and receiver's is same")
	}

	nbid, err := nb.GetBlockID(ftInfo[0].Token)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get block id of the new block, FT : %v; err : %v", ftInfo[0].Token, err)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}

	txnDetails := &model.TransactionDetails{
		TransactionID:   transactionID,
		TransactionType: nb.GetTransType(),
		BlockID:         nbid,
		Mode:            wallet.FTTransferMode,
		SenderDID:       sc.GetSenderDID(),
		ReceiverDID:     sc.GetReceiverDID(),
		Comment:         sc.GetComment(),
		DateTime:        time.Now(),
		Status:          true,
		Epoch:           int64(txEpoch),
	}

	resp.Result = txnDetails

	resp.Status = true
	resp.Message = "FT transferred successfully"
	c.log.Debug("******* FT cvr-1 completed ***********")
	return resp
}

// prepare self-transfer RBTs and create self-transfer contract block
func (c *Core) CreateSelfTransferFTContract(selfTransferFTsMap map[string]struct{}, dc did.DIDCrypto, req *model.TransferFTReq, txnEpoch int) *model.BasicResponse {
	resp := &model.BasicResponse{
		Status: false,
	}

	// get all self-transfer tokens,
	// the final self-transfer token list is : selfTransferTokensList
	var selfTransactionID string
	var selfTransferFTBlock *block.Block
	var selfTransferFTContract *contract.Contract
	var selfTransferFTCount float64
	selfTransferFTsList := make([]contract.TokenInfo, 0)
	lockedFTsForSelfTransfer := make([]wallet.FTToken, 0)

	for selfTransferFT := range selfTransferFTsMap {

		// check if the token is spendable, if not then do not include in self-transfer
		selftransferFTInfo, err := c.w.GetFTByDIDandStatus(selfTransferFT, req.Sender, wallet.TokenIsSpendable)
		if err != nil {
			// errMsg := fmt.Sprintf("failed to get token for self-transfer, token: %v, error: %v", selfTransferToken, err)
			// c.log.Error(errMsg)
			continue
			// c.w.UnlockLockedTokens(req.Sender, lockedTokensForSelfTransfer, c.testNet)
			// resp.Message = errMsg
			// return resp
		}

		ftTypeStr := FTString
		ftType := c.TokenType(ftTypeStr)
		blk := c.w.GetLatestTokenBlock(selfTransferFT, ftType)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			for _, selfTransFtInfo := range lockedFTsForSelfTransfer {

				c.w.UnlockFT(selfTransFtInfo.TokenID)
			}
			return resp
		}
		bid, err := blk.GetBlockID(selfTransferFT)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			for _, selfTransFtInfo := range lockedFTsForSelfTransfer {

				c.w.UnlockFT(selfTransFtInfo.TokenID)
			}
			return resp
		}

		selftransferFTinfo := contract.TokenInfo{
			Token:      selfTransferFT,
			TokenType:  ftType,
			TokenValue: selftransferFTInfo.TokenValue,
			OwnerDID:   req.Sender,
			BlockID:    bid,
		}
		selfTransferFTsList = append(selfTransferFTsList, selftransferFTinfo)
		lockedFTsForSelfTransfer = append(lockedFTsForSelfTransfer, *selftransferFTInfo)
		selfTransferFTCount = selfTransferFTCount + selftransferFTInfo.TokenValue

		// pinning self-transfer fts
		_, err = c.w.Pin(selftransferFTInfo.TokenID, wallet.OwnerRole, req.Sender, "TID-Not Generated", req.Sender, req.Sender, selftransferFTInfo.TokenValue)
		if err != nil {
			errMsg := fmt.Sprintf("failed to pin the token: %v, error: %v", selftransferFTInfo.TokenID, err)
			c.log.Error(errMsg)
		}
	}

	c.log.Debug("************ self transfer ft count : ", selfTransferFTCount)

	// if there are no available tokens for self-transfer then no need of self-transfer
	if len(selfTransferFTsList) != 0 {
		c.log.Debug("********** self transfer fts : ", selfTransferFTsList)
		//create new contract for self transfer
		selfTransferContractType := &contract.ContractType{
			Type:       contract.SCFTType,
			PledgeMode: contract.PeriodicPledgeMode,
			TransInfo: &contract.TransInfo{
				SenderDID:   req.Sender,
				ReceiverDID: req.Sender,
				// Comment:     req.Comment,
				TransTokens: selfTransferFTsList,
			},
			ReqID: reqID,
		}
		// c.log.Debug("***********Contract type for self transaction in cvr-1******* ", selfTransferContractType)

		c.log.Debug("*******creating the contract for self transaction in cvr-1*******")

		selfTransferFTContract = contract.CreateNewContract(selfTransferContractType)

		c.log.Debug("******** sender signing on self-transfer txn id")

		err := selfTransferFTContract.UpdateSignature(dc)
		if err != nil {
			errMsg := fmt.Sprintf("failed to update the signature on the self transfer contract, error: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}

		// creating self transfer block in cvr stage-1
		selfTransactionID = util.HexToStr(util.CalculateHash(selfTransferFTContract.GetBlock(), "SHA3-256"))

		c.log.Debug("trasactionID for self transaction", selfTransactionID)

		selfTransferTokens := make([]block.TransTokens, 0)
		latestTokenChainBlock := make(map[string]*block.Block)

		for i := range selfTransferFTsList {
			selfTransTokens := block.TransTokens{
				Token:     selfTransferFTsList[i].Token,
				TokenType: selfTransferFTsList[i].TokenType,
			}
			selfTransferTokens = append(selfTransferTokens, selfTransTokens)
			latestBlock := c.w.GetLatestTokenBlock(selfTransferFTsList[i].Token, selfTransferFTsList[i].TokenType)
			latestTokenChainBlock[selfTransferFTsList[i].Token] = latestBlock
		}

		selfTransBlockTransInfo := &block.TransInfo{
			Comment: selfTransferFTContract.GetComment(),
			TID:     selfTransactionID,
			Tokens:  selfTransferTokens,
		}

		signData, senderNLSSShare, senderPrivSign, err := selfTransferFTContract.GetHashSig(dc.GetDID())
		if err != nil {
			errMsg := fmt.Sprintf("failed to fetch sender sign, error: %v", err)
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
			TransactionType:    block.SpendableTokenTransferredType,     // cvr stage-1
			TokenOwner:         selfTransferFTContract.GetReceiverDID(), //ReceiverDID is same as req.Sender because it is self transfer
			TransInfo:          selfTransBlockTransInfo,
			SmartContract:      selfTransferFTContract.GetBlock(),
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

		selfTransferFTBlock = block.CreateNewBlock(latestTokenChainBlock, &selfTransferTCB)
		if selfTransferFTBlock == nil {
			c.log.Error("Failed to create a selftransfer token chain block - qrm init")
			resp.Message = "Failed to create a selftransfer token chain block - qrm init"
			return resp
		}
		//update token type
		var ok bool
		for _, t := range selfTransferFTsList {

			selfTransferFTBlock, ok = selfTransferFTBlock.UpdateTokenType(t.Token, token.CVR_RBTTokenType+t.TokenType)
			if !ok {
				errMsg := fmt.Sprintf("failed to update token cvr-1 type for self transfer block, error: %v", err)
				c.log.Error(errMsg)
				resp.Message = errMsg
				return resp
			}

		}

		//update the levelDB and SqliteDB
		updatedTokenStateHashesAfterSelfTransfer, err := c.w.FTTokensReceived(req.Sender, selfTransferFTsList, selfTransferFTBlock, c.peerID, c.peerID, c.ipfs, lockedFTsForSelfTransfer[0])
		if err != nil {
			errMsg := fmt.Sprintf("failed to add self-transfer token block and update status, error: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
		//TODO: Send these updated TokenStateHashes After SelfTransfer to quorums for pinning
		c.log.Debug(fmt.Sprintf("Updated token state hashes after self transfer are:%v", updatedTokenStateHashesAfterSelfTransfer))

	}

	resp.Result = selfTransferFTContract

	resp.Status = true
	resp.Message = "self-ytransfer contract created"
	return resp
}

func (c *Core) GetPresiceFractionalValue(a, b int) (float64, error) {
	if b == 0 || a == 0 {
		return 0, errors.New("RBT value or FT count should not be zero")
	}
	result := float64(a) / float64(b)
	decimalPlaces := len(strconv.FormatFloat(result, 'f', -1, 64)) - 2 // Subtract 2 for "0."

	if decimalPlaces > 3 {
		// Find the nearest possible value for b by checking from b-10 to b+10
		var nearestB int
		minDiff := math.MaxFloat64
		found := false

		for i := b - 10; i <= b+10; i++ {
			if i <= 0 {
				continue // Skip non-positive values of b
			}
			tempResult := float64(a) / float64(i)
			tempDecimalPlaces := len(strconv.FormatFloat(tempResult, 'f', -1, 64)) - 2

			if tempDecimalPlaces <= 3 {
				diff := math.Abs(result - tempResult)
				if diff < minDiff {
					minDiff = diff
					nearestB = i
					found = true
				}
			}
		}

		if found {
			return 0, fmt.Errorf("FT value exceeds 3 decimal places, nearest possible value for FT count is %d", nearestB)
		} else {
			return 0, fmt.Errorf("FT value exceeds 3 decimal places, no suitable b found within range")
		}
	}
	return result, nil
}

func (c *Core) updateFTTable() error {
	AllFTs, err := c.w.GetAllFTsAndCount()
	// If no records are found, remove all entries from the FT table
	if err != nil {
		fetchErr := fmt.Sprint(err)
		if strings.Contains(fetchErr, "no records found") {
			err = c.s.Delete(wallet.FTStorage, &wallet.FT{}, "ft_name!=?", "")
			if err != nil {
				deleteErr := fmt.Sprint(err)
				if strings.Contains(deleteErr, "no records found") {
					c.log.Info("FT table is empty")
				} else {
					c.log.Error("Failed to delete all entries from FT table:", err)
					return err
				}
			}
			return nil
		} else {
			c.log.Error("Failed to get FTs and Count")
			return err
		}
	}
	err = c.s.Delete(wallet.FTStorage, &wallet.FT{}, "ft_name!=?", "")
	ReadErr := fmt.Sprint(err)
	if err != nil {
		if strings.Contains(ReadErr, "no records found") {
			c.log.Info("FT table is empty")
		}
		c.log.Error("Failed to remove current FTs from storage to add new:", err)
		return err
	}
	for _, Ft := range AllFTs {
		addErr := c.s.Write(wallet.FTStorage, &Ft)
		if addErr != nil {
			c.log.Error("Failed to add new FT:", Ft.FTName, "Error:", addErr)
			return addErr
		}
	}
	return nil
}

func (c *Core) UnlockFTs() error {
	lockedFTs, err := c.w.GetLockedFTs()
	if err != nil {
		c.log.Error("Failed to get locked FTs", "err", err)
		return err
	}

	for _, ft := range lockedFTs {
		if ft.TokenID == "" {
			continue
		}

		ft.TokenStatus = wallet.TokenIsFree

		// First, delete the token
		err := c.s.Delete(wallet.FTTokenStorage, &wallet.FT{}, "token_id=?", ft.TokenID)
		if err != nil {
			c.log.Error("Failed to delete FT", "token_id", ft.TokenID, "err", err)
			continue
		}

		// Then, re-insert the same token — this moves it to the bottom (new rowid)
		err = c.s.Write(wallet.FTTokenStorage, &ft)
		if err != nil {
			c.log.Error("Failed to re-insert FT", "token_id", ft.TokenID, "err", err)
			continue
		}
	}
	c.log.Info("Unlocked FT")
	return nil
}

func (c *Core) UpdateTransferredFTsInfo(tokenList []wallet.FTToken, newTokenStatus int, txnID string) error {
	c.log.Debug("****** updating transferred FT info to", "new status ", newTokenStatus)
	for _, tokenInfo := range tokenList {
		tokenInfo.TokenStatus = newTokenStatus
		tokenInfo.TransactionID = txnID
		err := c.w.UpdateFTToken(&tokenInfo)
		if err != nil {
			errMsg := fmt.Sprintf("failed to update token : %v from Tokentable, err : %v", tokenInfo.TokenID, err)
			c.log.Error(errMsg)
			return fmt.Errorf(errMsg)
		}
		c.log.Debug("****** FT updated in db")
	}
	return nil
}

func (c *Core) InitiateFTCVRTwo(reqID string, req *model.CvrAPIRequest) {
	br := c.GatherFreeFTsForConsensus(reqID, req)
	c.log.Debug("***** received ft cvr-2 response : ", br)
	didChannel := c.GetWebReq(reqID)
	if didChannel == nil {
		c.log.Error("Failed to get did channels for FT cvr-2")
		return
	}
	c.log.Debug("!!!!!!!!!!!!!!!!!!! final response from cvr ", br)
	didChannel.OutChan <- br
}

// this function gathers all the required free tokens for CVR and creates a temp contract block for conensus
func (c *Core) GatherFreeFTsForConsensus(reqID string, req *model.CvrAPIRequest) *model.BasicResponse {

	c.log.Debug("****** receievd API request for CVR-2 : ", reqID, "request :", req)

	response := &model.BasicResponse{
		Status: false,
	}
	// gather free tokens for cvr and prepare contract block
	freeFTsList, err := c.w.GetFreeFTsByDID(req.DID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get all free ft tokens for DID : %v, err : %v", req.DID, err)
		c.log.Error(errMsg)
		response.Message = errMsg
		return response
	}

	c.log.Debug("************ length of list of free FTs : ", len(freeFTsList))

	if len(freeFTsList) == 0 {
		c.log.Error("No tokens present for cvr")
		response.Message = "No tokens present for cvr"
		return response
	}

	// release the locked tokens before exit
	defer c.w.ReleaseFTs(freeFTsList)

	segratedFtList, err := c.SegregateFtWithNameAndCreatorDID(freeFTsList)
	if err != nil {
		errMsg := fmt.Sprintf("failed to segregate all free tokens for DID : %v, err : %v", req.DID, err)
		c.log.Error(errMsg)
		response.Message = errMsg
		return response
	}
	// c.log.Debug("******* segregated FTs list : ", segratedFtList)

	cvrSuccessFtList := make([]string, 0)
	cvrFailureFtList := make([]string, 0)
	// initiate cvr-2 for each FT name and creator did, sequentially
	for ftNameAndCreatorDID, ftList := range segratedFtList {
		c.log.Debug("********** initiating cvr-2 for ft name and creator did : ", ftNameAndCreatorDID, "ft list: ", ftList)
		cvrResp := c.CreateFTContractAndInitiateCVrTwo(reqID, ftList, req.DID, req.QuorumType)
		if !cvrResp.Status {
			cvrFailureFtList = append(cvrFailureFtList, ftNameAndCreatorDID)
			errMsg := fmt.Sprintf("failed to initiate CVR-2 for ft name and creator did : %v; err : %v ", ftNameAndCreatorDID, cvrResp.Message)
			c.log.Error(errMsg)
		} else {
			cvrSuccessFtList = append(cvrSuccessFtList, ftNameAndCreatorDID)
		}
	}

	c.log.Debug("********cvr success FTs list length :", len(cvrSuccessFtList))

	if len(cvrSuccessFtList) > 0 {
		response.Status = true
		msg := fmt.Sprintf("cvr-2 completed for FTs (ftname-creatordid): %v, and failed for FTs (ftname-creatordid) : %v", cvrSuccessFtList, cvrFailureFtList)
		response.Message = msg
	} else {
		response.Message = "cvr-2 failed for all free FTs"
	}
	c.log.Debug("*******", response.Message)
	return response
}

// this function segregates FTs according to FTName and CreatorDID
func (c *Core) SegregateFtWithNameAndCreatorDID(ftList []wallet.FTToken) (map[string][]wallet.FTToken, error) {
	segragatedFTMap := make(map[string][]wallet.FTToken)
	for _, ftInfo := range ftList {
		ftNameAndCreatorDID := ftInfo.FTName + "-" + ftInfo.CreatorDID // key is "ftname-creatordid"
		segragatedFTMap[ftNameAndCreatorDID] = append(segragatedFTMap[ftNameAndCreatorDID], ftInfo)
	}
	return segragatedFTMap, nil
}

// this function creates contract block for cvr-2, initiator signs on the txn id, and initiates CVR-2
func (c *Core) CreateFTContractAndInitiateCVrTwo(reqId string, ftList []wallet.FTToken, initiatorDID string, quorumType int) *model.BasicResponse {
	response := &model.BasicResponse{
		Status: false,
	}
	dc, err := c.SetupDID(reqId, initiatorDID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to setup DID, err : %v", err)
		c.log.Error(errMsg)
		response.Message = errMsg
		return response
	}

	//TODO: handle the error in Pin func
	for i := range ftList {
		c.w.Pin(ftList[i].TokenID, wallet.OwnerRole, initiatorDID, "TID-Not Generated", initiatorDID, "", ftList[i].TokenValue)
	}

	tis := make([]contract.TokenInfo, 0)
	totalValue := 0

	for i := range ftList {
		tts := FTString
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(ftList[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			response.Message = "failed to get latest block, invalid token chain"
			return response
		}

		bid, err := blk.GetBlockID(ftList[i].TokenID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to get block id; err: %v", err)
			c.log.Error(errMsg)
			response.Message = errMsg
			return response
		}
		ti := contract.TokenInfo{
			Token:      ftList[i].TokenID,
			TokenType:  tt,
			TokenValue: floatPrecision(ftList[i].TokenValue, MaxDecimalPlaces),
			OwnerDID:   ftList[i].DID,
			BlockID:    bid,
		}
		tis = append(tis, ti)

		// calculate all free tokens sum
		totalValue += int(ftList[i].TokenValue)

	}

	// preaparing the block for tokens to be transferred to receiver
	contractType := &contract.ContractType{
		Type:       contract.SCFTType,
		PledgeMode: contract.PeriodicPledgeMode,
		TransInfo: &contract.TransInfo{
			SenderDID:   initiatorDID,
			ReceiverDID: initiatorDID,
			TransTokens: tis,
		},
		ReqID: reqId,
	}
	sc := contract.CreateNewContract(contractType)

	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		response.Message = err.Error()
		return response
	}

	st := time.Now()
	txEpoch := int(st.Unix())

	ftInfo := model.FTInfo{
		FTName:  ftList[0].FTName,
		FTCount: len(ftList),
	}

	cvrReq := &wallet.PrePledgeRequest{
		DID:                 initiatorDID,
		QuorumType:          quorumType,
		TransferMode:        SpendableFTTransferMode,
		SCSelfTransferBlock: sc.GetBlock(),
		SCTransferBlock:     nil,
		ReqID:               reqId,
		TxnEpoch:            int64(txEpoch),
		FTInfo:              ftInfo,
	}

	response = c.initiateCVRTwo(cvrReq)
	c.log.Debug("*******cvr-2 response :", response.Message)
	return response
}
