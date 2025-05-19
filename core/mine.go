package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"

	// "github.com/rubixchain/rubixgoplatform/did"

	// "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	// tokenLevel   = 004
	miningPubSub = "mining-service"
	// tokenFile    = `C:\Users\psy_k\OneDrive\Desktop\RubixGo\vaishnav\rubixgoplatformBKP\token.txt`
)

type CreditsDetailsMapValue struct {
	MiningID         string `json:"miningID"`
	RemainingCredits uint64 `json:"remainingCredits"`
}

// func (c *Core) readTokenNumber() int {
// 	data, err := os.ReadFile(tokenFile)
// 	if err != nil {
// 		c.log.Error("Failed to read token file", "error", err)
// 		return 1500055 // Fallback default
// 	}
// 	tokenStr := string(bytes.TrimSpace(data))
// 	tokenNumber, err := strconv.Atoi(tokenStr)
// 	if err != nil || tokenNumber <= 0 {
// 		c.log.Error("Invalid token number", "error", err)
// 		return 1500055 // Fallback default
// 	}
// 	return tokenNumber
// }

func (c *Core) InitiateMineRBT(reqID string, req *model.MiningRequest) {
	br := c.initiateMineRBT(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}
func (c *Core) initiateMineRBT(reqID string, MiningReq *model.MiningRequest) *model.BasicResponse {
	// tokenNumber := uint64(c.readTokenNumber())
	st := time.Now()
	txEpoch := int(st.Unix())
	// var latestMined wallet.MiningRecord
	resp := &model.BasicResponse{
		Status: false,
	}

	// Sync the rubix_mining_chain

	// // Hardcode the peerID where mining chain can be synced
	err := c.SyncMiningChain(AddressForMiningChainSync)
	if err != nil {
		c.log.Error("Failed to get peer for syncing mining chain")
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

	// 3. Check how many credits are needed for mining the next token according to token level
	latestMined, err := c.w.GetLatestMiningChainBlock(block.GetMiningChainID())
	if err != nil {
		c.log.Error("Failed to get latest mining chain block")
	}
	// if last mining token number is maxtokennumber in a level, Then it need to go to next level  with 1 as token number
	latestMiningTokenLevel, err := latestMined.GetTokenLevel()
	if err != nil {
		c.log.Error("Failed to get token level from latest mining chain block")
	}
	latestMiningTokenNumber, err := latestMined.GetTokenNumber()
	if err != nil {
		c.log.Error("Failed to get token number from latest mining chain block")
	}
	var nextMiningTokenLevel int
	var nextMiningTokenNumber int
	maxTokenNumberfromLevel := token.MaxTokenFromLevel(latestMiningTokenLevel)
	if maxTokenNumberfromLevel == latestMiningTokenNumber {
		nextMiningTokenLevel = latestMiningTokenLevel + 1
		nextMiningTokenNumber = 1
	} else {
		nextMiningTokenLevel = latestMiningTokenLevel
		nextMiningTokenNumber = latestMiningTokenNumber + 1
	}
	var nextMiningTokenDetails model.NewTokenDetails
	nextMiningTokenDetails.TokenLevel = nextMiningTokenLevel
	nextMiningTokenDetails.TokenNumber = uint64(nextMiningTokenNumber)

	MiningReq.MiningTokenDetails = nextMiningTokenDetails

	//Need to add a function to get the credits required for the next token
	// creditsRequired := uint64(500)
	creditsRequired, err := GetCreditsRequired(nextMiningTokenDetails.TokenLevel)
	if err != nil {
		c.log.Error("Unable to get the credits required for the next token")
	}
	// creditsRequired := uint64(100) //Right now I am manually adding as 100 but in actual senario we should fetch using the above function.

	// 4. Validate if total credits are sufficient
	if totalCredits < creditsRequired {
		resp.Message = fmt.Sprintf("Total credits (%d) are less than the required credits (%d) to mine the next token", totalCredits, creditsRequired)
		return resp
	}

	// TODO: Handle the used credits: Update the used credit's creditStatus to 2
	usedCredits, _, _ := wallet.CollectRequiredCredits(tokenDetails, creditsRequired)

	// Add remaining credits in the credit details/ pledge history details
	didCryptoLib, err := c.SetupDID(reqID, MiningReq.MinerDid)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	//3.take all those token details and send it to mining quorum(to do that we might need an internal API)
	MiningContractDetails := &contract.ContractType{
		Type:               contract.MineRBTType,
		PledgeMode:         contract.PeriodicPledgeMode,
		ReqTokenCredits:    creditsRequired,
		TokenCreditDetails: usedCredits,
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

	miningConsensusReq := c.getMiningConsensusReq(miningContract.GetBlock(), *MiningReq, txEpoch)

	_, _, _, err = c.initiateConsensus(miningConsensusReq, miningContract, didCryptoLib)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}

	// TODO: Update used credits in the pledge history table after mining is completed
	updateErr := c.w.UpdateCredits(usedCredits)
	if updateErr != nil {
		c.log.Error("Unable to update used credits", "err", updateErr)
	}

	// TODO: Add sending mining details to explorer
	c.log.Info("Mining successfully completed. Mined TokenID:", miningConsensusReq.MiningTokenID)
	c.log.Info("Mining ID: %s", miningConsensusReq.TransactionID)
	resp.Status = true
	resp.Message = "Mining successfully completed"
	return resp

}

func (c *Core) getMiningConsensusReq(contractBlock []byte, miningReq model.MiningRequest, txnEpoch int) *ConensusRequest {
	var consensusRequest *ConensusRequest = &ConensusRequest{
		Mode:             MiningMode,
		ReqID:            uuid.New().String(),
		ContractBlock:    contractBlock,
		MiningInfo:       miningReq,
		Type:             miningReq.Type,
		TransactionEpoch: txnEpoch,
	}
	return consensusRequest
}

// TokensCanbeMinedFromCreditsInGivenLevel calculates how many whole tokens can be mined from the requested tokenCredits
// and returns the remaining credits.
// func TokensCanbeMinedFromCreditsInGivenLevel(reqTokenCredits uint64, tokenLevel int) (uint64, uint64, error) {
// 	creditsPerToken := token.CreditsRequiredforLevel(tokenLevel)
// 	// Calculate whole tokens that can be mined
// 	tokensCanbeMined := reqTokenCredits / creditsPerToken
// 	remainingCredits := reqTokenCredits % creditsPerToken

// 	return tokensCanbeMined, remainingCredits, nil
// }

// This function, For a given requested token credits, tokenLevel and tokenNumber it outputs number of tokens can be mined
// func TokensCanbeMinedFromCredits(reqTokenCredits uint64, tokenLevel int, tokenNumber int) (map[int]uint64, uint64, error) {
// 	result := make(map[int]uint64)
// 	remainingCredits := reqTokenCredits

// 	// Base case: if we've exceeded max level or have no credits left
// 	if tokenLevel > 78 || remainingCredits == 0 {
// 		return result, remainingCredits, nil
// 	}

// 	// Get current level's requirements
// 	creditsPerToken, ok := token.CreditLevelMap[tokenLevel]
// 	if !ok {
// 		return nil, 0, fmt.Errorf("credit level %d not found in the credit level map", tokenLevel)
// 	}

// 	maxTokensForLevel, ok := token.TokenMap[tokenLevel]
// 	if !ok {
// 		return nil, 0, fmt.Errorf("token level %d not found in token level map", tokenLevel)
// 	}

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
	err := json.Unmarshal(data, &miningData)
	c.log.Debug("Mining record update")
	if err != nil {
		c.log.Error("Failed to parse mining callback data", "err", err)
		return
	}
	err = c.SyncMiningChain(miningData.MinerPeerID)
	if err != nil {
		c.log.Error("Failed to get Rubix mining chain ID")
	}
	// Populate miningRecord from miningData
	miningRecord := wallet.MiningRecord{
		MiningID:     miningData.MiningID,
		MinedTokenID: miningData.MinedTokenID,
		MinerDID:     miningData.MinerDID,
		TokenLevel:   miningData.TokenLevel,
		TokenNumber:  miningData.TokenNumber,
		Epoch:        miningData.Epoch,
	}
	fmt.Println("Mining record at the subscriber is ", miningRecord)
	// Add mining record to the database or wallet
	err = c.w.AddMiningRecords(miningRecord)
	if err != nil {
		c.log.Error("Failed to add mining record to mining records table")
		return
	}
}

func (c *Core) QueryMiningRecord(transactionID string, transferTokenID string, minerDID string) (*CreditsDetailsMapValue, error) {
	// Create the composite key
	key := transactionID + "-" + transferTokenID + "-" + minerDID

	// Get the MiningChainID (assuming a function or constant exists to fetch it)
	miningChainID := block.GetMiningChainID() // Adjust based on your actual implementation

	// Fetch all blocks starting from block 1
	blocks, _, err := c.w.GetAllMiningChainBlocks(miningChainID, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get mining chain blocks: %v", err)
	}

	// If no blocks exist, return nil (not found)
	if len(blocks) == 0 {
		return nil, nil
	}

	// Iterate through each block
	for _, blockBytes := range blocks {
		// Deserialize the block (assuming MiningChainBlock has a method to deserialize)
		var miningBlock block.MiningChain
		err := json.Unmarshal(blockBytes, &miningBlock)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize mining chain block: %v", err)
		}

		// Get MiningChainBlockInfo (adjust method name based on actual struct)
		infos, err := miningBlock.GetMiningInfos()
		if err != nil {
			return nil, fmt.Errorf("failed to get mining infos: %v", err)
		}

		// Access CreditDetails map
		creditDetails, ok := infos[block.MICreditDetailsKey].(map[string]interface{})
		if !ok {
			continue // Skip if no CreditDetails in this block
		}

		// Check if the composite key exists
		if value, exists := creditDetails[key]; exists {
			// Convert interface{} to CreditsDetailsMapValue
			valueBytes, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal credit detail value: %v", err)
			}

			var miningValue CreditsDetailsMapValue
			err = json.Unmarshal(valueBytes, &miningValue)
			if err != nil {
				return nil, fmt.Errorf("failed to decode mining record: %v", err)
			}

			c.log.Debug("Retrieved mining record",
				"miningID", miningValue.MiningID,
				"remainingCredits", miningValue.RemainingCredits)

			return &miningValue, nil
		}
	}

	// Key not found in any block
	return nil, nil
}

func GetCreditsRequired(tokenLevel int) (uint64, error) {
	creditsPerToken, ok := token.CreditLevelMap[tokenLevel]
	if !ok {
		return 0, fmt.Errorf("credit level %d not found in the credit level map", tokenLevel)
	}
	return creditsPerToken, nil
}
