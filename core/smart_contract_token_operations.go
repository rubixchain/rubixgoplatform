package core

import (
	"fmt"
	"time"

	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) DeploySmartContractToken(reqID string, deployReq *model.DeploySmartContractRequest) {
	br := c.deploySmartContractToken(reqID, deployReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

/*
 * Input params : reqID String , deployReq model.DeploySmartContractRequest
 * Output : model.BasicResponse
 * This methods generates the req for smart contract deploying consensus which includes, tokens to be commited, pledge amount
 */
func (c *Core) deploySmartContractToken(reqID string, deployReq *model.DeploySmartContractRequest) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(deployReq.DeployerAddress)
	if !ok {
		resp.Message = "Invalid Deployer DID"
		return resp
	}
	didCryptoLib, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Deployer DID, " + err.Error()
		return resp
	}
	//check the smartcontract token from the DB base
	_, err = c.w.GetSmartContractToken(deployReq.SmartContractToken)
	if err != nil {
		c.log.Error("Failed to retrieve smart contract Token details from storage", err)
		resp.Message = err.Error()
		return resp
	}
	//Get the RBT details from DB for the associated amount/ if token amount is of PArts create
	rbtTokensToCommitDetails, err := c.GetTokens(didCryptoLib, did, deployReq.RBTAmount)
	if err != nil {
		c.log.Error("Failed to retrieve Tokens to be committed", "err", err)
		resp.Message = "Failed to retrieve Tokens to be committed , err : " + err.Error()
		return resp
	}

	rbtTokensToCommit := make([]string, 0)

	defer c.w.ReleaseTokens(rbtTokensToCommitDetails)

	for i := range rbtTokensToCommitDetails {
		c.w.Pin(rbtTokensToCommitDetails[i].TokenID, wallet.OwnerRole, did)
		rbtTokensToCommit = append(rbtTokensToCommit, rbtTokensToCommitDetails[i].TokenID)
	}

	rbtTokenInfoArray := make([]contract.TokenInfo, 0)
	smartContractInfoArray := make([]contract.TokenInfo, 0)
	for i := range rbtTokensToCommitDetails {
		tokenTypeString := "rbt"
		if rbtTokensToCommitDetails[i].TokenValue != 1 {
			tokenTypeString = "part"
		}
		tokenType := c.TokenType(tokenTypeString)
		latestBlk := c.w.GetLatestTokenBlock(rbtTokensToCommitDetails[i].TokenID, tokenType)
		if latestBlk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		blockId, err := latestBlk.GetBlockID(rbtTokensToCommitDetails[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		tokenInfo := contract.TokenInfo{
			Token:      rbtTokensToCommitDetails[i].TokenID,
			TokenType:  tokenType,
			TokenValue: rbtTokensToCommitDetails[i].TokenValue,
			OwnerDID:   rbtTokensToCommitDetails[i].DID,
			BlockID:    blockId,
		}
		rbtTokenInfoArray = append(rbtTokenInfoArray, tokenInfo)
	}

	smartContractInfo := contract.TokenInfo{
		Token:      deployReq.SmartContractToken,
		TokenType:  c.TokenType("sc"),
		TokenValue: deployReq.RBTAmount,
		OwnerDID:   did,
	}
	smartContractInfoArray = append(smartContractInfoArray, smartContractInfo)

	consensusContractDetails := &contract.ContractType{
		Type:       contract.SmartContractDeployType,
		PledgeMode: contract.POWPledgeMode,
		TotalRBTs:  deployReq.RBTAmount,
		TransInfo: &contract.TransInfo{
			DeployerDID:        did,
			Comment:            deployReq.Comment,
			CommitedTokens:     rbtTokenInfoArray,
			SmartContractToken: deployReq.SmartContractToken,
			TransTokens:        smartContractInfoArray,
		},
	}
	consensusContract := contract.CreateNewContract(consensusContractDetails)
	if consensusContract == nil {
		c.log.Error("Failed to create Consensus contract")
		resp.Message = "Failed to create Consensus contract"
		return resp
	}
	err = consensusContract.UpdateSignature(didCryptoLib)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	conensusRequest := &ConensusRequest{
		ReqID:              uuid.New().String(),
		Type:               deployReq.QuorumType,
		DeployerPeerID:     c.peerID,
		ContractBlock:      consensusContract.GetBlock(),
		SmartContractToken: deployReq.SmartContractToken,
		Mode:               SmartContractDeployMode,
	}

	txnDetails, _, err := c.initiateConsensus(conensusRequest, consensusContract, didCryptoLib)

	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	txnDetails.Amount = deployReq.RBTAmount
	txnDetails.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(txnDetails)
	tokens := make([]string, 0)
	tokens = append(tokens, deployReq.SmartContractToken)
	explorerTrans := &ExplorerTrans{
		TID:         txnDetails.TransactionID,
		DeployerDID: did,
		Amount:      deployReq.RBTAmount,
		TrasnType:   conensusRequest.Type,
		TokenIDs:    tokens,
		QuorumList:  conensusRequest.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
	}
	c.ec.ExplorerTransaction(explorerTrans)

	//Todo pubsub - publish smart contract token details
	c.log.Info("Smart Contract Token Deployed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("Smart Contract Token Deployed successfully in %v", dif)
	resp.Message = msg
	return resp
}

func (c *Core) executeSmartContractToken(smartcontractTokenHash string, function string, input []string) {

}
