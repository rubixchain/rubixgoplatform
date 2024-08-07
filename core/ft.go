package core

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
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
	fmt.Println("FT name is ", FTName)
	fmt.Println("FT count is ", numFTs)
	fmt.Println("num Whole token is ", numWholeTokens)
	fmt.Println("DID is ", did)
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

	// Fetch whole tokens using GetToken
	wholeTokens, err := c.w.GetTokens(did, float64(numWholeTokens))
	if err != nil || wholeTokens == nil {
		c.log.Error("Failed to fetch whole token for FT creation")
		return err
	}

	// Calculate the value of each fractional token
	fractionalValue := float64(len(wholeTokens)) / float64(numFTs)

	newFTs := make([]wallet.FT, 0, numFTs)
	newFTTokenIDs := make([]string, numFTs)

	for i := 0; i < numFTs; i++ {
		parentTokenID := wholeTokens[i%len(wholeTokens)].TokenID
		racType := &rac.RacType{
			Type:        c.RACFTType(),
			DID:         did,
			TokenNumber: uint64(i),
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
			FTInfo: &rac.RacFTInfo{
				Parents: parentTokenID,
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
						ParentID:    parentTokenID,
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
		ft := &wallet.FT{
			TokenID:       ftID,
			FTName:        FTName,
			ParentTokenID: parentTokenID,
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
	return nil
}

func (c *Core) GetFTInfo() ([]model.FTInfo, error) {
	FT, err := c.w.GetAllFTs()
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
	fmt.Println(rpeerid, did, rdid, dc, st)

	return resp
}
