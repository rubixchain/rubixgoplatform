package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"

	// "github.com/rubixchain/rubixgoplatform/did"

	// "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	tokenLevel   = 004
	tokenNumber  = 1000034
	miningPubSub = "mining-service"
)

type CreditsDetailsMapValue struct {
	MiningID         string `json:"miningID"`
	RemainingCredits uint64 `json:"remainingCredits"`
}

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
	// var latestMined wallet.MiningRecord
	resp := &model.BasicResponse{
		Status: false,
	}

	// Sync the rubix_mining_chain -- from hardcodded peer, TODO: Sync from the peer who pinned the mining chain

	// MiningIDreader := bytes.NewReader([]byte(RubixMiningChainIDString))
	// RubixMiningChainID, err := c.ipfs.Add(MiningIDreader, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	// // Hardcode the peerID where mining chain can be synced
	// peerForMiningChainSync, err := c.getPeer(AddressForMiningChainSync, "")
	// if err != nil {
	// 	c.log.Error("Failed to get peer for syncing mining chain")
	// }
	// err = c.syncTokenChainFrom(peerForMiningChainSync, "", RubixMiningChainID, token.MiningChainType)

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
	// latestMined, err = c.w.FindLatestTokenLevelAndNumber()
	// if err != nil {
	// 	c.log.Error("Unable to find last mined token")
	// }
	//if last mining token number is maxtokennumber in a level, Then it need to go to next level  with 1 as token number
	// nextMiningTokenLevel := latestMined.TokenLevel
	// nextMiningTokenNumber := latestMined.TokenNumber + 1
	// maxTokenNumberfromLevel := token.MaxTokenFromLevel(nextMiningTokenLevel)
	// if maxTokenNumberfromLevel < nextMiningTokenNumber {
	// 	nextMiningTokenLevel += 1
	// 	nextMiningTokenNumber = 1
	// }
	// nextMiningTokenLevel := tokenLevel
	var nextMiningTokenDetails model.NewTokenDetails
	nextMiningTokenDetails.TokenLevel = tokenLevel
	nextMiningTokenDetails.TokenNumber = tokenNumber

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

	MiningIDreader := bytes.NewReader([]byte(RubixMiningChainIDString))
	RubixMiningChainID, err := c.ipfs.Add(MiningIDreader, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	if err != nil {
		c.log.Error("Failed to get Rubix mining chain ID")
	}
	minerAddress := miningData.MinerPeerID + "." + miningData.MinerDID
	minerPeer, err := c.getPeer(minerAddress)
	c.syncTokenChainFrom(minerPeer, "", RubixMiningChainID, token.MiningChainType)

	// Populate miningRecord from miningData
	miningRecord := wallet.MiningRecord{
		MiningID:     miningData.MiningID,
		MinedTokenID: miningData.MinedTokenID,
		MinerDID:     miningData.MinerDID,
		TokenLevel:   miningData.TokenLevel,
		TokenNumber:  miningData.TokenNumber,
	}
	fmt.Println("Mining record at the subscriber is ", miningRecord)
	// Add mining record to the database or wallet
	err = c.w.AddMiningRecords(miningRecord)
	if err != nil {
		c.log.Error("Failed to add mining record to mining records table")
		return
	}
	// batch := new(leveldb.Batch)
	// for _, pledge := range miningData.PledgeHistory {
	// 	key := pledge.TransactionID + "-" + pledge.TransferTokenID + "-" + miningRecord.MinerDID
	// 	keyBytes := []byte(key)
	// 	miningChainValue := MiningLevelDBValue{
	// 		MiningID:         miningData.MiningID,
	// 		RemainingCredits: pledge.RemainingCredits,
	// 	}

	// 	valueBytes, err := json.Marshal(miningChainValue)
	// 	if err != nil {
	// 		c.log.Error("Failed to marshal MiningLevelDBValue", "err", err)
	// 		continue
	// 	}
	// 	batch.Put(keyBytes, valueBytes)
	// }
	// err = c.w.MiningRecordsChainStorage.DB.Write(batch, nil)
	// if err != nil {
	// 	c.log.Error("Failed to batch write key-value pairs to LevelDB", "err", err)
	// }
}

// func (c *Core) QueryMiningRecord(transactionID string, transferTokenID string, minerDID string) (*MiningLevelDBValue, error) {
// 	// Create the composite key
// 	key := transactionID + "-" + transferTokenID + "-" + minerDID
// 	keyBytes := []byte(key)

// 	// Read from LevelDB
// 	valueBytes, err := c.w.MiningRecordsChainStorage.DB.Get(keyBytes, nil)
// 	if err != nil {
// 		if err == leveldb.ErrNotFound {
// 			return nil, fmt.Errorf("mining record not found")
// 		}
// 		return nil, fmt.Errorf("mining record read error: %v", err)
// 	}

// 	// Unmarshal the value
// 	var miningValue MiningLevelDBValue
// 	err = json.Unmarshal(valueBytes, &miningValue)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode mining record: %v", err)
// 	}

// 	c.log.Debug("Retrieved mining record",
// 		"miningID", miningValue.MiningID,
// 		"remainingCredits", miningValue.RemainingCredits)

// 	return &miningValue, nil
// }

func GetCreditsRequired(tokenLevel int) (uint64, error) {
	creditsPerToken, ok := token.CreditLevelMap[tokenLevel]
	if !ok {
		return 0, fmt.Errorf("credit level %d not found in the credit level map", tokenLevel)
	}
	return creditsPerToken, nil
}
