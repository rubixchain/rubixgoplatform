package core

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) CreateFTs(reqID string, did string, ftcount int, ftname string, wholeToken float64) {
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		c.log.Error("Failed to setup DID")
		return
	}
	err = c.createFTs(dc, ftname, ftcount, wholeToken, did)
	br := model.BasicResponse{
		Status:  true,
		Message: "DID registered successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	channel := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	channel.OutChan <- &br
}

func (c *Core) createFTs(dc did.DIDCrypto, FTName string, numFTs int, numWholeTokens float64, did string) error {
	if dc == nil {
		return fmt.Errorf("DID crypto is not initialized")
	}

	// Validate input parameters
	if numFTs <= 0 {
		return fmt.Errorf("number of tokens to create must be greater than zero")
	}
	if numWholeTokens <= 0 {
		return fmt.Errorf("number of whole tokens must be greater than zero")
	}
	if numFTs > int(numWholeTokens*1000) {
		return fmt.Errorf("Max allowed FT count is 1000 for 1 RBT")
	}

	// Fetch whole tokens using GetToken
	wholeTokens, err := c.w.GetTokens(did, float64(numWholeTokens))
	if err != nil || wholeTokens == nil {
		c.log.Error("Failed to fetch whole token for FT creation")
		return err
	}
	//TODO: Need to test and verify whether tokens are getiing unlocked if there is an error in creating FT.
	defer c.w.ReleaseTokens(wholeTokens)
	fractionalValue, err := c.GetPresiceFractionalValue(len(wholeTokens), numFTs)
	if err != nil {
		c.log.Error("Failed to calculate FT token value", err)
		return err
	}

	newFTs := make([]wallet.FTToken, 0, numFTs)
	newFTTokenIDs := make([]string, numFTs)

	var parentTokenIDsArray []string
	for _, token := range wholeTokens {
		parentTokenIDsArray = append(parentTokenIDsArray, token.TokenID)
	}
	parentTokenIDs := strings.Join(parentTokenIDsArray, ",")
	for i := 0; i < numFTs; i++ {
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

		// racBlockData := racBlocks[0].GetBlock()
		// fr := bytes.NewBuffer(racBlockData)

		ftnumString := strconv.Itoa(i)
		parts := []string{FTName, ftnumString}
		result := strings.Join(parts, "")
		byteArray := []byte(result)
		ftBuffer := bytes.NewBuffer(byteArray)
		ftID, err := c.w.Add(ftBuffer, did, wallet.AddFunc)
		if err != nil {
			c.log.Error("Failed to create FT, Failed to add token to IPFS", "err", err)
			return err
		}
		fmt.Println("ft ID is ", ftID)
		newFTTokenIDs[i] = ftID
		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     ftID,
					TokenType: c.TokenType(FTString),
				},
			},
			Comment: "FT generated at : " + time.Now().String(),
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
			c.log.Error("Failed to create FT, failed to add token chan block", "err", err)
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
			TransactionType: block.TokenBurntType,
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
		wholeTokens[i].TokenStatus = wallet.TokenIsBurnt
		err = c.w.UpdateToken(&wholeTokens[i])
		if err != nil {
			c.log.Error("FT token creation failed, failed to update token status", "err", err)
			return err
		}
		release = false
	}

	for i := range newFTs {
		ft := &newFTs[i]
		fmt.Println("ft is ", ft)
		err = c.w.CreateFT(ft)
		if err != nil {
			c.log.Error("Failed to create fractional token", "err", err)
			return err
		}
	}
	c.updateFTTable()
	return nil
}

func (c *Core) GetFTInfo() ([]model.FTInfo, error) {
	FT, err := c.w.GetAllFreeFTs()
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens", "err", err)
		return []model.FTInfo{}, fmt.Errorf("failed to get tokens")
	}
	ftNameCounts := make(map[string]int)

	ftCount := 0
	for _, t := range FT {
		ftCount++
		ftNameCounts[t.FTName]++
	}
	info := make([]model.FTInfo, 0, len(ftNameCounts))
	for name, count := range ftNameCounts {
		info = append(info, model.FTInfo{
			FTName:  name,
			FTCount: count,
		})
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
	if req.FTCount < 0 {
		resp.Message = "Input transaction amount is less than minimum FT transaction amount"
		return resp
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	FTs := make([]wallet.FTToken, 0)
	AllFTs, err := c.w.GetFreeFTsByName(req.FTName, did)
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
	if len(FTsForTxn) != 0 {
		FTs = append(FTs, FTsForTxn...)
	}

	//TODO: Pinning of tokens

	p, err := c.getPeer(req.Receiver, "")
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer p.Close()

	FTTokenIDs := make([]string, 0)
	for i := range FTsForTxn {
		FTTokenIDs = append(FTTokenIDs, FTsForTxn[i].TokenID)
	}
	TokenInfo := make([]contract.TokenInfo, 0)
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
		Mode:           FTTrasnferMode,
		ReqID:          uuid.New().String(),
		Type:           req.Type,
		SenderPeerID:   c.peerID,
		ReceiverPeerID: rpeerid,
		ContractBlock:  sc.GetBlock(),
		FTinfo:         FTData,
	}
	td, _, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	td.Amount = float64(req.FTCount)
	td.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(td)
	etrans := &ExplorerTrans{
		TID:         td.TransactionID,
		SenderDID:   did,
		ReceiverDID: rdid,
		Amount:      float64(req.FTCount),
		TrasnType:   req.Type,
		TokenIDs:    FTTokenIDs,
		QuorumList:  cr.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
	}
	c.ec.ExplorerTransaction(etrans)
	c.log.Info("FT Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
	resp.Status = true
	msg := fmt.Sprintf("FT Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
	resp.Message = msg
	c.updateFTTable()
	return resp
}

func (c *Core) GetPresiceFractionalValue(a, b int) (float64, error) {
	if b == 0 {
		return 0, errors.New("FT count should not be zero")

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
	fmt.Println("Updating FT table...")

	AllFTs, err := c.w.GetFTsAndCount()
	if err != nil {
		c.log.Error("Failed to get FTs and Count")
		return err
	}

	for _, Ft := range AllFTs {
		err = c.s.Update(wallet.FTStorage, &Ft, "ft_name=?", Ft.FTName)
		if err != nil {
			c.log.Error("Failed to update FT:", Ft.FTName, "Error:", err)
			return err
		}
	}

	return nil
}
