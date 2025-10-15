package core

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) GetAllRBTs() ([]wallet.SyncedRBT, error) {
	RBTs, err := c.w.GetAllRBTs()
	if err != nil {
		return nil, err
	}
	return RBTs, nil
}

func (c *Core) GetAllFTs() ([]wallet.SyncedFT, error) {
	FTs, err := c.w.GetAllFTs()
	if err != nil {
		return nil, err
	}
	return FTs, nil
}

func (c *Core) GetAllNFTs() ([]wallet.SyncedNFT, error) {
	NFTs, err := c.w.GetAllNFTs()
	if err != nil {
		return nil, err
	}
	return NFTs, nil
}

func (c *Core) GetAllSmartContracts() ([]wallet.SyncedSmartContract, error) {
	SCs, err := c.w.GetAllSmartContracts()
	if err != nil {
		return nil, err
	}
	return SCs, nil
}

func (c *Core) GetRBTsbyDID(DID string) ([]wallet.SyncedRBT, error) {
	RBTs, err := c.w.GetAllRBTbyDID(DID)
	if err != nil {
		return nil, err
	}
	return RBTs, nil
}

func (c *Core) GetFTsbyDID(DID string) ([]wallet.FTToken, error) {
	FTs, err := c.w.GetAllFTsbyDID(DID)
	if err != nil {
		return nil, err
	}
	return FTs, nil
}

// returning all the NFTs(syncedNFT) by DID
func (c *Core) GetNFTsbyDID(DID string) ([]wallet.SyncedNFT, error) {
	NFTs, err := c.w.GetAllNFTsbyDID(DID)
	if err != nil {
		return nil, err
	}
	return NFTs, nil
}

// returning all the SmartContracts(syncedSmartContract) by DID
func (c *Core) GetSmartContractsbyDID(DID string) ([]wallet.SyncedSmartContract, error) {
	SCs, err := c.w.GetAllSmartContractsbyDID(DID)
	if err != nil {
		return nil, err
	}
	return SCs, nil
}

func (c *Core) GetRBTFullTokenchain(TokenID string) *model.GetTokenChainResponce {
	getRBTChainReply := &model.GetTokenChainResponce{
		BasicResponse: model.BasicResponse{
			Status: false,
			Result: nil,
		},
		TokenChainData: nil,
	}

	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	tokenTypeString := RBTString

	// Initialize blockID for fetching token blocks
	for {
		blks, nextID, err := c.w.GetAllFullNodeTokenBlocks(TokenID, c.TokenType(tokenTypeString), blockID)
		if err != nil {
			getRBTChainReply.Message = "Failed to get RBT token chain blocks"
			c.log.Error(getRBTChainReply.Message, "err", err)
			return getRBTChainReply
		}

		// Process each block received
		for _, blk := range blks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				c.log.Error("Invalid RBT block")
			}
		}

		// Update blockID for the next iteration
		blockID = nextID
		if nextID == "" {
			break // Exit loop if there are no more blocks to fetch
		}
	}

	str, err := tcMarshal("", blocks)
	if err != nil {
		c.log.Error("Failed to marshal RBT token chain", "err", err)
		return nil
	}

	byteArray := []byte(str)
	var data []interface{}

	err = json.Unmarshal(byteArray, &data)
	if err != nil {
		fmt.Println("Error unmarshal JSON for RBT tokenchain :", err)
		return nil
	}

	for i, item := range data {
		flattenedItem := flattenKeys("", item)
		mappedItem := applyKeyMapping(flattenedItem)
		data[i] = mappedItem
	}

	getRBTChainReply.Status = true
	getRBTChainReply.Message = "RBT tokenchain data fetched successfully"
	getRBTChainReply.TokenChainData = data

	if len(getRBTChainReply.TokenChainData) == 0 {
		getRBTChainReply.Status = true
		getRBTChainReply.Message = "No RBT tokenchain data available"
		return getRBTChainReply
	}

	return getRBTChainReply
}

