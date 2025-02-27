package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"

	// "github.com/rubixchain/rubixgoplatform/did"

	// "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) InitiateMineRBTs(reqID string, req *model.MiningRequest, tokenCreditDetails []model.PledgeHistory) *model.BasicResponse {
	fmt.Println("Executing MineRBTs function")

	resp := &model.BasicResponse{
		Status: false,
	}

	// 1. Fetch pledge history records where tokenCreditStatus = 1 (ready to mine)
	pledges, err := c.w.GetTokenDetailsByQuorumDID(req.MinerDid, 1)
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

	didCryptoLib, err := c.SetupDID(reqID, req.MinerDid)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	//3.take all those token details and send it to mining quorum(to do that we might need an internal API)
	MiningContractDetails := &contract.ContractType{
		Type:               contract.MineRBTType,
		PledgeMode:         contract.PeriodicPledgeMode,
		ReqTokenCredits:    req.TokenCredits,
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
	// miningConsensusReq := c.getMiningConsensusReq(req.MinerDid, miningContract.GetBlock(), *req)

	// //for now manually hardcoding the miningQuorumlist
	// var miningQuorumlist []string
	// miningQuorumlist = []string{"sai1", "sai2", "sai3", "sai4", "sai5"}

	//4.Refer Initiate RBT flow for collecting and sending the token details.
	resp.Status = true
	resp.Message = "Mining contract successfully initiated"
	resp.Result = miningContract

	return resp

}

func (c *Core) getMiningConsensusReq(minerPeerID string, contractBlock []byte, miningReq model.MiningRequest) *ConensusRequest {
	var consensusRequest *ConensusRequest = &ConensusRequest{
		ReqID:         uuid.New().String(),
		SenderPeerID:  miningReq.MinerDid,
		ContractBlock: contractBlock,
		MiningInfo:    miningReq,
	}
	return consensusRequest
}

// func (c *Core) initiateMiningConsensus(miningContractDetails *contract.ContractType, consensusReq *ConsensusRequest, didCrypto did.DIDCrypto) {
// 	//get pledge amount depending on the credit details,
// 	//fetch requird number of credits to mine next RBT
// 	//for a given number of credits, we should check next mining token level, token number,
// 	//for that particular level, how many tokens still we are yet to mine.
// 	//get how many number of tokens we can mine of this particular level with the given number of credits.
// 	//If that number is >number of tokens yet to mine in current level
//     //Inputs: number of total req token credits
// 	//

//     consensusReq.MiningInfo.TokenCredits
// 	//

// }
