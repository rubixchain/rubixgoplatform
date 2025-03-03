package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/token"

	// "github.com/rubixchain/rubixgoplatform/did"

	// "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) InitiateMineRBTs(reqID string, MiningReq *model.MiningRequest, tokenCreditDetails []model.PledgeHistory) *model.BasicResponse {
	fmt.Println("Executing MineRBTs function")

	resp := &model.BasicResponse{
		Status: false,
	}

	// 1. Fetch pledge history records where tokenCreditStatus = 1 (ready to mine)
	pledges, err := c.w.GetTokenDetailsByQuorumDID(MiningReq.MinerDid, 1)
	if err != nil {
		resp.Message = "Failed to fetch pledge history, " + err.Error()
		return resp // Return error if fetching fails
	}

	// 2. Calculate total token credit
	totalCredits := 0
	for _, pledge := range pledges {
		totalCredits += pledge.TokenCredit
	}

	// 3. Check how many credits are needed for mining the next token
	creditsForNextToken := 100 // Fetch from mining chain if dynamic

	// 4. Validate if total credits are sufficient
	if totalCredits < creditsForNextToken {
		resp.Message = fmt.Sprintf("Total credits (%d) are less than the required credits (%d) to mine the next token", totalCredits, creditsForNextToken)
		return resp // Return an error
	}

	didCryptoLib, err := c.SetupDID(reqID, MiningReq.MinerDid)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	//3.take all those token details and send it to mining quorum(to do that we might need an internal API)
	MiningContractDetails := &contract.ContractType{
		Type:               contract.MineRBTType,
		PledgeMode:         contract.PeriodicPledgeMode,
		ReqTokenCredits:    MiningReq.TokenCredits,
		TokenCreditDetails: tokenCreditDetails,
		ReqID:              reqID,
	}
	miningContract := contract.CreateNewContract(MiningContractDetails)

	err = miningContract.UpdateSignature(didCryptoLib)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	miningConsensusReq := c.getMiningConsensusReq(miningContract.GetBlock(), *MiningReq)

	_, _, _, _ = c.initiateConsensus(miningConsensusReq, miningContract, didCryptoLib)

	//4.Refer Initiate RBT flow for collecting and sending the token details.
	resp.Status = true
	resp.Message = "Mining contract successfully initiated"
	resp.Result = miningContract

	return resp

}

func (c *Core) getMiningConsensusReq(contractBlock []byte, miningReq model.MiningRequest) *ConensusRequest {
	var consensusRequest *ConensusRequest = &ConensusRequest{
		Mode:          MiningMode,
		ReqID:         uuid.New().String(),
		ContractBlock: contractBlock,
		MiningInfo:    miningReq,
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

//////////
