package core

import (
	"fmt"
	"strconv"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

const fiveWeeksInSeconds = 5 * 7 * 24 * 60 * 60 // 5 weeks = 5 * 7 days * 24 hours * 60 minutes * 60 seconds

func (c *Core) ValidateCredits(did string, creditRequestValue int, pledgeDetails []model.PledgeHistory) error {
	ok := c.w.IsDIDExist(did)
	if !ok {
		c.log.Error("Invalid did of mining quorum")
		return fmt.Errorf("invalid did of mining quorum: %v", did)
	}

	totalCredits := 0

	for _, tokenInfo := range pledgeDetails {
		c.log.Debug("Validating credits for token: ", tokenInfo.TransferTokenID)
		validatingBlockNumberStr := (tokenInfo.TransferBlockID[0:1])
		validatingBlockNumber, err := strconv.Atoi(validatingBlockNumberStr)
		if err != nil {
			c.log.Error("Error converting block number to integer for credit validation:", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Check the pins on the tokenstatehash when next block is created.
		peers, err := c.GetDHTddrs(tokenInfo.LatestTokenStateHash)
		if err != nil {
			c.log.Error("Error fetching pinned peers for credit validation", "tokenStateHash", tokenInfo.LatestTokenStateHash, "err", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Connect with the nodes who pinned the tokenstatehash and get the latest tokenchain.
		err = c.SyncTokenChainFromListOfPeers(peers, tokenInfo.TransferTokenID, tokenInfo.TransferTokenType)
		if err != nil {
			c.log.Error("Error syncing tokenchain from peers for credit validation", "tokenID", tokenInfo.TransferTokenID, "err", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Get the transfer block which credits need to be validated
		transferBlockArray, err := c.w.GetTokenBlock(tokenInfo.TransferTokenID, tokenInfo.TransferTokenType, tokenInfo.TransferBlockID)
		if err != nil {
			c.log.Error("Error getting token block for credit validation", "tokenID", tokenInfo.TransferTokenID, "blockID", tokenInfo.TransferBlockID, "err", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		transferBlock := block.InitBlock(transferBlockArray, nil)

		// Check whether the 5 weeks is passed
		currentEpoch := time.Now().Unix()
		if (currentEpoch - transferBlock.GetEpoch()) < fiveWeeksInSeconds {
			c.log.Error("Failed to validate credits; 5 weeks not passed after transaction")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Check if block is TokenTransferredType
		if transferBlock.GetTransType() != block.TokenTransferredType {
			c.log.Error("Failed to verify credits; given block is not Token Transfer Block")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Get all the blocks
		var blocks [][]byte
		blockId := ""
		for {
			allBlocks, nextBlockID, err := c.w.GetAllTokenBlocks(tokenInfo.TransferTokenID, tokenInfo.TransferTokenType, blockId)
			if err != nil {
				c.log.Error("Failed to get token chain block for credit validation")
				c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
				continue
			}
			blocks = append(blocks, allBlocks...)
			blockId = nextBlockID
			if nextBlockID == "" {
				break
			}
		}

		// Previous block ID validation
		if validatingBlockNumber <= 0 || validatingBlockNumber > len(blocks) {
			c.log.Error("Invalid block number for credit validation: %d", validatingBlockNumber)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}
		prevBlock := block.InitBlock(blocks[validatingBlockNumber-1], nil)
		prevBlockIDfromTC, err := prevBlock.GetBlockID(tokenInfo.TransferTokenID)
		if err != nil {
			c.log.Error("Invalid previous block for credit validation")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}
		storedPrevBlockID, err := transferBlock.GetPrevBlockID(tokenInfo.TransferTokenID)
		if err != nil {
			c.log.Error("Failed to fetch previous-block-ID; could not validate block hash for credit validation")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		if prevBlockIDfromTC != storedPrevBlockID {
			c.log.Error("Previous-block-ID does not match; block hash validation failed in credit validation")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Block hash validation
		storedBlockHash, err := transferBlock.GetHash()
		if err != nil {
			c.log.Error("Failed to fetch block hash; could not validate block hash in credit validation")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}
		calculatedBlockHash, err := transferBlock.CalculateBlockHash()
		if err != nil {
			c.log.Error("Error calculating block hash:", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		if storedBlockHash != calculatedBlockHash {
			c.log.Debug("Stored block hash:", storedBlockHash)
			c.log.Debug("Calculated block hash:", calculatedBlockHash)
			c.log.Error("Block hash does not match; block hash validation failed in credit validation")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Check if the requesting miner is signed in the respective transfer block of the TC.
		// Validate sender signature
		response, err := c.ValidateSender(transferBlock)
		if err != nil {
			c.log.Error("Failed to verify sender for credit validation,", response.Message, "err:", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Validate all quorums' signatures
		response, err = c.ValidateQuorums(transferBlock, did)
		if err != nil {
			c.log.Error("Failed to verify quorum signature for credit validation", response.Message, "err:", err)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}

		// Check for the transfer token values and credit values.
		b, err := c.getFromIPFS(tokenInfo.TransferTokenID)
		if err != nil {
			c.log.Error("Failed to get token details from IPFS for credit validation", "err", err, "token", tokenInfo.TransferTokenID)
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		}
		_, isWholeToken, _ := token.CheckWholeToken(string(b), c.testNet)
		tt := token.RBTTokenType
		fetchedTransferTokenValue := float64(1)
		if !isWholeToken {
			blk := util.StrToHex(string(b))
			rb, err := rac.InitRacBlock(blk, nil)
			if err != nil {
				c.log.Error("Invalid token; invalid RAC block", "err", err)
				c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
				continue
			}

			tt = rac.RacType2TokenType(rb.GetRacType())

			if c.TokenType(PartString) == tt {
				fetchedTransferTokenValue = rb.GetRacValue()
			}
		}

		if fetchedTransferTokenValue != float64(tokenInfo.TransferTokenValue) {
			c.log.Error("Failed to verify transfer value check in credit validation")
			c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
			continue
		} else {
			c.log.Debug("Transfer value check passed")
		}

		// Check for the credit value
		var credits int64
		nextBlockofTransfer := block.InitBlock(blocks[validatingBlockNumber+1], nil)
		nextBlockofTransferEpoch := nextBlockofTransfer.GetEpoch()
		epochDiffWithNextBlock := nextBlockofTransferEpoch - transferBlock.GetEpoch()
		if int(tokenInfo.TransactionType) == 2 {
			credits = epochDiffWithNextBlock * int64(tokenInfo.TransferTokenValue)
		} else if int(tokenInfo.TransactionType) == 1 {
			credits = epochDiffWithNextBlock * int64(tokenInfo.TransferTokenValue) * 15
		}

		if credits != int64(tokenInfo.TokenCredit) {
			c.log.Error("Failed to verify credit value check in credit validation")
			c.log.Error("Validation failed for token:", tokenInfo.TransferTokenID)
			continue
		} else {
			c.log.Debug("Credit value check passed")
		}

		// If all checks pass, add the tokens credit value to totalCredits.
		totalCredits += int(tokenInfo.TokenCredit)
		c.log.Debug("Credit validation passed. Adding credits:", tokenInfo.TokenCredit)
	}

	// After processing all tokens, check if totalCredits matches creditRequestValue.
	if totalCredits != creditRequestValue {
		c.log.Error("Total credits from tokens do not match requested value")
		return fmt.Errorf("Total credits from tokens do not match requested value. Expected %d but got %d.", creditRequestValue, totalCredits)
	}
	c.log.Debug("Total credit value matched.")
	return nil
}
