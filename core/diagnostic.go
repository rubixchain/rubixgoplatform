package core

import (
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

func (c *Core) GetFinalQuorumList(ql []string) ([]string, error) {
	// Initialize finalQl as an empty slice to store the groups that meet the condition
	var finalQl []string
	var opError error
	// Loop through ql in groups of the Minimum Quorum Required
	for i := 0; i < len(ql); i += MinQuorumRequired {
		end := i + MinQuorumRequired
		if end > len(ql) {
			end = len(ql)
		}
		group := ql[i:end]

		// Initialize a variable to keep track of whether all items in the group meet the condition
		allQuorumSetup := true

		// Loop through the items in the group and check if their response message is "quorum is setup"
		for _, item := range group {
			opError = nil
			parts := strings.Split(item, ".")
			if len(parts) != 2 {
				continue
			}
			peerID := parts[0]
			did := parts[1]
			msg, _, err := c.CheckQuorumStatus(peerID, did)
			if err != nil || strings.Contains(msg, "Quorum Connection Error") {
				c.log.Error("Failed to check quorum status:", err)
				opError = fmt.Errorf("failed to check quorum status:  %v", err)
				allQuorumSetup = false
				break
			}
			if msg != "Quorum is setup" {
				// If any item in the group does not have the response message as "quorum is setup",
				// set allQuorumSetup to false and break the loop
				allQuorumSetup = false
				break
			}
			if strings.Contains(msg, "Quorum is not setup") {
				c.log.Error("quorums are currently unavailable for this trnx")
				opError = fmt.Errorf("quorums are uncurrently available for this trnx")
				allQuorumSetup = false
				break
			}
		}

		// If all items in the group have the response message as "quorum is setup",
		// append the group to finalQl
		if allQuorumSetup {
			finalQl = append(finalQl, group...)
			break
		}
	}
	// Return finalQl
	return finalQl, opError
}
