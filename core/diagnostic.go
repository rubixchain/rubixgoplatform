package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
)

func (c *Core) DumpTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}

	// if the boolian value - FullnodeToken - is true, then extract the chain from the fullnode db
	if dr.FullnodeToken {
		return c.DumpFullnodeTokenChain(dr)
	}

	t, err := c.w.ReadToken(dr.Token)
	if err != nil {
		ds.Message = "Failed to get token chain block, token does not exist"
		return ds
	}
	ts := RBTString
	if t.TokenValue < 1.0 {
		ts = PartString
	}
	blks := make([][]byte, 0)
	var nextID string

	blks, nextID, err = c.w.GetAllTokenBlocks(dr.Token, c.TokenType(ts), dr.BlockID)
	if err != nil {
		ds.Message = "Failed to get token chain block"
		return ds
	}
	ds.Status = true
	ds.Message = "Successfully got the token chain block"
	ds.Blocks = blks
	ds.NextBlockID = nextID
	return ds
}

func (c *Core) DumpFullnodeTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	// if it is not a fullnode, then return error msg
	if !c.fullNode {
		ds.Message = "not a fullnode, please remove the boolian param 'fullnodetoken'"
		c.log.Error(ds.Message)
		return ds
	}

	blks := make([][]byte, 0)
	var nextID string
	var err error

	switch dr.AssetType {
	case "rbt", "RBT":
		t, err := c.w.ReadSyncedRBTFromTable(dr.Token)
		if err != nil {
			ds.Message = "Failed to get token chain block, token does not exist"
			return ds
		}
		ts := RBTString
		if t.TokenValue < 1.0 {
			ts = PartString
		}

		blks, nextID, err = c.w.GetAllFullNodeTokenBlocks(dr.Token, c.TokenType(ts), dr.BlockID)
		if err != nil {
			ds.Message = "Failed to get token chain block"
			return ds
		}
	case "ft", "FT":
		blks, nextID, err = c.w.GetAllFullNodeTokenBlocks(dr.Token, c.TokenType(FTString), dr.BlockID)
		if err != nil {
			ds.Message = "Failed to get token chain block"
			return ds
		}
	case "nft", "NFT":
		blks, nextID, err = c.w.GetAllFullNodeTokenBlocks(dr.Token, c.TokenType(NFTString), dr.BlockID)
		if err != nil {
			ds.Message = "Failed to get token chain block"
			return ds
		}
	case "sc", "SC", "smartcontract", "SmartContract":
		blks, nextID, err = c.w.GetAllFullNodeTokenBlocks(dr.Token, c.TokenType(SmartContractString), dr.BlockID)
		if err != nil {
			ds.Message = "Failed to get token chain block"
			return ds
		}
	default:
		errMsg := fmt.Sprintf("invalid asset type, please choose among : %s, %s, %s, and %s", RBTString, FTString, NFTString, SmartContractString)
		c.log.Error(errMsg)
		ds.Message = errMsg
		return ds
	}

	ds.Status = true
	ds.Message = "Successfully got the token chain block"
	ds.Blocks = blks
	ds.NextBlockID = nextID
	return ds
}

func (c *Core) DumpFTTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}

	ts := FTString

	blks, nextID, err := c.w.GetAllTokenBlocks(dr.Token, c.TokenType(ts), dr.BlockID)
	if err != nil {
		ds.Message = "Failed to get token chain block"
		return ds
	}
	ds.Status = true
	ds.Message = "Successfully got the token chain block"
	ds.Blocks = blks
	ds.NextBlockID = nextID
	return ds
}

func (c *Core) GetFTTokenchain(FTTokenID string) *model.GetTokenChainResponce {
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
		blks, nextID, err := c.w.GetAllTokenBlocks(FTTokenID, c.TokenType(tokenTypeString), blockID)
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

func (c *Core) DumpSmartContractTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	_, err := c.w.GetSmartContractToken(dr.Token)
	if err != nil {
		ds.Message = "Failed to get smart contract token chain block, token does not exist"
		return ds
	}
	tokenTypeString := SmartContractString
	blks, nextID, err := c.w.GetAllTokenBlocks(dr.Token, c.TokenType(tokenTypeString), dr.BlockID)
	if err != nil {
		ds.Message = "Failed to get smart contract token chain block"
		return ds
	}
	ds.Status = true
	ds.Message = "Successfully got the smart contract token chain block"
	ds.Blocks = blks
	ds.NextBlockID = nextID
	return ds
}

func (c *Core) DumpNFTTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	_, err := c.w.GetNFTToken(dr.Token)
	if err != nil {
		ds.Message = "Failed to get nft, token does not exist"
		return ds
	}
	tokenTypeString := NFTString
	blks, nextID, err := c.w.GetAllTokenBlocks(dr.Token, c.TokenType(tokenTypeString), dr.BlockID)
	if err != nil {
		ds.Message = "Failed to get nft token chain block"
		return ds
	}
	ds.Status = true
	ds.Message = "Successfully got nft token chain block"
	ds.Blocks = blks
	ds.NextBlockID = nextID
	return ds
}

