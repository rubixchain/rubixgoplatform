package core

import (
	"fmt"
	"sync"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) TokenChainValidation(userDID string, tokenId string, blockCount int) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}
	ok := c.w.IsDIDExist(userDID)
	if !ok {
		response.Message = "Invalid did, please pass did of the tokenchain validator"
		return response, fmt.Errorf("invalid did: %v, please pass did of the tokenchain validator", userDID)
	}

	if tokenId == "" { //if provided the boolean flag 'allmyToken', all the tokens' chain from tokens table will be validated
		c.log.Info("Validating all tokens from your tokens table")
		tokensList, err := c.w.GetAllTokens(userDID)
		if err != nil {
			response.Message = "failed to fetch all tokens"
			return response, err
		}
		for _, tkn := range tokensList {
			//Fetch token information
			tokenInfo, err := c.w.ReadToken(tkn.TokenID)
			if err != nil {
				response.Message = "Failed to get token chain block, token does not exist"
				return response, err
			}
			//Get token type
			typeString := RBTString
			if tokenInfo.TokenValue < 1.0 {
				typeString = PartString
			}
			tokenType := c.TokenType(typeString)
			//Validate tokenchain for each token in the tokens table
			response, err = c.ValidateTokenChain(userDID, tokenInfo, tokenType, blockCount)
			if err != nil || !response.Status {
				c.log.Error("token chain validation failed for token:", tkn.TokenID, "Error :", err, "msg:", response.Message)
				return response, err
			}
		}

	} else {
		//Fetch token information
		tokenInfo, err := c.w.ReadToken(tokenId)
		if err != nil {
			response.Message = "Failed to get token chain block, token does not exist"
			return response, err
		}

		//Get token type
		typeString := RBTString
		if tokenInfo.TokenValue < 1.0 {
			typeString = PartString
		}
		tokenType := c.TokenType(typeString)
		//Validate tokenchain for the provided token
		response, err = c.ValidateTokenChain(userDID, tokenInfo, tokenType, blockCount)
		if err != nil || !response.Status {
			c.log.Error("token chain validation failed for token:", tokenId, "Error :", err, "msg:", response.Message)
			return response, err
		}
	}
	return response, nil
}

