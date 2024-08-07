package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) SmartContractTokenChainValidation(user_did string, tokenId string, blockCount int) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}
	ok := c.w.IsDIDExist(user_did)
	if !ok {
		response.Message = "Invalid did, please pass did of the tokenchain validator"
		return response, fmt.Errorf("invalid did: %v, please pass did of the tokenchain validator", user_did)
	}

	if tokenId == "" { //if provided the boolean flag 'allmyToken', all the tokens' chain from tokens table will be validated
		c.log.Info("Validating all smart contracts from your smart contract table")
		tokens_list, err := c.w.GetSmartContractTokenByDeployer(user_did)
		if err != nil {
			response.Message = "failed to fetch all smart contract tokens"
			return response, err
		}
		//Validate smart contract tokenchain for each smart contrtact in the smart contract table
		for _, token_info := range tokens_list {
			//Get token type
			type_string := SmartContractString
			token_type := c.TokenType(type_string)

			response, err = c.ValidateSmartContractTokenChain(user_did, token_info, token_type, blockCount)
			if err != nil || !response.Status {
				c.log.Error("token chain validation failed for token:", token_info.SmartContractHash, "Error :", err, "msg:", response.Message)
				return response, err
			}
		}

	} else {
		//Fetch token information
		token_info, err := c.w.GetSmartContractToken(tokenId)
		if err != nil {
			response.Message = "Failed to get smart contract token, smart contract does not exist"
			return response, err
		}
		//Get token type
		type_string := SmartContractString
		token_type := c.TokenType(type_string)
		//Validate tokenchain for the provided token
		response, err = c.ValidateSmartContractTokenChain(user_did, token_info[0], token_type, blockCount)
		if err != nil || !response.Status {
			c.log.Error("token chain validation failed for token:", tokenId, "Error :", err, "msg:", response.Message)
			return response, err
		}
	}
	return response, nil
}

// Validates tokenchain for the given token upto the specified block height
func (c *Core) ValidateSmartContractTokenChain(user_did string, token_info wallet.SmartContract, token_type int, blockCount int) (*model.BasicResponse, error) {
	c.log.Info("validating smart copntract tokenchain", token_info.SmartContractHash)
	response := &model.BasicResponse{
		Status: false,
	}

	validated_block_count := 0
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
		blocks, nextBlockID, err = c.w.GetAllTokenBlocks(token_info.SmartContractHash, token_type, blockId)
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
		//validated_block_count keeps count of the number of blocks validated, including failed blocks
		validated_block_count++

		if b != nil {
			//calculate block height
			block_height, err := b.GetBlockNumber(token_info.SmartContractHash)
			if err != nil {
				response.Message = "failed to fetch BlockNumber"
				return response, fmt.Errorf("invalid token chain block")
			}

			c.log.Info("validating at block height:", block_height)

			//fetch transaction type to validate the block accordingly
			txn_type := b.GetTransType()
			switch txn_type {
			case block.TokenDeployedType:
				prevBlockId := ""
				//validate smart contract deployed block
				response, err = c.Validate_SC_Block(b, token_info.SmartContractHash, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenExecutedType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(token_info.SmartContractHash)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate smart contract executed block
				response, err = c.Validate_SC_Block(b, token_info.SmartContractHash, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			default:
				prevBlockId := ""
				//validate smart contract deployed block
				response, err = c.Validate_SC_Block(b, token_info.SmartContractHash, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			}

		} else {
			c.log.Error("Invalid block")
		}

		c.log.Info("validated_block_count", validated_block_count)
		// //If blockCount is provided, then we will stop validating when we reach the blockCount
		// //If blockCount is not provided,i.e., is 0, then it will never be equal to validated_block_count
		// //and thus will be continued till genesis block
		if validated_block_count == blockCount {
			break
		}
	}

	//Get latest block in the token chain
	latestBlock := c.w.GetLatestTokenBlock(token_info.SmartContractHash, token_type)

	//Verify if the token is pinned only by the current owner aka receiver in the latest block
	response, err = c.CurrentOwnerPinCheck(latestBlock, token_info.SmartContractHash, user_did)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	//verify if the current token state is pinned by the quorums in the latest block
	response, err = c.CurrentQuorumStatePinCheck(latestBlock, token_info.SmartContractHash, token_type, user_did)
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
func (c *Core) Validate_SC_Block(b *block.Block, tokenId string, calculated_prevBlockId string, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenId, calculated_prevBlockId)
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
	response, err = c.ValidateQuorums(b, user_did)
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

// Deployer/Executor signature verification in a (non-genesis)block
func (c *Core) ValidateTxnInitiator(b *block.Block) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	var initiator string
	txn_type := b.GetTransType()
	if txn_type == block.TokenDeployedType {
		initiator = b.GetDeployerDID()
	} else if txn_type == block.TokenExecutedType {
		initiator = b.GetExecutorDID()
	} else {
		c.log.Info("Failed to identify transaction type, transaction initiated with old executable")
		response.Message = "Failed to identify transaction type"
		return response, nil
	}

	initiator_sign := b.GetInitiatorSignature()
	//check if it is a block addded to chain before adding initiator signature to block structure
	if initiator_sign == nil {
		c.log.Info("old block, initiator signature not found")
		response.Message = "old block, initiator signature not found"
		return response, nil
	} else if initiator_sign.DID != initiator {
		c.log.Info("invalid initiator, initiator did does not match")
		response.Message = "invalid initiator, initiator did does not match"
		return response, fmt.Errorf("invalid initiator, initiator did does not match")
	}

	var initiator_didType int
	//sign type = 0, means it is a BIP signature and the did was created in light mode
	//sign type = 1, means it is an NLSS-based signature and the did was created using NLSS scheme
	//and thus the did could be initialised in basic mode to fetch the public key
	if initiator_sign.SignType == 0 {
		initiator_didType = did.LiteDIDMode
	} else {
		initiator_didType = did.BasicDIDMode
	}

	//Initialise initiator did
	didCrypto, err := c.InitialiseDID(initiator, initiator_didType)
	if err != nil {
		c.log.Error("failed to initialise initiator did:", initiator)
		response.Message = "failed to initialise initiator did"
		return response, err
	}

	//initiator signature verification
	if initiator_didType == did.LiteDIDMode {
		response.Status, err = didCrypto.PvtVerify([]byte(initiator_sign.Hash), util.StrToHex(initiator_sign.Private_sign))
		if err != nil {
			c.log.Error("failed to verify initiator:", initiator, "err", err)
			response.Message = "invalid initiator"
			return response, err
		}
	} else {
		response.Status, err = didCrypto.NlssVerify(initiator_sign.Hash, util.StrToHex(initiator_sign.NLSS_share), util.StrToHex(initiator_sign.Private_sign))
		if err != nil {
			c.log.Error("failed to verify initiator:", initiator, "err", err)
			response.Message = "invalid initiator"
			return response, err
		}
	}

	response.Message = "initiator validated successfully"
	c.log.Debug("initiator (deployer/executor) validated successfully")
	return response, nil
}