func (c *Core) GetSmartContractTokenChainData(getReq *model.SmartContractTokenChainDataReq) *model.SmartContractDataReply {
	reply := &model.SmartContractDataReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}

	_, err := c.w.GetSmartContractToken(getReq.Token)
	if err != nil {
		reply.Message = "Failed to get smart contract token data, token does not exist"
		return reply
	}

	sctDataArray := make([]model.SCTDataReply, 0)
	c.log.Debug("latest flag ", getReq.Latest)
	if getReq.Latest {
		latestBlock := c.w.GetLatestTokenBlock(getReq.Token, c.TokenType(SmartContractString))
		if latestBlock == nil {
			reply.Message = "Failed to get smart contract token data, block is empty"
			return reply
		}

		blockNo, err := latestBlock.GetBlockNumber(getReq.Token)
		if err != nil {
			reply.Message = "Failed to get smart contract token latest block number"
			return reply
		}

		blockId, err := latestBlock.GetBlockID(getReq.Token)
		if err != nil {
			reply.Message = "Failed to get smart contract token latest block number"
			return reply
		}

		epoch := latestBlock.GetEpoch()
		scData := latestBlock.GetSmartContractData()

		var initiatorSignature string
		var initiatorSignData string

		signObj := latestBlock.GetInitiatorSignature()
		if signObj == nil {
			reply.Message = "unable to fetch intiateor signature"
			return reply
		} else {
			initiatorSignature = signObj.PrivateSign
			initiatorSignData = signObj.Hash
		}

		executorDID := latestBlock.GetExecutorDID()
		deployerDID := latestBlock.GetDeployerDID()

		sctData := model.SCTDataReply{
			BlockNo:            blockNo,
			BlockId:            blockId,
			SmartContractData:  scData,
			Epoch:              epoch,
			InitiatorSignature: initiatorSignature,
			ExecutorDID:        executorDID,
			DeployerDID:        deployerDID,
			InitiatorSignData:  initiatorSignData,
		}

		sctDataArray = append(sctDataArray, sctData)
		reply.SCTDataReply = sctDataArray
		reply.Status = true
		reply.Message = "Fetched latest block smart contract data"
		return reply
	}

	blks, _, err := c.w.GetAllTokenBlocks(getReq.Token, c.TokenType(SmartContractString), "")
	if err != nil {
		reply.Message = "unable to fetch token blocks for contract: " + getReq.Token
		return reply
	}

	var deployerDIDFromFirstBlock string

	for _, blk := range blks {
		block := block.InitBlock(blk, nil)
		if block == nil {
			reply.Message = "Failed to initialize smart contract block"
			return reply
		}
		blockNo, err := block.GetBlockNumber(getReq.Token)
		if err != nil {
			reply.Message = "Failed to get smart contract token latest block number"
			return reply
		}
		blockId, err := block.GetBlockID(getReq.Token)
		if err != nil {
			reply.Message = "Failed to get smart contract token latest block number"
			return reply
		}
		scData := block.GetSmartContractData()

		epoch := block.GetEpoch()

		var executorSignature string
		var executorSignData string
		signObj := block.GetInitiatorSignature()
		if signObj == nil {
			reply.Message = "unable to fetch intiateor signature"
			return reply
		} else {
			executorSignature = signObj.PrivateSign
			executorSignData = signObj.Hash
		}

		executorDID := block.GetExecutorDID()
		deployerDID := block.GetDeployerDID()

		// Save deployer DID from block 0
		if blockNo == 0 && deployerDID != "" {
			deployerDIDFromFirstBlock = deployerDID
		}

		// If current block doesn't have deployer DID, use the one from block 0
		if deployerDID == "" && deployerDIDFromFirstBlock != "" {
			deployerDID = deployerDIDFromFirstBlock
		}

		fmt.Println("The deployer did is :", deployerDID)
		sctData := model.SCTDataReply{
			BlockNo:            blockNo,
			BlockId:            blockId,
			SmartContractData:  scData,
			Epoch:              epoch,
			InitiatorSignature: executorSignature,
			ExecutorDID:        executorDID,
			DeployerDID:        deployerDID,
			InitiatorSignData:  executorSignData,
		}

		sctDataArray = append(sctDataArray, sctData)
	}
	reply.SCTDataReply = sctDataArray
	reply.Status = true
	reply.Message = "Fetched Smart contract data"
	return reply
}

