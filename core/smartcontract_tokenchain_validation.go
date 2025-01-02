package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) SmartContractTokenChainValidation(userDID string, tokenId string, blockCount int) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}
	ok := c.w.IsDIDExist(userDID)
	if !ok {
		response.Message = "Invalid did, please pass did of the tokenchain validator"
		return response, fmt.Errorf("invalid did: %v, please pass did of the tokenchain validator", userDID)
	}

	if tokenId == "" { //if provided the boolean flag 'allmyToken', all the tokens' chain from tokens table will be validated
		c.log.Info("Validating all smart contracts from your smart contract table")
		tokensList, err := c.w.GetSmartContractTokenByDeployer(userDID)
		if err != nil {
			response.Message = "failed to fetch all smart contract tokens"
			return response, err
		}
		//Validate smart contract tokenchain for each smart contrtact in the smart contract table
		for _, tokenInfo := range tokensList {
			//Get token type
			typeString := SmartContractString
			tokenType := c.TokenType(typeString)

			response, err = c.ValidateSmartContractTokenChain(userDID, tokenInfo, tokenType, blockCount)
			if err != nil || !response.Status {
				c.log.Error("token chain validation failed for token:", tokenInfo.SmartContractHash, "Error :", err, "msg:", response.Message)
				return response, err
			}
		}

	} else {
		//Fetch token information
		tokenInfo, err := c.w.GetSmartContractToken(tokenId)
		if err != nil {
			response.Message = "Failed to get smart contract token, smart contract does not exist"
			return response, err
		}
		//Get token type
		typeString := SmartContractString
		tokenType := c.TokenType(typeString)
		//Validate tokenchain for the provided token
		response, err = c.ValidateSmartContractTokenChain(userDID, tokenInfo[0], tokenType, blockCount)
		if err != nil || !response.Status {
			c.log.Error("token chain validation failed for token:", tokenId, "Error :", err, "msg:", response.Message)
			return response, err
		}
	}
	return response, nil
}

// Validates tokenchain for the given token upto the specified block height
func (c *Core) ValidateSmartContractTokenChain(userDID string, tokenInfo wallet.SmartContract, tokenType int, blockCount int) (*model.BasicResponse, error) {
	c.log.Info("validating smart copntract tokenchain", tokenInfo.SmartContractHash)
	response := &model.BasicResponse{
		Status: false,
	}

	validatedBlockCount := 0
	blockId := ""

	var blocks [][]byte
	var prevBlockId string
	var nextBlockID string
	var err error

	//This for loop ensures that we fetch all the blocks in the token chain
	//starting from genesis block to latest block
	for {
		//GetAllTokenBlocks returns next 100 blocks and nextBlockID of the 100th block,
		//starting from the given block Id, in the direction: genesis to latest block
		blocks, nextBlockID, err = c.w.GetAllTokenBlocks(tokenInfo.SmartContractHash, tokenType, blockId)
		if err != nil {
			response.Message = "Failed to get token chain block"
			return response, err
		}
		//the nextBlockID of the latest block is empty string
		blockId = nextBlockID
		if nextBlockID == "" {
			break
		}
	}

	c.log.Info("token chain length", len(blocks))
	for i := len(blocks) - 1; i >= 0; i-- {
		b := block.InitBlock(blocks[i], nil)
		//validatedBlockCount keeps count of the number of blocks validated, including failed blocks
		validatedBlockCount++

		if b != nil {
			//calculate block height
			blockHeight, err := b.GetBlockNumber(tokenInfo.SmartContractHash)
			if err != nil {
				response.Message = "failed to fetch BlockNumber"
				return response, fmt.Errorf("invalid token chain block")
			}

			c.log.Info("validating at block height:", blockHeight)

			//fetch transaction type to validate the block accordingly
			txnType := b.GetTransType()
			switch txnType {
			case block.TokenDeployedType:
				prevBlockId := ""
				//validate smart contract deployed block
				response, err = c.ValidateSmartContractBlock(b, tokenInfo.SmartContractHash, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenExecutedType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(tokenInfo.SmartContractHash)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate smart contract executed block
				response, err = c.ValidateSmartContractBlock(b, tokenInfo.SmartContractHash, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			default:
				prevBlockId := ""
				//validate smart contract deployed block
				response, err = c.ValidateSmartContractBlock(b, tokenInfo.SmartContractHash, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			}

		} else {
			c.log.Error("Invalid block")
		}

		c.log.Info("validatedBlockCount", validatedBlockCount)
		// //If blockCount is provided, then we will stop validating when we reach the blockCount
		// //If blockCount is not provided,i.e., is 0, then it will never be equal to validatedBlockCount
		// //and thus will be continued till genesis block
		if validatedBlockCount == blockCount {
			break
		}
	}

	//Get latest block in the token chain
	latestBlock := c.w.GetLatestTokenBlock(tokenInfo.SmartContractHash, tokenType)

	//Verify if the token is pinned only by the current owner aka receiver in the latest block
	response, err = c.CurrentOwnerPinCheck(latestBlock, tokenInfo.SmartContractHash, userDID)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	//verify if the current token state is pinned by the quorums in the latest block
	response, err = c.CurrentQuorumStatePinCheck(latestBlock, tokenInfo.SmartContractHash, tokenType, userDID)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	c.log.Info("token chain validated successfully")
	response.Message = "token chain validated successfully"
	response.Status = true
	return response, nil
}

// validate block of type: TokenTransferredType = "02" / TokenDeployedType = "09" / TokenExecutedType = "10"
func (c *Core) ValidateSmartContractBlock(b *block.Block, tokenId string, calculatedPrevBlockId string, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenId, calculatedPrevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//Validate sender signature
	response, err = c.ValidateTxnInitiator(b)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//validate quorums signature
	response, err = c.ValidateQuorums(b, userDID)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	response.Status = true
	if b.GetTransType() == block.TokenDeployedType { //smart contract deployed mode
		response.Message = "smart contract deployed block validated successfully"
		c.log.Debug("successfully validated smart contract deployed block")
		// return response, nil
	} else { //smart contract executed mode
		response.Message = "smart contract executed block validated successfully"
		c.log.Debug("successfully validated smart contract executed block")
		// return response, nil
	}
	return response, nil
}
