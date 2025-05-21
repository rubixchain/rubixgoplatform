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
	// tokenLevel   = 1
	miningPubSub = "mining-service"
	// tokenFile    = `C:\Users\psy_k\OneDrive\Desktop\RubixGo\vaishnav\rubixgoplatformBKP\token.txt`
)

// type CreditsDetailsMapValue struct {
// 	MiningID         string `json:"miningID"`
// 	RemainingCredits uint64 `json:"remainingCredits"`
// }

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
	st := time.Now()
	txEpoch := int(st.Unix())
	resp := &model.BasicResponse{
		Status: false,
	}

	// Sync the rubix_mining_chain
	err := c.SyncMiningChain(AddressForMiningChainSync)
	if err != nil {
		c.log.Error("Failed to get peer for syncing mining chain")
		resp.Message = "Failed to sync mining chain"
		return resp
	}

	// Fetch pledge history records
	tokenDetails, err := c.w.GetTokenDetailsByQuorumDID(MiningReq.MinerDid, 1)
	if err != nil {
		resp.Message = "Failed to fetch pledge history, " + err.Error()
		return resp
	}

	// Calculate total token credit
	var totalCredits uint64
	for _, tokenDetail := range tokenDetails {
		totalCredits += tokenDetail.TokenCredit
	}

	// Get latest mining chain block
	latestMined, err := c.w.GetLatestMiningChainBlock()
	if err != nil || latestMined == nil {
		c.log.Error("Failed to get valid latest mining chain block", "error", err)
		resp.Message = "Failed to get latest mining chain block"
		return resp
	}

	// Get token level
	latestMiningTokenLevel, err := latestMined.GetTokenLevel()
	if err != nil || latestMiningTokenLevel == 0 {
		c.log.Error("Failed to get token level from latest mining chain block", "error", err)
		resp.Message = "Failed to get token level from latest mining chain block"
		return resp
	}

	// Get token number
	latestMiningTokenNumber, err := latestMined.GetTokenNumber()
	if err != nil || latestMiningTokenNumber == 0 {
		c.log.Error("Failed to get token number from latest mining chain block", "error", err)
		resp.Message = "Failed to get token number from latest mining chain block"
		return resp
	}

	fmt.Println("latestMiningTokenLevel:", latestMiningTokenLevel)
	fmt.Println("latestMiningTokenNumber:", latestMiningTokenNumber)

	// Determine next token level and number
	var nextMiningTokenLevel int
	var nextMiningTokenNumber uint64
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
	minerAddressForSync := fmt.Sprintf("%s.%s", miningData.MinerPeerID, miningData.MinerDID)
	fmt.Println(minerAddressForSync)
	err = c.SyncMiningChain(minerAddressForSync)
	if err != nil {
		c.log.Error("Failed to get sync mining chain", "err:", err)
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

func (c *Core) QueryMiningRecord(transactionID string, transferTokenID string, minerDID string) (uint64, bool, error) {
	// Create the composite key
	key := transactionID + "-" + transferTokenID + "-" + minerDID
	// Fetch all blocks starting from block 1
	blocks, _, err := c.w.GetAllMiningChainBlocks(block.GetMiningChainID(), 1)
	if err != nil || len(blocks) == 0 {
		return 0, false, fmt.Errorf("failed to get mining chain blocks: %v", err)
	}
	// Iterate through each block
	for i, blockBytes := range blocks {
		// Initialize MiningChain with CBOR bytes
		miningBlock := block.InitMiningBlock(blockBytes, nil)
		if miningBlock == nil {
			return 0, false, fmt.Errorf("failed to initialize mining block at index %d", i)
		}
		// Access CreditDetails map using GetCreditDetails
		creditDetails, err := miningBlock.GetCreditDetails()
		if err != nil || creditDetails == nil {
			return 0, false, fmt.Errorf("failed to get credit details at block for credit validation %d: %v", i, err)
		}
		// Check if the composite key exists
		if value, exists := creditDetails[key]; exists {
			// Convert interface{} to uint64
			remainingCredits, ok := value.(uint64)
			if !ok {
				// Handle possible float64 from CBOR deserialization
				if f, ok := value.(float64); ok && f == float64(uint64(f)) {
					remainingCredits = uint64(f)
				}
			}
			return remainingCredits, true, nil
		}
	}

	// Key not found in any block
	c.log.Debug("QueryMiningRecord: Key not found in any block", "key", key)
	return 0, false, nil
}

func GetCreditsRequired(tokenLevel int) (uint64, error) {
	creditsPerToken, ok := token.CreditLevelMap[tokenLevel]
	if !ok {
		return 0, fmt.Errorf("credit level %d not found in the credit level map", tokenLevel)
	}
	return creditsPerToken, nil
}