func (c *Core) GetNFTTokenChainData(getReq *model.SmartContractTokenChainDataReq) *model.NFTDataReply {
	reply := &model.NFTDataReply{
		BasicResponse: model.BasicResponse{Status: false},
	}

	// Check if token exists
	_, err := c.w.GetNFTToken(getReq.Token)
	if err != nil {
		reply.Message = "Failed to get NFT data, token does not exist"
		return reply
	}

	var nftDataArray []model.NFTData

	if getReq.Latest {
		latestBlock := c.w.GetLatestTokenBlock(getReq.Token, c.TokenType(NFTString))
		if latestBlock == nil {
			reply.Message = "Failed to get NFT data, block is empty"
			return reply
		}

		blockNo, err1 := latestBlock.GetBlockNumber(getReq.Token)
		blockId, err2 := latestBlock.GetBlockID(getReq.Token)
		if err1 != nil || err2 != nil {
			reply.Message = "Failed to get latest block details"
			return reply
		}

		nftDataArray = append(nftDataArray, model.NFTData{
			BlockNo:       blockNo,
			BlockId:       blockId,
			NFTData:       latestBlock.GetNFTData(),
			NFTOwner:      latestBlock.GetOwner(),
			NFTValue:      latestBlock.GetTokenValue(),
			Epoch:         latestBlock.GetEpoch(),
			TransactionID: latestBlock.GetTid(),
		})
	} else {
		blks, _, err := c.w.GetAllTokenBlocks(getReq.Token, c.TokenType(NFTString), "")
		if err != nil {
			reply.Message = "Failed to get NFT token blocks"
			return reply
		}

		for _, blk := range blks {
			block := block.InitBlock(blk, nil)
			if block == nil {
				reply.Message = "Failed to initialize NFT block"
				return reply
			}

			blockNo, err1 := block.GetBlockNumber(getReq.Token)
			blockId, err2 := block.GetBlockID(getReq.Token)
			if err1 != nil || err2 != nil {
				reply.Message = "Failed to get block details"
				return reply
			}

			nftDataArray = append(nftDataArray, model.NFTData{
				BlockNo:       blockNo,
				BlockId:       blockId,
				NFTData:       block.GetNFTData(),
				NFTOwner:      block.GetOwner(),
				NFTValue:      block.GetTokenValue(),
				Epoch:         block.GetEpoch(), // Fixed the missing Epoch value
				TransactionID: block.GetTid(),
			})
		}
	}

	// Set final response
	reply.NFTDataReply = nftDataArray
	reply.Status = true
	reply.Message = "Fetched NFT data"
	return reply
}

func (c *Core) RegisterCallBackURL(registerReq *model.RegisterCallBackUrlReq) *model.BasicResponse {
	reply := &model.BasicResponse{
		Status: false,
	}
	input := &wallet.CallBackUrl{
		SmartContractHash: registerReq.SmartContractToken,
		CallBackUrl:       registerReq.CallBackURL,
		CreatedAt:         time.Now(),
	}
	err := c.w.WriteCallBackUrlToDB(input)
	if err != nil {
		reply.Message = "Failed to register call back url to DB"
		return reply
	}
	c.log.Debug("Call back URL registered successfully")
	reply.Status = true
	reply.Message = "Call back URL registered successfully"
	return reply
}

func (c *Core) RemoveTokenChainBlock(removeReq *model.TCRemoveRequest) *model.TCRemoveReply {
	removeReply := &model.TCRemoveReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	tt := token.RBTTokenType
	if c.testNet {
		tt = token.TestTokenType
	}
	err := c.w.RemoveTokenChainBlocklatest(removeReq.Token, tt)
	if err != nil {
		tt = token.PartTokenType
		if c.testNet {
			tt = token.TestPartTokenType
		}
		err = c.w.RemoveTokenChainBlocklatest(removeReq.Token, tt)
		if err != nil {
			removeReply.Message = "Failed to remove parts token chain block"
			return removeReply
		} else {
			removeReply.Message = "Failed to remove whole token chain block"
			return removeReply
		}

	}
	removeReply.Status = true
	removeReply.Message = "Successfully removed token chain block " + removeReq.Token
	return removeReply
}

func (c *Core) ReleaseAllLockedTokens() model.BasicResponse {
	response := &model.BasicResponse{
		Status: false,
	}
	err := c.w.ReleaseAllLockedTokens()
	if err != nil {
		c.log.Error("failed to release Locked tokens, ", err)
		response.Message = "failed to release Locked tokens, " + err.Error()
		return *response
	}
	response.Status = true
	response.Message = "All Locked Tokens Releases Successfully Or NO Locked Tokens to release"
	return *response
}

func (c *Core) GetActiveQuorums(ql []string) []string {
	var activeQuorumList []string

	// Loop through ql in groups of the Minimum Quorum Required
	for i := 0; i < len(ql); i += QuorumRequired {
		end := i + QuorumRequired
		if end > len(ql) {
			end = len(ql)
		}
		group := ql[i:end]

		// Loop through the items in the group and check if their response message is "quorum is setup"
		for _, item := range group {
			if len(activeQuorumList) == QuorumRequired {
				return activeQuorumList
			}

			parts := strings.Split(item, ".")
			if len(parts) != 2 {
				continue
			}

			peerID := parts[0]
			did := parts[1]

			msg, _, err := c.CheckQuorumStatus(peerID, did)
			if err != nil || strings.Contains(msg, "Quorum Connection Error") {
				c.log.Error(fmt.Sprintf("Failed to check quorum status for quorum %v, error: %v", item, err))
				continue
			}

			if strings.Contains(msg, "Quorum is not setup") {
				errMsg := fmt.Sprintf("Quorum %v is not setup", item)
				c.log.Error(errMsg)
				continue
			}

			activeQuorumList = append(activeQuorumList, item)
		}
	}

	return activeQuorumList
}