func (c *Core) GetFTFullTokenchain(FTTokenID string) *model.GetTokenChainResponce {
	getFTReply := &model.GetTokenChainResponce{
		BasicResponse: model.BasicResponse{
			Status: false,
			Result: nil,
		},
		TokenChainData: nil,
	}

	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	tokenTypeString := FTString

	// Initialize blockID for fetching token blocks
	for {
		blks, nextID, err := c.w.GetAllFullNodeTokenBlocks(FTTokenID, c.TokenType(tokenTypeString), blockID)
		if err != nil {
			getFTReply.Message = "Failed to get FT token chain blocks"
			c.log.Error(getFTReply.Message, "err", err)
			return getFTReply
		}

		// Process each block received
		for _, blk := range blks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				c.log.Error("Invalid FT block")
			}
		}

		// Update blockID for the next iteration
		blockID = nextID
		if nextID == "" {
			break // Exit loop if there are no more blocks to fetch
		}
	}

	str, err := tcMarshal("", blocks)
	if err != nil {
		c.log.Error("Failed to marshal FT token chain", "err", err)
		return nil
	}

	byteArray := []byte(str)
	var data []interface{}

	err = json.Unmarshal(byteArray, &data)
	if err != nil {
		fmt.Println("Error unmarshal JSON for FT tokenchain :", err)
		return nil
	}

	for i, item := range data {
		flattenedItem := flattenKeys("", item)
		mappedItem := applyKeyMapping(flattenedItem)
		data[i] = mappedItem
	}

	getFTReply.Status = true
	getFTReply.Message = "FT tokenchain data fetched successfully"
	getFTReply.TokenChainData = data

	if len(getFTReply.TokenChainData) == 0 {
		getFTReply.Status = true
		getFTReply.Message = "No FT tokenchain data available"
		return getFTReply
	}

	return getFTReply
}

func (c *Core) GetRBTTokenGenesisBlock(tokenID string) *model.FullNodeGenesisBlock {
	var fnGB model.FullNodeGenesisBlock
	tokenType := RBTString

	genesisBlock := c.w.GetFullNodeGenesisTokenBlock(tokenID, c.TokenType(tokenType))
	if genesisBlock == nil {
		c.log.Error("genesis block not found", "tokenID", tokenID, "type", tokenType)
		return nil
	}

	tokenLevel, tokenNumber := genesisBlock.GetTokenLevel(tokenID)
	parentID, grandParents, err := genesisBlock.GetParentDetials(tokenID)
	if err != nil {
		c.log.Error("Failed to get parentIDs or grandParentIDs for explorer service", "tokenID", tokenID, "err", err)
	}

	fnGB.Token = tokenID
	fnGB.TokenType = tokenType
	fnGB.TokenLevel = tokenLevel
	fnGB.TokenNumber = tokenNumber
	fnGB.ParentID = parentID
	fnGB.GrandParentID = append(fnGB.GrandParentID, grandParents...)

	return &fnGB
}

func (c *Core) GetFTTokenGenesisBlock(tokenID string) *model.FullNodeGenesisBlock {
	var fnGB model.FullNodeGenesisBlock
	tokenType := FTString

	genesisBlock := c.w.GetFullNodeGenesisTokenBlock(tokenID, c.TokenType(tokenType))
	if genesisBlock == nil {
		c.log.Error("genesis block not found", "tokenID", tokenID, "type", tokenType)
		return nil
	}

	_, tokenNumber := genesisBlock.GetTokenLevel(tokenID)
	parentID, _, err := genesisBlock.GetParentDetials(tokenID)
	if err != nil {
		c.log.Error("Failed to get parentIDs for explorer service", "tokenID", tokenID, "err", err)
	}

	fnGB.Token = tokenID
	fnGB.TokenType = tokenType
	fnGB.TokenNumber = tokenNumber
	fnGB.ParentID = parentID

	return &fnGB
}

