package core

import (
	"encoding/json"
	"fmt"
	"strings"

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

func (c *Core) GetTokenchain(TokenID string, TokenType string) *model.GetTokenChainResponce {
	getTokenChainReply := &model.GetTokenChainResponce{
		BasicResponse: model.BasicResponse{
			Status: false,
			Result: nil,
		},
		TokenChainData: nil,
	}

	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	var tokenTypeString string

	switch strings.ToUpper(TokenType) {
	case "RBT":
		tokenTypeString = RBTString
	case "PART":
		tokenTypeString = PartString
	case "FT":
		tokenTypeString = FTString
	case "NFT":
		tokenTypeString = NFTString
	case "SC":
		tokenTypeString = SmartContractString
	default:
		getTokenChainReply.Message = fmt.Sprintf("Invalid token type: %s", TokenType)
		c.log.Error(getTokenChainReply.Message)
		return getTokenChainReply
	}

	// Fetch token chain blocks iteratively
	for {
		blks, nextID, err := c.w.GetAllFullNodeTokenBlocks(TokenID, c.TokenType(tokenTypeString), blockID)
		if err != nil {
			getTokenChainReply.Message = fmt.Sprintf("Failed to get %s token chain blocks", TokenType)
			c.log.Error(getTokenChainReply.Message, "err", err)
			return getTokenChainReply
		}

		for _, blk := range blks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				c.log.Error(fmt.Sprintf("Invalid %s block", TokenType))
			}
		}

		blockID = nextID
		if nextID == "" {
			break
		}
	}

	str, err := tcMarshal("", blocks)
	if err != nil {
		c.log.Error(fmt.Sprintf("Failed to marshal %s token chain", TokenType), "err", err)
		return nil
	}

	byteArray := []byte(str)
	var data []interface{}

	err = json.Unmarshal(byteArray, &data)
	if err != nil {
		c.log.Error(fmt.Sprintf("Error unmarshalling JSON for %s tokenchain", TokenType), "err", err)
		return nil
	}

	for i, item := range data {
		flattenedItem := flattenKeys("", item)
		mappedItem := applyKeyMapping(flattenedItem)
		data[i] = mappedItem
	}

	getTokenChainReply.Status = true
	getTokenChainReply.Message = fmt.Sprintf("%s tokenchain data fetched successfully", TokenType)
	getTokenChainReply.TokenChainData = data

	if len(getTokenChainReply.TokenChainData) == 0 {
		getTokenChainReply.Message = fmt.Sprintf("No %s tokenchain data available", TokenType)
	}

	return getTokenChainReply
}

func (c *Core) GetTokenchainLatest(TokenID string, TokenType string) *model.GetTokenChainResponce {
	getTokenChainReply := &model.GetTokenChainResponce{
		BasicResponse: model.BasicResponse{
			Status: false,
			Result: nil,
		},
		TokenChainData: nil,
	}

	var tokenTypeString string

	switch strings.ToUpper(TokenType) {
	case "RBT":
		tokenTypeString = RBTString
	case "PART":
		tokenTypeString = PartString
	case "FT":
		tokenTypeString = FTString
	case "NFT":
		tokenTypeString = NFTString
	case "SC":
		tokenTypeString = SmartContractString
	default:
		getTokenChainReply.Message = fmt.Sprintf("Invalid token type: %s", TokenType)
		c.log.Error(getTokenChainReply.Message)
		return getTokenChainReply
	}

	// Fetch only the latest 10 blocks
	blks, err := c.w.GetLatestFullNodeTokenBlocks(c.TokenType(tokenTypeString), TokenID, 10)
	if err != nil {
		getTokenChainReply.Message = fmt.Sprintf("Failed to get %s token chain blocks", TokenType)
		c.log.Error(getTokenChainReply.Message, "err", err)
		return getTokenChainReply
	}

	blocks := make([]map[string]interface{}, 0)
	for _, blk := range blks {
		b := block.InitBlock(blk, nil)
		if b != nil {
			blocks = append(blocks, b.GetBlockMap())
		} else {
			c.log.Error(fmt.Sprintf("Invalid %s block", TokenType))
		}
	}

	str, err := tcMarshal("", blocks)
	if err != nil {
		c.log.Error(fmt.Sprintf("Failed to marshal %s token chain", TokenType), "err", err)
		return nil
	}

	byteArray := []byte(str)
	var data []interface{}

	err = json.Unmarshal(byteArray, &data)
	if err != nil {
		c.log.Error(fmt.Sprintf("Error unmarshalling JSON for %s tokenchain", TokenType), "err", err)
		return nil
	}

	for i, item := range data {
		flattenedItem := flattenKeys("", item)
		mappedItem := applyKeyMapping(flattenedItem)
		data[i] = mappedItem
	}

	getTokenChainReply.Status = true
	getTokenChainReply.Message = fmt.Sprintf("%s latest tokenchain data fetched successfully", TokenType)
	getTokenChainReply.TokenChainData = data

	if len(getTokenChainReply.TokenChainData) == 0 {
		getTokenChainReply.Message = fmt.Sprintf("No %s tokenchain data available", TokenType)
	}

	return getTokenChainReply
}

func (c *Core) GettxnAmountFromFullNode(txnID string) (*model.FullNodeTxnHistoryInfo, error) {
	RBTs, err := c.w.GetTxnAmountFromFullNode(txnID)
	if err != nil {
		return nil, err
	}
	return RBTs, nil
}

func (c *Core) GetFullTokenChainHeight(tokenID string, tokenType string) (int, error) {
	BlockHeight, err := c.w.GetBlockHeightFromFullNode(tokenID, tokenType)
	if err != nil {
		return 0, err
	}
	return BlockHeight, err
}
