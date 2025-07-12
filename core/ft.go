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
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/rac"
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
	defer c.w.ReleaseTokens(wholeTokens)
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
	FT, err := c.w.GetFTsAndCount(did)
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
	br := c.initiateFTTransfer(reqID, req)
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
