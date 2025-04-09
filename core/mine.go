package core

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/syndtr/goleveldb/leveldb"

	// "github.com/rubixchain/rubixgoplatform/did"

	// "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	tokenLevel   = 004
	tokenNumber  = 1000000
	miningPubSub = "mining-service"
)

func (c *Core) InitiateMineRBTs(reqID string, req *model.MiningRequest) {
	br := c.initiateMineRBTs(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}
func (c *Core) initiateMineRBTs(reqID string, MiningReq *model.MiningRequest) *model.BasicResponse {
	fmt.Println("Executing MineRBTs function")

	resp := &model.BasicResponse{
		Status: false,
	}

	// 1. Fetch pledge history records where tokenCreditStatus = 1 (ready to mine)
	tokenDetails, err := c.w.GetTokenDetailsByQuorumDID(MiningReq.MinerDid, 1)
	if err != nil {
		resp.Message = "Failed to fetch pledge history, " + err.Error()
		return resp // Return error if fetching fails
	}

	// 2. Calculate total token credit
	var totalCredits uint64
	totalCredits = 0
	for _, tokenDetail := range tokenDetails {
		totalCredits += tokenDetail.TokenCredit
	}

	// TODO: Sync the mining pubsub get the latest entry, get the next token level and number
	// 3. Check how many credits are needed for mining the next token according to token level
	var creditsForNextToken uint64
	creditsForNextToken = 100 // TODO: Fetch from mining chain if dynamic

	// 4. Validate if total credits are sufficient
	if totalCredits < creditsForNextToken {
		resp.Message = fmt.Sprintf("Total credits (%d) are less than the required credits (%d) to mine the next token", totalCredits, creditsForNextToken)
		return resp // Return an error
	}
    
	selectedTokenCredits,remainingCredits,totalSelectedCredits:= wallet.CollectRequiredCredits(tokenDetails,creditsForNextToken)
	

	didCryptoLib, err := c.SetupDID(reqID, MiningReq.MinerDid)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	//3.take all those token details and send it to mining quorum(to do that we might need an internal API)
	MiningContractDetails := &contract.ContractType{
		Type:               contract.MineRBTType,
		PledgeMode:         contract.PeriodicPledgeMode,
		ReqTokenCredits:    totalSelectedCredits,
		TokenCreditDetails: selectedTokenCredits,
		RemainingCredits: remainingCredits,
		ReqID:              reqID,
		TransInfo: &contract.TransInfo{
			MinerDID: MiningReq.MinerDid,
		},
	}
	miningContract := contract.CreateNewContract(MiningContractDetails)

	err = miningContract.UpdateSignature(didCryptoLib)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	miningConsensusReq := c.getMiningConsensusReq(miningContract.GetBlock(), *MiningReq)

	_, _, _, err = c.initiateConsensus(miningConsensusReq, miningContract, didCryptoLib)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}

	// TODO: Add sending mining details to explorer
	resp.Status = true
	resp.Message = "Mining contract successfully initiated"
	return resp

}

func (c *Core) getMiningConsensusReq(contractBlock []byte, miningReq model.MiningRequest) *ConensusRequest {
	var consensusRequest *ConensusRequest = &ConensusRequest{
		Mode:          MiningMode,
		ReqID:         uuid.New().String(),
		ContractBlock: contractBlock,
		MiningInfo:    miningReq,
		Type:          miningReq.Type,
	}
	return consensusRequest
}

// TokensCanbeMinedFromCreditsInGivenLevel calculates how many whole tokens can be mined from the requested tokenCredits
// and returns the remaining credits.
func TokensCanbeMinedFromCreditsInGivenLevel(reqTokenCredits uint64, tokenLevel int) (uint64, uint64, error) {
	creditsPerToken := token.CreditsRequiredforLevel(tokenLevel)
	// Calculate whole tokens that can be mined
	tokensCanbeMined := reqTokenCredits / creditsPerToken
	remainingCredits := reqTokenCredits % creditsPerToken

	return tokensCanbeMined, remainingCredits, nil
}

