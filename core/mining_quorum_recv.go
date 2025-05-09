package core

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

const fiveWeeksInSeconds = 5 * 7 * 24 * 60 * 60 // 5 weeks = 5 * 7 days * 24 hours * 60 minutes * 60 seconds

func (c *Core) ValidateCredits(did string, creditRequestValue int, pledgeDetails []model.PledgeHistory, miningToken model.NewTokenDetails) error {

	totalCredits := 0
	// // record, err := c.w.FindLatestTokenLevelAndNumber()
	// // if err != nil {
	// // 	c.log.Error("Failed to get latest token level and number from mining records. err:", err)
	// // }
	// // fmt.Printf("Highest Token: Level %d, Number %d", record.TokenLevel, record.TokenNumber)
	// fmt.Println("PledgeHistory details in ValidateCredits", pledgeDetails)
	for _, tokenInfo := range pledgeDetails {
		// Query miningLevel DB to avoid double mining
		// existingMining, err := c.QueryMiningRecord(tokenInfo.TransactionID, tokenInfo.TransferTokenID, tokenInfo.QuorumDID)
		// if existingMining.MiningID != "" && existingMining.RemainingCredits == 0 {
		// 	c.log.Error("Given record is already used for mining")
		// 	return fmt.Errorf("Given record is already used for mining")
		// }
		c.log.Debug("Validating credits for token: ", tokenInfo.TransferTokenID)
		validatingBlockNumberStr := (tokenInfo.TransferBlockID[0:1])
		validatingBlockNumber, err := strconv.Atoi(validatingBlockNumberStr)
		if err != nil {
			c.log.Error("Error converting block number to integer for credit validation:", err)
			return fmt.Errorf("failed to convert block number for token %s: %v", tokenInfo.TransferTokenID, err)
		}

		//Get the peers from week epoch
		var peers []string
		currentWeek := util.GetWeeksPassed()
		peers, err = c.getPeerWhoPinTokenEpoch(tokenInfo.TransferTokenID, currentWeek-1)
		if err != nil {
			c.log.Error("Error getting peers for token: ", tokenInfo.TransferTokenID, "err", err)
			return fmt.Errorf("failed to get peers for token %s: %v", tokenInfo.TransferTokenID, err)
		}
		// Connect with the nodes who pinned the current week epoch and get the latest tokenchain.
		err = c.SyncTokenChainFromListOfPeers(peers, tokenInfo.TransferTokenID, tokenInfo.TransferTokenType)
		if err != nil {
			c.log.Error("Error syncing tokenchain from peers for credit validation", "tokenID", tokenInfo.TransferTokenID, "err", err)
			return fmt.Errorf("failed to sync tokenchain for token %s: %v", tokenInfo.TransferTokenID, err)
		}

		// Get the transfer block which credits need to be validated
		transferBlockBytes, err := c.w.GetTokenBlock(tokenInfo.TransferTokenID, tokenInfo.TransferTokenType, tokenInfo.TransferBlockID)
		if err != nil {
			c.log.Error("Error getting token block for credit validation", "tokenID", tokenInfo.TransferTokenID, "blockID", tokenInfo.TransferBlockID, "err", err)
			return fmt.Errorf("failed to get token block for token %s: %v", tokenInfo.TransferTokenID, err)
		}

		transferBlock := block.InitBlock(transferBlockBytes, nil)

		// Check if block is TokenTransferredType
		if transferBlock.GetTransType() != block.TokenTransferredType {
			c.log.Error("Failed to verify credits; given block is not Token Transfer Block")
			return fmt.Errorf("invalid block type for token %s: not a token transfer block", tokenInfo.TransferTokenID)
		}

		// Get all the blocks
		var blocks [][]byte
		blockId := ""
		for {
			allBlocks, nextBlockID, err := c.w.GetAllTokenBlocks(tokenInfo.TransferTokenID, tokenInfo.TransferTokenType, blockId)
			if err != nil {
				c.log.Error("Failed to get token chain block for credit validation")
				return fmt.Errorf("failed to get token chain blocks for token %s: %v", tokenInfo.TransferTokenID, err)
			}
			blocks = append(blocks, allBlocks...)
			blockId = nextBlockID
			if nextBlockID == "" {
				break
			}
		}
		// Check whether the 5 weeks is passed
		currentEpoch := time.Now().Unix()

		c.log.Debug("Epoch difference is ", (currentEpoch - int64(transferBlock.GetEpoch()))) //TODO: Remove, Added for testing
		c.log.Debug("Days pledged is ", (currentEpoch-int64(transferBlock.GetEpoch()))/86400) //TODO: Remove, Added for testing

		// TODO:Revert below after testing, fiveweeks pass check
		nextBlockofTransfer := block.InitBlock(blocks[validatingBlockNumber+1], nil)
		// if (currentEpoch - int64(nextBlockofTransfer.GetEpoch())) < fiveWeeksInSeconds {
		// 	c.log.Error("Failed to validate credits; 5 weeks not passed after transaction")
		// 	c.log.Error("Validation failed for token: ", tokenInfo.TransferTokenID)
		// 	return fmt.Errorf("failed to validate credits for token %s: 5 weeks not passed after next block transaction", tokenInfo.TransferTokenID)
		// }

		// Validate transactionID
		validatingBlock := block.InitBlock(blocks[validatingBlockNumber], nil)
		validatingBlockTransactionID := validatingBlock.GetTid()
		if validatingBlockTransactionID != tokenInfo.TransactionID {
			c.log.Error("Invalid transaction ID for credit validation")
			return fmt.Errorf("invalid transaction ID for token %s", tokenInfo.TransferTokenID)
		}

		// Previous block ID validation
		if validatingBlockNumber <= 0 || validatingBlockNumber > len(blocks) {
			c.log.Error("Invalid block number for credit validation: %d", validatingBlockNumber)
			return fmt.Errorf("invalid block number %d for token %s", validatingBlockNumber, tokenInfo.TransferTokenID)
		}
		prevBlock := block.InitBlock(blocks[validatingBlockNumber-1], nil)
		var prevBlockIDfromTC string
		if prevBlock.GetMinerDID() != "" {
			prevBlockIDfromTC, err = prevBlock.GetMinedTokenBlockID(tokenInfo.TransferTokenID)
		} else {
			prevBlockIDfromTC, err = prevBlock.GetBlockID(tokenInfo.TransferTokenID)
		}
		if err != nil {
			c.log.Error("Invalid previous block for credit validation")
			return fmt.Errorf("failed to get previous block ID for token %s: %v", tokenInfo.TransferTokenID, err)
		}
		storedPrevBlockID, err := transferBlock.GetPrevBlockID(tokenInfo.TransferTokenID)
		if err != nil {
			c.log.Error("Failed to fetch previous-block-ID; could not validate block hash for credit validation")
			return fmt.Errorf("failed to fetch previous block ID for token %s: %v", tokenInfo.TransferTokenID, err)
		}

		if prevBlockIDfromTC != storedPrevBlockID {
			c.log.Error("Previous-block-ID does not match; block hash validation failed in credit validation")
			return fmt.Errorf("previous block ID mismatch for token %s", tokenInfo.TransferTokenID)
		}

		// Block hash validation
		storedBlockHash, err := transferBlock.GetHash()
		if err != nil {
			c.log.Error("Failed to fetch block hash; could not validate block hash in credit validation")
			return fmt.Errorf("failed to fetch block hash for token %s: %v", tokenInfo.TransferTokenID, err)
		}
		calculatedBlockHash, err := transferBlock.CalculateBlockHash()
		if err != nil {
			c.log.Error("Error calculating block hash:", err)
			return fmt.Errorf("failed to calculate block hash for token %s: %v", tokenInfo.TransferTokenID, err)
		}

		if storedBlockHash != calculatedBlockHash {
			// c.log.Debug("Stored block hash:", storedBlockHash)
			// c.log.Debug("Calculated block hash:", calculatedBlockHash)
			c.log.Error("Block hash does not match; block hash validation failed in credit validation")
			return fmt.Errorf("block hash mismatch for token %s", tokenInfo.TransferTokenID)
		}

		// Validate all quorums' signatures
		response, err := c.ValidateQuorums(transferBlock, did)
		if err != nil {
			c.log.Error("Failed to verify quorum signature for credit validation", response.Message, "err:", err)
			return fmt.Errorf("failed to verify quorum signature for token %s: %v", tokenInfo.TransferTokenID, err)
		}

		// Check for the transfer token values and credit values.
		b, err := c.getFromIPFS(tokenInfo.TransferTokenID)
		if err != nil {
			c.log.Error("Failed to get token details from IPFS for credit validation", "err", err, "token", tokenInfo.TransferTokenID)
			return fmt.Errorf("failed to get token details from IPFS for token %s: %v", tokenInfo.TransferTokenID, err)
		}
		isWholeToken, _ := token.CheckWholeToken(string(b), c.testNet)
		tt := token.RBTTokenType
		fetchedTransferTokenValue := float64(1)
		if !isWholeToken {
			blk := util.StrToHex(string(b))
			rb, err := rac.InitRacBlock(blk, nil)
			if err != nil {
				c.log.Error("Invalid token; invalid RAC block", "err", err)
				return fmt.Errorf("invalid RAC block for token %s: %v", tokenInfo.TransferTokenID, err)
			}

			tt = rac.RacType2TokenType(rb.GetRacType())

			if c.TokenType(PartString) == tt {
				fetchedTransferTokenValue = rb.GetRacValue()
			}
		}

		if fetchedTransferTokenValue != float64(tokenInfo.TransferTokenValue) {
			c.log.Error("Failed to verify transfer value check in credit validation")
			return fmt.Errorf("transfer value mismatch for token %s", tokenInfo.TransferTokenID)
		}
		// c.log.Debug("Transfer value check passed")

		// Check for the credit value
		var cumulativeCredits int64
		var creditsForEachQuorum int64
		// nextBlockofTransfer := block.InitBlock(blocks[validatingBlockNumber+1], nil)
		nextBlockofTransferEpoch := nextBlockofTransfer.GetEpoch()
		epochDiffWithNextBlock := nextBlockofTransferEpoch - transferBlock.GetEpoch()
		if int(tokenInfo.TransactionType) == 2 {
			cumulativeCredits = int64(epochDiffWithNextBlock) * int64(tokenInfo.TransferTokenValue)
			creditsForEachQuorum = cumulativeCredits / 5
		} else if int(tokenInfo.TransactionType) == 1 {
			cumulativeCredits = int64(epochDiffWithNextBlock) * int64(tokenInfo.TransferTokenValue) * 15
			creditsForEachQuorum = cumulativeCredits / 5
		}
		fmt.Println("creditsForEachQuorum", creditsForEachQuorum)
		fmt.Println("tokenInfo.TokenCredit", tokenInfo.TokenCredit)

		if creditsForEachQuorum < int64(tokenInfo.TokenCredit) {
			c.log.Error("Failed to verify credit value check in credit validation")
			return fmt.Errorf("credit value mismatch for token %s", tokenInfo.TransferTokenID)
		} else {
			c.log.Debug("Credit value check passed")
		}
		//TODO:Add a check whether with the given tokenLevel and tokenNumber, alredy a token is created or not(check pin on the token with rubix epoch)
		tokenID, err := c.w.CreateTokenID(miningToken)
		if err != nil {
			c.log.Error("Failed to create token ID for credit validation", "err", err)
			return fmt.Errorf("failed to create token ID for credit validation: %v", err)
		}

		peers, err = c.getPeerWhoPinTokenEpoch(tokenID, currentWeek)
		if err != nil {
			c.log.Error("Error getting peers for token: ", tokenInfo.TransferTokenID, "err", err)
			return fmt.Errorf("failed to get peers for token %s: %v", tokenInfo.TransferTokenID, err)
		}
		if peers != nil {
			return fmt.Errorf("cannot mine token ID %s: token already exists in the system; use a different token number and token level", tokenID)

		}
		// If all checks pass, add the tokens credit value to totalCredits.
		totalCredits += int(tokenInfo.TokenCredit)
		c.log.Debug("Credit validation passed. Adding credits:", tokenInfo.TokenCredit)
	}

	// After processing all tokens, check if totalCredits matches creditRequestValue.
	if totalCredits < creditRequestValue {
		c.log.Error("Total credits from tokens do not match requested value")
		return fmt.Errorf("total credits from tokens do not match requested value. Expected %d but got %d", creditRequestValue, totalCredits)
	}
	c.log.Debug("Total credit value matched.")
	return nil
}

