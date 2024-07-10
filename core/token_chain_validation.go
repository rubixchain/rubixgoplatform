package core

import (
	"fmt"
	"sync"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) TokenChainValidation(user_did string, allMyTokens bool, tokenId string, blockCount int) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}
	if allMyTokens { //if provided the boolean flag 'allmyToken', all the tokens' chain from tokens table will be validated
		c.log.Info("Validating all tokens from your tokens table")
		tokens_list, err := c.w.GetAllTokens(user_did)
		if err != nil {
			response.Message = "failed to fetch all tokens"
			return response, err
		}
		for _, tkn := range tokens_list {
			//Fetch token information
			token_info, err := c.w.ReadToken(tkn.TokenID)
			if err != nil {
				response.Message = "Failed to get token chain block, token does not exist"
				return response, err
			}
			//Get token type
			type_string := RBTString
			if token_info.TokenValue < 1.0 {
				type_string = PartString
			}
			token_type := c.TokenType(type_string)
			//Validate tokenchain for each token in the tokens table
			response, err = c.ValidateTokenChain(user_did, token_info, token_type, blockCount)
			if err != nil || !response.Status {
				c.log.Error("token chain validation failed for token:", tkn.TokenID, "\nError :", err, "\nmsg:", response.Message)
				if token_info.TokenStatus == wallet.TokenIsFree {
					//if token chain validation failed and the validator is the current owner of the token,
					//then lock the token
					response, err = c.LockInvalidToken(tkn.TokenID, token_type, user_did)
					if err != nil {
						c.log.Error(response.Message, tkn.TokenID)
						return response, err
					}
					c.log.Info(response.Message, tkn.TokenID)
				} else {
					c.log.Error("token is not free, token state is", token_info.TokenStatus)
				}
			}
		}

	} else {
		//Fetch token information
		token_info, err := c.w.ReadToken(tokenId)
		if err != nil {
			response.Message = "Failed to get token chain block, token does not exist"
			return response, err
		}

		//Get token type
		type_string := RBTString
		if token_info.TokenValue < 1.0 {
			type_string = PartString
		}
		token_type := c.TokenType(type_string)
		//Validate tokenchain for the provided token
		response, err = c.ValidateTokenChain(user_did, token_info, token_type, blockCount)
		if err != nil || !response.Status {
			c.log.Error("token chain validation failed for token:", tokenId, "\nError :", err, "\nmsg:", response.Message)
			if token_info.TokenStatus == wallet.TokenIsFree {
				response, err1 := c.LockInvalidToken(tokenId, token_type, user_did)
				if err1 != nil {
					c.log.Error(response.Message, tokenId)
					return response, err1
				}
				c.log.Info(response.Message, tokenId)
				return response, err
			}
			return response, err
		}
	}
	return response, nil
}