// This function, For a given requested token credits, tokenLevel and tokenNumber it outputs number of tokens can be mined
func TokensCanbeMinedFromCredits(reqTokenCredits uint64, tokenLevel int, tokenNumber int) (map[int]uint64, uint64, error) {
	result := make(map[int]uint64)
	remainingCredits := reqTokenCredits

	// Base case: if we've exceeded max level or have no credits left
	if tokenLevel > 78 || remainingCredits == 0 {
		return result, remainingCredits, nil
	}

	// Get current level's requirements
	creditsPerToken, ok := token.CreditLevelMap[tokenLevel]
	if !ok {
		return nil, 0, fmt.Errorf("credit level %d not found in the credit level map", tokenLevel)
	}

	maxTokensForLevel, ok := token.TokenMap[tokenLevel]
	if !ok {
		return nil, 0, fmt.Errorf("token level %d not found in token level map", tokenLevel)
	}

	// Calculate how many we could potentially mine in this level
	availableTokens := maxTokensForLevel - tokenNumber
	if availableTokens <= 0 {
		// Move to next level if current level is full
		return TokensCanbeMinedFromCredits(remainingCredits, tokenLevel+1, 1)
	}

	// Calculate possible tokens to mine
	tokensCanBeMined := remainingCredits / creditsPerToken
	actualTokens := uint64(availableTokens)
	if tokensCanBeMined < actualTokens {
		actualTokens = tokensCanBeMined
	}

	if actualTokens > 0 {
		// Calculate used credits
		usedCredits := actualTokens * creditsPerToken
		remainingCredits -= usedCredits

		// Add to result
		result[tokenLevel] = actualTokens

		// Check if we filled this level
		newTokenNumber := tokenNumber + int(actualTokens)
		if newTokenNumber >= maxTokensForLevel {
			// Move to next level with remaining credits
			nextLevelResult, remaining, err := TokensCanbeMinedFromCredits(remainingCredits, tokenLevel+1, 1)
			if err != nil {
				return nil, 0, err
			}
			mergeResults(result, nextLevelResult)
			return result, remaining, nil
		}

		// Still capacity in current level, return remaining credits
		return result, remainingCredits, nil
	}

	// Not enough credits for this level, try next level
	nextLevelResult, remaining, err := TokensCanbeMinedFromCredits(remainingCredits, tokenLevel+1, 1)
	if err != nil {
		return nil, 0, err
	}
	mergeResults(result, nextLevelResult)
	return result, remaining, nil
}

func mergeResults(target, source map[int]uint64) {
	for level, count := range source {
		target[level] += count
	}
}

// From the given requested token credits,it outputs total number of tokens that can be mined, remaining
func TotalTokensCanBeMinedFromCredits(reqTokenCredits uint64) (uint64, uint64, error) {
	//Todo:Fetch Latest tokenLevel and token number from the Mining chain
	tokensCanBeMined, remainingCredits, err := TokensCanbeMinedFromCredits(reqTokenCredits, tokenLevel, tokenNumber)
	if err != nil {
		return 0, 0, err
	}
	var totalTokens uint64
	// Sum up the total tokens from all levels
	for _, numberOfTokens := range tokensCanBeMined {
		totalTokens += numberOfTokens

	}
	return totalTokens, remainingCredits, nil

}

func (c *Core) publishMiningdetails(miningRecord *model.MiningRecordPubSub) error {
	if c.ps != nil {
		err := c.ps.Publish(miningPubSub, miningRecord)
		if err != nil {
			c.log.Error("Failed to publish mining record message", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) miningSubscription() error {
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	return c.ps.SubscribeTopic(miningPubSub, c.miningCallback)
}

func (c *Core) miningCallback(peerID string, topic string, data []byte) {
	var miningData model.MiningRecordPubSub
	var miningRecord wallet.MiningRecord
	err := json.Unmarshal(data, &miningData)
	c.log.Debug("Mining record update")
	if err != nil {
		c.log.Error("Failed to parse mining callback data", "err", err)
		return
	}

	// Populate miningRecord from miningData
	miningRecord.MiningID = miningData.MiningID
	miningRecord.MinedTokenID = miningData.MinedTokenID
	miningRecord.TokenLevelAndTokenNumber = miningData.TokenLevelAndTokenNumber

	// Add mining record to the database or wallet
	err = c.w.AddMiningRecords(miningRecord)
	if err != nil {
		c.log.Error("Failed to add mining record to mining records table")
		return
	}

	// Iterate over PledgeHistory and store key-value pairs
	batch := new(leveldb.Batch)
	for _, pledge := range miningData.PledgeHistory {
		// Create the key by concatenating TransactionID and TransferTokenID
		key := pledge.TransactionID + "_" + pledge.TransferTokenID
		keyBytes := []byte(key)
		valueBytes := []byte(miningData.MiningID)
		batch.Put(keyBytes, valueBytes)
	}
	err = c.w.MiningRecordsChainStorage.DB.Write(batch, nil)
	if err != nil {
		c.log.Error("Failed to batch write key-value pairs to LevelDB", "err", err)
	}
}

func (c *Core) QueryMiningRecord(transactionID string, transferTokenID string) (string, error) {
	// Create the key by concatenating TransactionID and TransferTokenID
	key := transactionID + "_" + transferTokenID
	keyBytes := []byte(key)

	// Read the value from LevelDB
	valueBytes, err := c.w.MiningRecordsChainStorage.DB.Get(keyBytes, nil)
	if err != nil || valueBytes == nil {
		return "", fmt.Errorf("failed to query MiningRecords: %v", err)
	}

	// Convert value to string
	miningID := string(valueBytes)
	fmt.Println("Mining ID found for the key pairs is ", miningID)

	return miningID, nil
}