func (c *Core) pinTokenEpoch(tokenId string, weekCount int) {
	toPin := fmt.Sprintf("%s-%d", tokenId, weekCount)
	reader := bytes.NewReader([]byte(toPin))
	newCID, err := c.ipfs.Add(reader)
	fmt.Println("PIN Week count : ", weekCount)  //TODO:REMOVE
	fmt.Println("CID when PINNING is :", newCID) // TODO:REMOVE
	if err != nil {
		c.log.Error("Failed to get CID for epoch pinning", "err", err, "tokenID", tokenId)
	}
	err = c.ipfs.Pin(string(newCID))
	if err != nil {
		c.log.Error("Failed to pin token epoch", "err", err, "tokenID", tokenId)
	} else {
		c.log.Info("Token successfully PINNED", "CID", newCID)
	}
}

func (c *Core) UnpinTokenEpoch(tokenId string, weekCount int) {

	toPin := fmt.Sprintf("%s-%d", tokenId, weekCount)
	reader := bytes.NewReader([]byte(toPin))
	newCid, err := c.ipfs.Add(reader, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	fmt.Println("UNPIN Week count : ", weekCount)  //TODO:REMOVE
	fmt.Println("CID when UNPINNING is :", newCid) // TODO:REMOVE
	if err != nil {
		c.log.Error("Failed to get CID for epoch unpinning", "err", err, "tokenID", tokenId)
	}
	err = c.ipfs.Unpin(string(newCid))
	if err != nil {
		c.log.Error("Failed to Unpin token epoch", "err", err, "tokenID", tokenId)
	} else {
		c.log.Info("Token successfully UN-PINNED", "CID", newCid)
	}
	c.ipfsRepoGc()

	/////
	peer, err := c.GetRoutingAddrs(newCid)
	if err != nil {
		c.log.Error("Routing findprovs failed", "CID:", newCid, "err:", err)
	}
	fmt.Println("Peers after unpinning are ", peer)

}

func (c *Core) getPeerWhoPinTokenEpoch(tokenID string, weekCount int) ([]string, error) {
	pinCheck := fmt.Sprintf("%s-%d", tokenID, weekCount)
	pinCheckStr := bytes.NewReader([]byte(pinCheck))
	newCID, _ := c.ipfs.Add(pinCheckStr, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	list, err := c.GetDHTddrs(newCID)
	if err != nil {
		c.log.Warn("DHT findprovs failed", "CID:", newCID, "err:", err)
	}
	if len(list) == 0 {
		list, err = c.GetRoutingAddrs(newCID)
		if err != nil {
			c.log.Error("Routing findprovs failed", "CID:", newCID, "err:", err)
			return nil, err
		}
	}
	if len(list) == 0 {
		c.log.Info("no peers found for CID:", newCID)
		return nil, err
	}
	return list, nil
}

// This function updates the week epoch pin when the week changes
func (c *Core) UpdateEpochPin() {
	const maxLookBackWeeks = 20 // adjust as needed to limit how far back it searches

	for {
		select {
		case <-c.epochPinningTicker.C:

			c.log.Info("Processing for token-weekEpoch pin updation")

			currentWeek := util.GetWeeksPassed()
			tokenIDs, err := c.w.GetTokenIDsWithoutNextBlockFromPledgeHistory()
			if err != nil {
				if strings.Contains(err.Error(), "no records found") {
					continue
				}
				c.log.Error("Failed to get tokenIDs for update week epoch pin", "err", err)
				continue
			}

			for _, tokenID := range tokenIDs {
				peerIDs, err := c.getPeerWhoPinTokenEpoch(tokenID, currentWeek)
				if err != nil {
					//c.log.Warn("Failed to find pins on token-weekEpoch for tokenID: " + tokenID + ", week:" + strconv.Itoa(currentWeek))
				}
				if len(peerIDs) == 0 {
					found := false
					lookBackLimit := currentWeek - maxLookBackWeeks
					if lookBackLimit < 0 {
						lookBackLimit = 0
					}

					for week := currentWeek - 1; week >= lookBackLimit; week-- {
						lastWeekPeers, err := c.getPeerWhoPinTokenEpoch(tokenID, week)
						if err != nil {
							if len(lastWeekPeers) == 0 || strings.Contains(err.Error(), "no peers found") {
								continue
							}
						}
						for _, pinnedPeerID := range lastWeekPeers {
							if pinnedPeerID == c.peerID {
								c.log.Info("Found pin by this node for", " week:", week, "and tokenID:", tokenID)

								c.UnpinTokenEpoch(tokenID, week)
								c.log.Info("Week Epoch UNPINNED for", " week:", week, " and tokenID:", tokenID)

								c.pinTokenEpoch(tokenID, currentWeek)
								c.log.Info("Week Epoch PINNED for", " week:", currentWeek, " and tokenID:", tokenID)

								found = true
								break
							}
						}
						if found {
							break
						}
					}

					if !found {
						// c.log.Warn("No pins found on token-weekEpoch for tokenID:" + tokenID + " after searching past " + strconv.Itoa(maxLookBackWeeks) + " weeks")
					}
				}
			}
		}
	}
}