// Validates tokenchain for the given token upto the specified block height
func (c *Core) ValidateTokenChain(user_did string, token_info *wallet.Token, token_type int, blockCount int) (*model.BasicResponse, error) {
	c.log.Info("--------validating tokenchain", token_info.TokenID, "---------")
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
		blocks, nextBlockID, err = c.w.GetAllTokenBlocks(token_info.TokenID, token_type, blockId)
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
			block_height, err := b.GetBlockNumber(token_info.TokenID)
			if err != nil {
				response.Message = "failed to fetch BlockNumber"
				return response, fmt.Errorf("invalid token chain block")
			}

			c.log.Info("validating at block height:", block_height)

			//fetch transaction type to validate the block accordingly
			txn_type := b.GetTransType()
			switch txn_type {
			case block.TokenTransferredType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(token_info.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate rbt transfer block
				response, err = c.Validate_RBTTransfer_Block(b, token_info.TokenID, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenGeneratedType:
				//validate genesis block
				response, err = c.ValidateGenesisBlock(b, *token_info, token_type, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenBurntType:
				//validate RBT burnt block
				response, err = c.Validate_RBTBurnt_Block(b, *token_info, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenPledgedType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(token_info.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate Pledged block
				response, err = c.Validate_Pledged_or_Unpledged_Block(b, token_info.TokenID, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenUnpledgedType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(token_info.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate Pledged block
				response, err = c.Validate_Pledged_or_Unpledged_Block(b, token_info.TokenID, prevBlockId, user_did)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenContractCommited:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(token_info.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate Pledged block
				response, err = c.Validate_Pledged_or_Unpledged_Block(b, token_info.TokenID, prevBlockId, user_did)
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
	latestBlock := c.w.GetLatestTokenBlock(token_info.TokenID, token_type)

	if latestBlock.GetTransType() == block.TokenTransferredType {
		//Verify if the token is pinned only by the current owner aka receiver in the latest block
		response, err = c.CurrentOwnerPinCheck(latestBlock, token_info.TokenID, user_did)
		if err != nil {
			c.log.Error("msg", response.Message)
			return response, err
		}

		//verify if the current token state is pinned by the quorums in the latest block
		response, err = c.CurrentQuorumStatePinCheck(latestBlock, token_info.TokenID, token_type, user_did)
		if err != nil {
			c.log.Error("msg", response.Message)
			return response, err
		}
	} else {
		//Verify if the token is pinned only by the current owner aka receiver in the latest block
		response, err = c.CurrentOwnerPinCheck(latestBlock, token_info.TokenID, user_did)
		if err != nil {
			c.log.Error("msg", response.Message)
			return response, err
		}
	}

	c.log.Info("token chain validated successfully")
	response.Message = "token chain validated successfully"
	response.Status = true
	return response, nil
}

// validate block of type: TokenTransferredType = "02" / TokenDeployedType = "09" / TokenExecutedType = "10"
func (c *Core) Validate_RBTTransfer_Block(b *block.Block, tokenId string, calculated_prevBlockId string, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenId, calculated_prevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}
	//Validate sender signature
	response, err = c.ValidateSender(b, tokenId)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//validate pledged quorum signature
	response, err = c.Validate_Owner_or_PledgedQuorum(b, user_did)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//validate all quorums' signatures
	response, err = c.ValidateQuorums(b, user_did)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	response.Status = true
	response.Message = "RBT transfer block validated successfully"
	c.log.Debug("successfully validated RBT transfer block")
	return response, nil
}

// validate block of type : TokenBurntType = "08"
func (c *Core) Validate_RBTBurnt_Block(b *block.Block, token_info wallet.Token, calculated_prevBlockId string, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, token_info.TokenID, calculated_prevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	//validate burnt-token owner signature
	response, err = c.Validate_Owner_or_PledgedQuorum(b, user_did)
	if err != nil {
		response.Message = "invalid token owner in RBT burnt block"
		c.log.Error("invalid token owner in RBT burnt block")
		return response, fmt.Errorf("failed to validate token owner in RBT burnt block")
	}

	response.Status = true
	response.Message = "RBT burnt block validated successfully"
	c.log.Debug("successfully validated RBT burnt block")
	return response, nil
}

// validate block of type : TokenPledgedType = "04" / TokenUnpledgedType = "06" / TokenContractCommited = "11"
func (c *Core) Validate_Pledged_or_Unpledged_Block(b *block.Block, tokenId string, calculated_prevBlockId string, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenId, calculated_prevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	//validate burnt-token owner signature
	response, err = c.Validate_Owner_or_PledgedQuorum(b, user_did)
	if err != nil {
		response.Message = "invalid token owner in RBT burnt block"
		c.log.Error("invalid token owner in RBT burnt block")
		return response, fmt.Errorf("failed to validate token owner in RBT burnt block")
	}

	response.Status = true
	response.Message = "RBT pledged/unpledged/committed block validated successfully"
	c.log.Debug("successfully validated RBT pledged/unpledged/committed block")
	return response, nil
}

// genesis block validation : validate block of type: TokenGeneratedType = "05"
func (c *Core) ValidateGenesisBlock(b *block.Block, token_info wallet.Token, token_type int, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash of genesis block
	response, err := c.ValidateBlockHash(b, token_info.TokenID, "")
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//initial token owner signature verification
	response, err = c.Validate_Owner_or_PledgedQuorum(b, user_did)
	if err != nil {
		response.Message = "invalid token owner in genesis block"
		c.log.Error("invalid token owner in genesis block")
		return response, fmt.Errorf("failed to validate token owner in genesis block")
	}

	//if part token, validate parent token chain
	if token_type == token.TestPartTokenType {
		response, err = c.Validate_ParentToken_LatestBlock(token_info.ParentTokenID, user_did)
		if err != nil {
			c.log.Error("msg", response.Message, "err", err)
			return response, err
		}
	}

	response.Status = true
	response.Message = "genesis block validated successfully"
	c.log.Debug("validated genesis block")
	return response, nil
}

// Validate Parent token latest block if token is part token
func (c *Core) Validate_ParentToken_LatestBlock(parent_tokenId string, user_did string) (*model.BasicResponse, error) {
	c.log.Debug("validating parent token chain latest block", parent_tokenId)
	response := &model.BasicResponse{
		Status: false,
	}

	parent_token_info, err := c.w.ReadToken(parent_tokenId)
	if err != nil {
		response.Message = "Failed to get parent token chain block, parent token does not exist"
		return response, err
	}

	if parent_token_info.TokenStatus != wallet.TokenIsBurnt {
		response.Message = "parent token not in burnt state"
		c.log.Error("msg", response.Message)
		return response, err
	}
	type_string := RBTString
	if parent_token_info.TokenValue < 1.0 {
		type_string = PartString
	}
	parent_token_type := c.TokenType(type_string)

	//Get latest block in the token chain
	parent_token_latestBlock := c.w.GetLatestTokenBlock(parent_tokenId, parent_token_type)
	response, err = c.Validate_RBTBurnt_Block(parent_token_latestBlock, *parent_token_info, "", user_did)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//if parent token is also a part token, then validate it's parent token latest block
	if parent_token_type == token.TestPartTokenType {
		response, err = c.Validate_ParentToken_LatestBlock(parent_token_info.ParentTokenID, user_did)
		if err != nil {
			c.log.Error("msg", response.Message, "err", err)
			return response, err
		}
	}

	c.log.Debug("validated parent tokenchain latest block:", parent_tokenId)
	response.Status = true
	response.Message = "validated parent tokenchain latest block"
	return response, nil
}

// Validate block hash and previous block hash
func (c *Core) ValidateBlockHash(b *block.Block, tokenId string, calculated_prevBlockId string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//fetching block hash from block map using key 'TCBlockHash'
	stored_blockHash, err := b.GetHash()
	if err != nil {
		c.log.Error("failed to fetch block hash")
		response.Message = "failed to fetch block hash, could not validate block hash"
		return response, err
	}

	//if previous block Id verification is not neessary, then calculated_prevBlockId can be paased as an empty string
	if calculated_prevBlockId != "" {
		//fetching previous block-Id from block map using key 'TTPreviousBlockIDKey'
		stored_prevBlockId, err := b.GetPrevBlockID(tokenId)
		if err != nil {
			c.log.Error("failed to fetch previous-block-Id")
			response.Message = "failed to fetch previous-block-Id, could not validate block hash"
			return response, err
		}

		//check the validity of the stored previous block-ID
		if calculated_prevBlockId != stored_prevBlockId {
			c.log.Error("previous-block-Id does not match")
			response.Message = "previous-block-Id does not match, block hash validation failed"
			return response, err
		}
	}

	//calculate block hash from block data
	calculated_blockHash, err := b.CalculateBlockHash()
	if err != nil {
		c.log.Error("err", err)
	}

	// block_hash_map := blockMap[TCBlockHashKey]
	if stored_blockHash != calculated_blockHash {
		c.log.Error("block hash does not match")
		c.log.Debug("stored block hash", stored_blockHash)
		c.log.Debug("calculated block hash", calculated_blockHash)
		response.Message = "block hash does not match, block hash validation failed"
		return response, fmt.Errorf("block hash does not match, block hash validation failed")
	}

	response.Status = true
	response.Message = "block hash validated succesfully"
	c.log.Debug("block hash validated")
	return response, nil
}

// sender signature verification in a (non-genesis)block
func (c *Core) ValidateSender(b *block.Block, tokenId string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	// block_map := b.GetBlockMap()
	sender := b.GetSenderDID()

	sender_sign := b.GetSenderSignature()
	//check if it is a block addded to chain before adding sender signature to block structure
	if sender_sign == nil {
		c.log.Info("old block, sender signature not found")
		response.Message = "old block, sender signature not found"
		return response, fmt.Errorf("old block, sender signature not found")
	} else if sender_sign.DID != sender {
		c.log.Info("invalid sender, sender did does not match")
		response.Message = "invalid sender, sender did does not match"
		return response, fmt.Errorf("invalid sender, sender did does not match")
	}

	var sender_didType int
	//sign type = 0, means it is a BIP signature and the did was created in light mode
	//sign type = 1, means it is an NLSS-based signature and the did was created using NLSS scheme
	//and thus the did could be initialised in basic mode to fetch the public key
	if sender_sign.SignType == 0 {
		sender_didType = did.LiteDIDMode
	} else {
		sender_didType = did.BasicDIDMode
	}

	//Initialise sender did
	didCrypto, err := c.InitialiseDID(sender, sender_didType)
	if err != nil {
		c.log.Error("failed to initialise sender did:", sender)
		response.Message = "failed to initialise sender did"
		return response, err
	}

	//sender signature verification
	if sender_didType == did.LiteDIDMode {
		response.Status, err = didCrypto.PvtVerify([]byte(sender_sign.Hash), util.StrToHex(sender_sign.Private_sign))
		if err != nil {
			c.log.Error("failed to verify sender:", sender, "err", err)
			response.Message = "invalid sender"
			return response, err
		}
	} else {
		response.Status, err = didCrypto.NlssVerify(sender_sign.Hash, util.StrToHex(sender_sign.NLSS_share), util.StrToHex(sender_sign.Private_sign))
		if err != nil {
			c.log.Error("failed to verify sender:", sender, "err", err)
			response.Message = "invalid sender"
			return response, err
		}
	}

	response.Message = "sender validated successfully"
	c.log.Debug("sender validated successfully")
	return response, nil
}

// pledged quorum signature verification
func (c *Core) Validate_Owner_or_PledgedQuorum(b *block.Block, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	signers, err := b.GetSigner()
	if err != nil {
		c.log.Error("failed to get signers", "err", err)
		return response, err
	}
	for _, signer := range signers {
		var dc did.DIDCrypto
		switch b.GetTransType() {
		case block.TokenGeneratedType, block.TokenBurntType:
			dc, err = c.SetupForienDID(signer, user_did)
			if err != nil {
				c.log.Error("failed to setup foreign DID", signer, "err", err)
				return response, err
			}
		default:
			dc, err = c.SetupForienDIDQuorum(signer, user_did)
			if err != nil {
				c.log.Error("failed to setup foreign DID quorum", signer, "err", err)
				return response, err
			}
		}
		err := b.VerifySignature(dc)
		if err != nil {
			c.log.Error("Failed to verify signature of signer", signer, "err", err)
			return response, err
		}
	}

	response.Status = true
	response.Message = "block validated successfully"
	c.log.Debug("validated signer (token owner / pledged quorum) successfully")
	return response, nil
}

// quorum signature validation
func (c *Core) ValidateQuorums(b *block.Block, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//signed data aka transaction Id
	signed_data := b.GetTid()
	quorum_sign_list, err := b.GetQuorumSignatureList()
	if err != nil || quorum_sign_list == nil {
		c.log.Error("failed to get quorum signature list")
	}

	response.Status = true
	for _, qrm := range quorum_sign_list {
		qrm_DIDCrypto, err := c.SetupForienDIDQuorum(qrm.DID, user_did)
		if err != nil {
			c.log.Error("failed to initialise quorum:", qrm.DID, "err", err)
			continue
		}
		var verificationStatus bool
		if qrm.SignType == "0" { //qrm sign type = 0, means qrm signature is BIP sign and DID is created in Lite mode
			verificationStatus, err = qrm_DIDCrypto.PvtVerify([]byte(signed_data), util.StrToHex(qrm.PrivSignature))
			if err != nil {
				c.log.Error("failed signature verification for quorum:", qrm.DID)
			}
		} else {
			verificationStatus, err = qrm_DIDCrypto.NlssVerify((signed_data), util.StrToHex(qrm.Signature), util.StrToHex(qrm.PrivSignature))
			if err != nil {
				c.log.Error("failed signature verification for quorum:", qrm.DID)
			}
		}
		response.Status = response.Status && verificationStatus
	}

	response.Message = "quorums validated successfully"
	c.log.Debug("validated all quorums successfully")
	return response, nil
}

// latest block owner(receiver) pin check
func (c *Core) CurrentOwnerPinCheck(b *block.Block, tokenId string, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//current owner should be the receiver in the latest block
	current_owner := b.GetOwner()
	var current_ownerPeerID string
	if current_owner == user_did {
		current_ownerPeerID = c.peerID
	} else {
		current_ownerPeerID = c.w.GetPeerID(current_owner)
	}

	results := make([]MultiPinCheckRes, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go c.pinCheck(tokenId, 0, current_ownerPeerID, "", results, &wg)
	wg.Wait()
	for i := range results {
		if results[i].Error != nil {
			c.log.Error("Error occured", "error", results[i].Error)
			response.Message = "Error while cheking Token multiple Pins"
			return response, results[i].Error
		}
		if results[i].Status {
			c.log.Error("Token has multiple owners", "token", results[i].Token, "owners", results[i].Owners)
			response.Message = "Token has multiple owners"
			return response, fmt.Errorf("token has multiple owners")
		}
	}

	response.Status = true
	response.Message = "current owner pin checked successfully"
	c.log.Debug("current owner pin checked successfully")
	return response, nil
}

// latest block pledged quorum pin check
func (c *Core) CurrentQuorumStatePinCheck(b *block.Block, tokenId string, token_type int, user_did string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//Get quorumList along with peerIds : QuorumList
	var quorumList []string
	quorum_sign_list, err := b.GetQuorumSignatureList()
	if err != nil || quorum_sign_list == nil {
		c.log.Error("failed to get quorum signature list from latest block")
		response.Message = "state pincheck failed"
		return response, err
	}
	for _, qrm := range quorum_sign_list {
		qrm_peerId := c.w.GetPeerID(qrm.DID)
		quorumList = append(quorumList, qrm_peerId+"."+qrm.DID)
	}

	tokenStateCheckResult := make([]TokenStateCheckResult, 1)
	c.log.Debug("entering validation to check if token state is exhausted")
	var wg sync.WaitGroup
	wg.Add(1)
	go c.checkTokenState(tokenId, user_did, 0, tokenStateCheckResult, &wg, quorumList, token_type)
	wg.Wait()

	for i := range tokenStateCheckResult {
		if tokenStateCheckResult[i].Error != nil {
			c.log.Error("Error occured", "error", tokenStateCheckResult[i].Error)
			response.Message = "Error while cheking Token State Message : " + tokenStateCheckResult[i].Message
			response.Status = false
			return response, tokenStateCheckResult[i].Error
		}
		if tokenStateCheckResult[i].Exhausted {
			c.log.Debug("Token state has been exhausted, Token being Double spent:", tokenStateCheckResult[i].Token)
			response.Message = tokenStateCheckResult[i].Message
			response.Status = false
			return response, fmt.Errorf("token state has been exhausted")
		}
		c.log.Debug("Token", tokenStateCheckResult[i].Token, "Message", tokenStateCheckResult[i].Message)
		response.Status = true
		response.Message = tokenStateCheckResult[i].Message
	}

	return response, nil
}