// Validates tokenchain for the given token upto the specified block height
func (c *Core) ValidateTokenChain(userDID string, tokenInfo *wallet.Token, tokenType int, blockCount int) (*model.BasicResponse, error) {
	c.log.Info("--------validating tokenchain", tokenInfo.TokenID, "---------")
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
		blocks, nextBlockID, err = c.w.GetAllTokenBlocks(tokenInfo.TokenID, tokenType, blockId)
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
			blockHeight, err := b.GetBlockNumber(tokenInfo.TokenID)
			if err != nil {
				response.Message = "failed to fetch BlockNumber"
				return response, fmt.Errorf("invalid token chain block")
			}

			c.log.Info("validating at block height:", blockHeight)

			//fetch transaction type to validate the block accordingly
			txnType := b.GetTransType()
			switch txnType {
			case block.TokenTransferredType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(tokenInfo.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate rbt transfer block
				response, err = c.ValidateRBTTransferBlock(b, tokenInfo.TokenID, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenGeneratedType:
				//validate genesis block
				response, err = c.ValidateGenesisBlock(b, *tokenInfo, tokenType, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenBurntType:
				//validate RBT burnt block
				response, err = c.ValidateRBTBurntBlock(b, *tokenInfo, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenPledgedType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(tokenInfo.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate Pledged block
				response, err = c.ValidatePledgedUnpledgedBlock(b, tokenInfo.TokenID, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenUnpledgedType:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(tokenInfo.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate Pledged block
				response, err = c.ValidatePledgedUnpledgedBlock(b, tokenInfo.TokenID, prevBlockId, userDID)
				if err != nil {
					c.log.Error("msg", response.Message, "err", err)
					return response, err
				}
			case block.TokenContractCommited:
				//calculate previous block Id
				prevBlock := block.InitBlock(blocks[i-1], nil)
				prevBlockId, err = prevBlock.GetBlockID(tokenInfo.TokenID)
				if err != nil {
					c.log.Error("invalid previous block")
					continue
				}
				//validate Pledged block
				response, err = c.ValidatePledgedUnpledgedBlock(b, tokenInfo.TokenID, prevBlockId, userDID)
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
	latestBlock := c.w.GetLatestTokenBlock(tokenInfo.TokenID, tokenType)

	if latestBlock.GetTransType() == block.TokenTransferredType {
		//Verify if the token is pinned only by the current owner aka receiver in the latest block
		response, err = c.CurrentOwnerPinCheck(latestBlock, tokenInfo.TokenID, userDID)
		if err != nil {
			c.log.Error("msg", response.Message)
			return response, err
		}

		//verify if the current token state is pinned by the quorums in the latest block
		response, err = c.CurrentQuorumStatePinCheck(latestBlock, tokenInfo.TokenID, tokenType, userDID)
		if err != nil {
			c.log.Error("msg", response.Message)
			return response, err
		}
	} else {
		//Verify if the token is pinned only by the current owner aka receiver in the latest block
		response, err = c.CurrentOwnerPinCheck(latestBlock, tokenInfo.TokenID, userDID)
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
func (c *Core) ValidateRBTTransferBlock(b *block.Block, tokenId string, calculatedPrevBlockId string, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenId, calculatedPrevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}
	//Validate sender signature
	response, err = c.ValidateSender(b)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//validate all quorums' signatures
	response, err = c.ValidateQuorums(b, userDID)
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
func (c *Core) ValidateRBTBurntBlock(b *block.Block, tokenInfo wallet.Token, calculatedPrevBlockId string, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenInfo.TokenID, calculatedPrevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	//validate burnt-token owner signature
	response, err = c.ValidateTokenOwner(b, userDID)
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
func (c *Core) ValidatePledgedUnpledgedBlock(b *block.Block, tokenId string, calculatedPrevBlockId string, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash
	response, err := c.ValidateBlockHash(b, tokenId, calculatedPrevBlockId)
	if err != nil {
		c.log.Error("msg", response.Message)
		return response, err
	}

	//validate burnt-token owner signature
	response, err = c.ValidateTokenOwner(b, userDID)
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
func (c *Core) ValidateGenesisBlock(b *block.Block, tokenInfo wallet.Token, tokenType int, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{}

	//Validate block hash of genesis block
	response, err := c.ValidateBlockHash(b, tokenInfo.TokenID, "")
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//initial token owner signature verification
	response, err = c.ValidateTokenOwner(b, userDID)
	if err != nil {
		response.Message = "invalid token owner in genesis block"
		c.log.Error("invalid token owner in genesis block")
		return response, fmt.Errorf("failed to validate token owner in genesis block")
	}

	//if part token, validate parent token chain
	if tokenType == token.TestPartTokenType {
		response, err = c.ValidateParentTokenLatestBlock(tokenInfo.ParentTokenID, userDID)
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
func (c *Core) ValidateParentTokenLatestBlock(parentTokenId string, userDID string) (*model.BasicResponse, error) {
	c.log.Debug("validating parent token chain latest block", parentTokenId)
	response := &model.BasicResponse{
		Status: false,
	}

	parentTokenInfo, err := c.w.ReadToken(parentTokenId)
	if err != nil {
		b, err := c.getFromIPFS(parentTokenId)
		if err != nil {
			c.log.Error("failed to get parent token detials from ipfs", "err", err, "token", parentTokenId)
			response.Message = "failed to get parent token detials from ipfs"
			return response, err
		}

		_, iswholeToken, _ := token.CheckWholeToken(string(b), c.testNet)
		tokenType := token.RBTTokenType
		tokenValue := float64(1)
		tokenOwner := ""
		if !iswholeToken {
			blk := util.StrToHex(string(b))
			rb, err := rac.InitRacBlock(blk, nil)
			if err != nil {
				c.log.Error("invalid token, invalid rac block", "err", err)
				response.Message = "invalid token, invalid rac block"
				return response, err
			}
			tokenType = rac.RacType2TokenType(rb.GetRacType())
			if c.TokenType(PartString) == tokenType {
				tokenValue = rb.GetRacValue()
			}
			tokenOwner = rb.GetDID()
		}
		parentTokenInfo = &wallet.Token{
			TokenID:     parentTokenId,
			TokenValue:  tokenValue,
			TokenStatus: wallet.TokenIsBurnt,
			DID:         tokenOwner,
		}
	}

	if parentTokenInfo.TokenStatus != wallet.TokenIsBurnt {
		response.Message = "parent token not in burnt state"
		c.log.Error("msg", response.Message)
		return response, err
	}
	typeString := RBTString
	if parentTokenInfo.TokenValue < 1.0 {
		typeString = PartString
	}
	parentTokenType := c.TokenType(typeString)

	//Get latest block in the token chain
	parentTokenLatestBlock := c.w.GetLatestTokenBlock(parentTokenId, parentTokenType)
	response, err = c.ValidateRBTBurntBlock(parentTokenLatestBlock, *parentTokenInfo, "", userDID)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		return response, err
	}

	//if parent token is also a part token, then validate it's parent token latest block
	if parentTokenType == c.TokenType(PartString) {
		if parentTokenInfo.ParentTokenID == "" {
			genesisBlock := c.w.GetGenesisTokenBlock(parentTokenId, parentTokenType)
			grandParentToken, _, err := genesisBlock.GetParentDetials(parentTokenId)
			if err != nil {
				c.log.Error("failed to get grand parent tokens to validate")
			}
			c.log.Debug("grand parent token:", grandParentToken)
			parentTokenInfo.ParentTokenID = grandParentToken
		}
		response, err = c.ValidateParentTokenLatestBlock(parentTokenInfo.ParentTokenID, userDID)
		if err != nil {
			c.log.Error("msg", response.Message, "err", err)
			return response, err
		}
	}

	c.log.Debug("validated parent tokenchain latest block:", parentTokenId)
	response.Status = true
	response.Message = "validated parent tokenchain latest block"
	return response, nil
}

// Validate block hash and previous block hash
func (c *Core) ValidateBlockHash(b *block.Block, tokenId string, calculatedPrevBlockId string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//fetching block hash from block map using key 'TCBlockHash'
	storedBlockHash, err := b.GetHash()
	if err != nil {
		c.log.Error("failed to fetch block hash")
		response.Message = "failed to fetch block hash, could not validate block hash"
		return response, err
	}

	//if previous block Id verification is not neessary, then calculatedPrevBlockId can be paased as an empty string
	if calculatedPrevBlockId != "" {
		//fetching previous block-Id from block map using key 'TTPreviousBlockIDKey'
		storedPrevBlockId, err := b.GetPrevBlockID(tokenId)
		if err != nil {
			c.log.Error("failed to fetch previous-block-Id")
			response.Message = "failed to fetch previous-block-Id, could not validate block hash"
			return response, err
		}

		//check the validity of the stored previous block-ID
		if calculatedPrevBlockId != storedPrevBlockId {
			c.log.Error("previous-block-Id does not match")
			response.Message = "previous-block-Id does not match, block hash validation failed"
			return response, err
		}
	}

	//calculate block hash from block data
	calculatedBlockHash, err := b.CalculateBlockHash()
	if err != nil {
		c.log.Error("err", err)
	}

	if storedBlockHash != calculatedBlockHash {
		c.log.Error("block hash does not match")
		c.log.Debug("stored block hash", storedBlockHash)
		c.log.Debug("calculated block hash", calculatedBlockHash)
		response.Message = "block hash does not match, block hash validation failed"
		return response, fmt.Errorf("block hash does not match, block hash validation failed")
	}

	response.Status = true
	response.Message = "block hash validated succesfully"
	c.log.Debug("block hash validated")
	return response, nil
}

// sender signature verification in a (non-genesis)block
func (c *Core) ValidateSender(b *block.Block) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	senderSign := b.GetInitiatorSignature()
	//check if it is a block addded to chain before adding sender signature to block structure
	if senderSign == nil {
		c.log.Info("old block, sender signature not found")
		response.Message = "old block, sender signature not found"
		return response, nil
		// } else if senderSign.DID != sender {
		// 	c.log.Info("invalid sender, sender did does not match; sender did in block : ", senderSign.DID, "sender did ", sender)
		// 	response.Message = "invalid sender, sender did does not match"
		// 	return response, fmt.Errorf("invalid sender, sender did does not match")
	}

	sender := senderSign.DID

	var senderDIDType int
	//sign type = 0, means it is a BIP signature and the did was created in light mode
	//sign type = 1, means it is an NLSS-based signature and the did was created using NLSS scheme
	//and thus the did could be initialised in basic mode to fetch the public key
	if senderSign.SignType == 0 {
		senderDIDType = did.LiteDIDMode
	} else {
		senderDIDType = did.BasicDIDMode
	}

	//Initialise sender did
	didCrypto, err := c.InitialiseDID(sender, senderDIDType)
	if err != nil {
		c.log.Error("failed to initialise sender did:", sender)
		response.Message = "failed to initialise sender did"
		return response, err
	}

	//sender signature verification
	if senderDIDType == did.LiteDIDMode {
		response.Status, err = didCrypto.PvtVerify([]byte(senderSign.Hash), util.StrToHex(senderSign.PrivateSign))
		if err != nil {
			c.log.Error("failed to verify sender:", sender, "err", err)
			response.Message = "invalid sender"
			return response, err
		}
	} else {
		response.Status, err = didCrypto.NlssVerify(senderSign.Hash, util.StrToHex(senderSign.NLSSShare), util.StrToHex(senderSign.PrivateSign))
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

// token owner signature verification
func (c *Core) ValidateTokenOwner(b *block.Block, userDID string) (*model.BasicResponse, error) {
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
			dc, err = c.SetupForienDID(signer, userDID)
			if err != nil {
				c.log.Error("failed to setup foreign DID", signer, "err", err)
				return response, err
			}
		default:
			dc, err = c.SetupForienDIDQuorum(signer, userDID)
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
	c.log.Debug("validated token owner successfully")
	return response, nil
}

// quorums signature validation
func (c *Core) ValidateQuorums(b *block.Block, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//signed data aka transaction Id
	signedData := b.GetTid()
	quorumSignList, err := b.GetQuorumSignatureList()
	if err != nil || quorumSignList == nil {
		c.log.Error("failed to get quorum signature list")
	}

	response.Status = true
	for _, qrm := range quorumSignList {
		qrmDIDCrypto, err := c.SetupForienDIDQuorum(qrm.DID, userDID)
		if err != nil {
			c.log.Error("failed to initialise quorum:", qrm.DID, "err", err)
			continue
		}
		var verificationStatus bool
		if qrm.SignType == "0" { //qrm sign type = 0, means qrm signature is BIP sign and DID is created in Lite mode
			verificationStatus, err = qrmDIDCrypto.PvtVerify([]byte(signedData), util.StrToHex(qrm.PrivSignature))
			if err != nil {
				c.log.Error("failed signature verification for quorum:", qrm.DID)
			}
		} else {
			verificationStatus, err = qrmDIDCrypto.NlssVerify((signedData), util.StrToHex(qrm.Signature), util.StrToHex(qrm.PrivSignature))
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
func (c *Core) CurrentOwnerPinCheck(b *block.Block, tokenId string, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//current owner should be the receiver in the latest block
	currentOwner := b.GetOwner()
	var currentOwnerPeerID string
	currentOwnerInfo, err := c.GetPeerDIDInfo(currentOwner)
	if currentOwnerInfo == nil || currentOwnerInfo.PeerID == "" {
		c.log.Error("failed to fetchh current owner peer id for pincheck, current owner ", currentOwner)
		response.Message = fmt.Sprintf("failed to fetchh current owner peer id for pincheck, current owner : %v", currentOwner)
		return response, fmt.Errorf(response.Message+"err : %v", err)
	}

	results := make([]MultiPinCheckRes, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go c.pinCheck(tokenId, 0, currentOwnerPeerID, "", results, &wg)
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
func (c *Core) CurrentQuorumStatePinCheck(b *block.Block, tokenId string, tokenType int, userDID string) (*model.BasicResponse, error) {
	response := &model.BasicResponse{
		Status: false,
	}

	//Get quorumList along with peerIds : QuorumList
	var quorumList []string
	quorumSignList, err := b.GetQuorumSignatureList()
	if err != nil || quorumSignList == nil {
		c.log.Error("failed to get quorum signature list from latest block")
		response.Message = "state pincheck failed"
		return response, err
	}
	for _, qrm := range quorumSignList {
		qrmInfo, err := c.GetPeerDIDInfo(qrm.DID)
		if qrmInfo == nil || qrmInfo.PeerID == "" {
			c.log.Error("failed to fetchh qrm peer id, qrm ", qrm.DID)
			response.Message = fmt.Sprintf("failed to fetchh qrm peer id for pincheck, qrm : %v", qrm.DID)
			return response, fmt.Errorf("failed to fetch qrm peer id, err %v ", err)
		}
		quorumList = append(quorumList, qrmInfo.PeerID+"."+qrm.DID)
	}

	tokenStateCheckResult := make([]TokenStateCheckResult, 1)
	c.log.Debug("entering validation to check if token state is exhausted")
	var wg sync.WaitGroup
	wg.Add(1)
	go c.checkTokenState(tokenId, userDID, 0, tokenStateCheckResult, &wg, quorumList, tokenType)
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
