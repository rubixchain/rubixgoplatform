package core

import (
	"bytes"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
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

	// Fetch whole tokens using GetToken
	wholeTokens, err := c.w.GetTokens(did, float64(numWholeTokens))
	if err != nil || wholeTokens == nil {
		c.log.Error("Failed to fetch whole token for FT creation")
		return err
	}

	// Calculate the value of each fractional token
	fractionalValue := float64(len(wholeTokens)) / float64(numFTs)

	// Create a slice to hold the new tokens
	newFTs := make([]wallet.FT, 0, numFTs)
	newFTTokenIDs := make([]string, 0, numFTs)

	// Create RAC type for fractional tokens
	for i := 0; i < numFTs; i++ {
		// Use the TokenID of the whole token as the Parent
		parentTokenID := wholeTokens[i%len(wholeTokens)].TokenID // Use modulo to cycle through available whole tokens

		racType := &rac.RacType{
			Type:        c.RACFTType(),
			DID:         did,
			TokenNumber: uint64(i),
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
			FTInfo: &rac.RacFTInfo{
				Parents: parentTokenID, // Use the fetched token ID
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

		// Expect one block to be created
		if len(racBlocks) != 1 {
			return fmt.Errorf("failed to create RAC block")
		}

		// Update the signature of the RAC block
		err = racBlocks[0].UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update DID signature", "err", err)
			return err
		}

		// Add the RAC block to the wallet
		racBlockData := racBlocks[0].GetBlock()
		fr := bytes.NewBuffer(racBlockData)
		ftID, err := c.w.Add(fr, did, wallet.AddFunc)
		if err != nil {
			c.log.Error("Failed to create FT, Failed to add RAC token to IPFS", "err", err)
			return err
		}

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
						Token:    ftID,
						ParentID: parentTokenID,
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
			FTName:        FTName,
			TokenID:       ftID,
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
	}

	// Create the token in the wallet
	for i := range newFTs {
		ft := &newFTs[i]
		err = c.w.CreateFT(ft)
		if err != nil {
			c.log.Error("Failed to create fractional token", "err", err)
			return err
		}
	}
	return nil
}