func (c *Core) GetRBTLatestBlock(tokenID string) *model.FullNodeTokenChainBlock {
	var fullNodeTokenBlock model.FullNodeTokenChainBlock
	var fullNodeTransBlock model.TransInfo

	tokenType := RBTString
	latestBlock := c.w.GetFullNodeLatestTokenBlock(tokenID, c.TokenType(tokenType))

	transType := latestBlock.GetTransType()
	tokenOwner := latestBlock.GetOwner()
	transBlockBytes := latestBlock.GetBlock()

	// --- Convert TransTokens (from []string)
	blockTransTokens := latestBlock.GetTransTokens() // []string
	var transTokens []model.TransTokens
	for _, t := range blockTransTokens {
		transTokens = append(transTokens, model.TransTokens{
			Token:       t,
			TokenType:   0, // default
			UnplededID:  "",
			CommitedDID: "",
		})
	}

	// --- Convert PledgeDetails
	blockPledgeDetails := latestBlock.GetPledgedTokens()
	var pledgeDetails []model.PledgeDetail
	for _, pd := range blockPledgeDetails {
		pledgeDetails = append(pledgeDetails, model.PledgeDetail{
			Token:        pd.Token,
			TokenType:    pd.TokenType,
			DID:          pd.DID,
			TokenBlockID: pd.TokenBlockID,
		})
	}

	// --- Convert QuorumSignature
	blockQuorumDetails, err := latestBlock.GetQuorumSignatureList()
	if err != nil {
		c.log.Error("Failed to get quorum signature list for RBT latest block for explorer")
	}
	var quorumDetails []model.CreditSignature
	for _, qs := range blockQuorumDetails {
		quorumDetails = append(quorumDetails, model.CreditSignature{
			Signature:     qs.Signature,
			PrivSignature: qs.PrivSignature,
			DID:           qs.DID,
			Hash:          qs.Hash,
			SignType:      qs.SignType,
		})
	}

	// --- Convert InitiatorSignature
	blockInitiatorDetails := latestBlock.GetInitiatorSignature()
	var initiatorDetails *model.InitiatorSignature
	if blockInitiatorDetails != nil {
		initiatorDetails = &model.InitiatorSignature{
			NLSSShare:   blockInitiatorDetails.NLSSShare,
			PrivateSign: blockInitiatorDetails.PrivateSign,
			DID:         blockInitiatorDetails.DID,
			Hash:        blockInitiatorDetails.Hash,
			SignType:    blockInitiatorDetails.SignType,
		}
	}

	tokenValue := latestBlock.GetTokenValue()
	childTokens := latestBlock.GetChildTokens()

	// Decode transaction block
	transBlock := block.InitBlock(transBlockBytes, nil)
	sender := transBlock.GetSenderDID()
	receiver := transBlock.GetReceiverDID()
	trxnComment := transBlock.GetComment()
	trxnID := transBlock.GetTid()

	// Fill TransInfo
	fullNodeTransBlock.SenderDID = sender
	fullNodeTransBlock.ReceiverDID = receiver
	fullNodeTransBlock.Comment = trxnComment
	fullNodeTransBlock.TID = trxnID
	fullNodeTransBlock.Block = transBlockBytes
	fullNodeTransBlock.Tokens = transTokens

	// Fill FullNodeTokenChainBlock
	fullNodeTokenBlock.TransactionType = transType
	fullNodeTokenBlock.TokenOwner = tokenOwner
	fullNodeTokenBlock.TransInfo = &fullNodeTransBlock
	fullNodeTokenBlock.PledgeDetails = pledgeDetails
	fullNodeTokenBlock.QuorumSignature = quorumDetails
	fullNodeTokenBlock.TokenValue = tokenValue
	fullNodeTokenBlock.ChildTokens = childTokens
	fullNodeTokenBlock.InitiatorSignature = initiatorDetails

	return &fullNodeTokenBlock
}

