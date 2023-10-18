package core

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) DumpTokenChain(dr *model.TCDumpRequest) *model.TCDumpReply {
	ds := &model.TCDumpReply{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
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
		scData := latestBlock.GetSmartContractData()
		if scData == "" && blockNo == 0 {
			reply.Message = "Gensys Block, No Smart contract Data"
		}
		sctData := model.SCTDataReply{
			BlockNo:           blockNo,
			BlockId:           blockId,
			SmartContractData: scData,
		}
		sctDataArray = append(sctDataArray, sctData)
		reply.SCTDataReply = sctDataArray
		reply.Status = true
		reply.Message = "Fetched latest block smart contract data"
		return reply
	}

	blks, _, err := c.w.GetAllTokenBlocks(getReq.Token, c.TokenType(SmartContractString), "")

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
		if scData == "" && blockNo == 0 {
			reply.Message = "Gensys Block, No Smart contract Data"
		}
		sctData := model.SCTDataReply{
			BlockNo:           blockNo,
			BlockId:           blockId,
			SmartContractData: scData,
		}
		sctDataArray = append(sctDataArray, sctData)
	}
	reply.SCTDataReply = sctDataArray
	reply.Status = true
	reply.Message = "Fetched Smart contract data"
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
