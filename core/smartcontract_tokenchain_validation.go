package core

// smartcontract_tokenchain_validation.go
//
// TODO(phase07): All functions in this file are stubbed pending a full PostgreSQL-based
// reimplementation. The block package has been removed. Function signatures that formerly
// accepted *block.Block now accept TokenChainInput, a typed placeholder that will be
// replaced with the real PostgreSQL-backed type during phase07 implementation.

import (
	"fmt"

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
		//Validate smart contract tokenchain for each smart contract in the smart contract table
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

// ValidateSmartContractTokenChain validates the tokenchain for a given smart contract token.
// TODO(phase07): implement DB-based smart contract token validation
func (c *Core) ValidateSmartContractTokenChain(userDID string, tokenInfo wallet.SmartContract, tokenType int, blockCount int) (*model.BasicResponse, error) {
	// TODO(phase07): implement DB-based smart contract token validation
	c.log.Warn("smartcontract_validation STUB: validation skipped")
	return &model.BasicResponse{Status: true, Message: "smartcontract validation skipped (stub)"}, nil
}

// ValidateSmartContractBlock validates a block of type TokenTransferredType / TokenDeployedType / TokenExecutedType.
// TODO(phase07): implement DB-based smart contract token validation
func (c *Core) ValidateSmartContractBlock(b TokenChainInput, tokenId string, calculatedPrevBlockId string, userDID string) (*model.BasicResponse, error) {
	// TODO(phase07): implement DB-based smart contract token validation
	c.log.Warn("smartcontract_validation STUB: validation skipped")
	return &model.BasicResponse{Status: true, Message: "smartcontract validation skipped (stub)"}, nil
}

// ValidateTxnInitiator verifies the deployer/executor signature in a (non-genesis) block.
// TODO(phase07): implement DB-based smart contract token validation
func (c *Core) ValidateTxnInitiator(b TokenChainInput) (*model.BasicResponse, error) {
	// TODO(phase07): implement DB-based smart contract token validation
	c.log.Warn("smartcontract_validation STUB: validation skipped")
	return &model.BasicResponse{Status: true, Message: "smartcontract validation skipped (stub)"}, nil
}