func (c *Core) GetFTLatestBlock(tokenID string) *model.FullNodeTokenChainBlock {
	var fullNodeTokenBlock model.FullNodeTokenChainBlock
	var fullNodeTransBlock model.TransInfo

	tokenType := FTString
	latestBlock := c.w.GetFullNodeLatestTokenBlock(tokenID, c.TokenType(tokenType))

	transType := latestBlock.GetTransType()
	tokenOwner := latestBlock.GetOwner()
	transBlockBytes := latestBlock.GetBlock()

	// --- Convert TransTokens (from []string)
	blockTransTokens := latestBlock.GetTransTokens() // []string
	var transTokens []model.TransTokens
	for _, t := range blockTransTokens {
		transTokens = append(transTokens, model.TransTokens{
			Token:       t,
			TokenType:   0, // default, adjust if needed
			UnplededID:  "",
			CommitedDID: "",
		})
	}

	// --- Convert PledgeDetails
	blockPledgeDetails := latestBlock.GetPledgedTokens()
	var pledgeDetails []model.PledgeDetail
	for _, pd := range blockPledgeDetails {
		pledgeDetails = append(pledgeDetails, model.PledgeDetail{
			Token:        pd.Token,
			TokenType:    pd.TokenType,
			DID:          pd.DID,
			TokenBlockID: pd.TokenBlockID,
		})
	}

	// --- Convert QuorumSignature
	blockQuorumDetails, err := latestBlock.GetQuorumSignatureList()
	if err != nil {
		c.log.Error("Failed to get quorum signature list for FT latest block for explorer")
	}
	var quorumDetails []model.CreditSignature
	for _, qs := range blockQuorumDetails {
		quorumDetails = append(quorumDetails, model.CreditSignature{
			Signature:     qs.Signature,
			PrivSignature: qs.PrivSignature,
			DID:           qs.DID,
			Hash:          qs.Hash,
			SignType:      qs.SignType,
		})
	}

	// --- Convert InitiatorSignature
	blockInitiatorDetails := latestBlock.GetInitiatorSignature()
	var initiatorDetails *model.InitiatorSignature
	if blockInitiatorDetails != nil {
		initiatorDetails = &model.InitiatorSignature{
			NLSSShare:   blockInitiatorDetails.NLSSShare,
			PrivateSign: blockInitiatorDetails.PrivateSign,
			DID:         blockInitiatorDetails.DID,
			Hash:        blockInitiatorDetails.Hash,
			SignType:    blockInitiatorDetails.SignType,
		}
	}

	tokenValue := latestBlock.GetTokenValue()
	childTokens := latestBlock.GetChildTokens()

	// Decode transaction block
	transBlock := block.InitBlock(transBlockBytes, nil)
	sender := transBlock.GetSenderDID()
	receiver := transBlock.GetReceiverDID()
	trxnComment := transBlock.GetComment()
	trxnID := transBlock.GetTid()

	// Fill TransInfo
	fullNodeTransBlock.SenderDID = sender
	fullNodeTransBlock.ReceiverDID = receiver
	fullNodeTransBlock.Comment = trxnComment
	fullNodeTransBlock.TID = trxnID
	fullNodeTransBlock.Block = transBlockBytes
	fullNodeTransBlock.Tokens = transTokens

	// Fill FullNodeTokenChainBlock
	fullNodeTokenBlock.TransactionType = transType
	fullNodeTokenBlock.TokenOwner = tokenOwner
	fullNodeTokenBlock.TransInfo = &fullNodeTransBlock
	fullNodeTokenBlock.PledgeDetails = pledgeDetails
	fullNodeTokenBlock.QuorumSignature = quorumDetails
	fullNodeTokenBlock.TokenValue = tokenValue
	fullNodeTokenBlock.ChildTokens = childTokens
	fullNodeTokenBlock.InitiatorSignature = initiatorDetails

	return &fullNodeTokenBlock
}
