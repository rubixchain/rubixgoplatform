package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
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
		c.w.Pin(rbtTokensToCommitDetails[i].TokenID, wallet.OwnerRole, did, "NA", "NA", "NA", float64(0)) //TODO: Ensure whether trnxId should be added ?
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
		ReqID: reqID,
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

	consensusContractBlock := consensusContract.GetBlock()
	if consensusContractBlock == nil {
		c.log.Error("failed to create consensus contract block")
		resp.Message = "failed to create consensus contract block"
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
		//BlockHash:   txnDetails.BlockID,
	}
	c.ec.ExplorerTransaction(explorerTrans)

	c.log.Info("Smart Contract Token Deployed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("Smart Contract Token Deployed successfully in %v", dif)
	resp.Message = msg
	return resp
}

func (c *Core) ExecuteSmartContractToken(reqID string, executeReq *model.ExecuteSmartContractRequest) {
	br := c.executeSmartContractToken(reqID, executeReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) executeSmartContractToken(reqID string, executeReq *model.ExecuteSmartContractRequest) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}

	_, did, ok := util.ParseAddress(executeReq.ExecutorAddress)
	if !ok {
		resp.Message = "Invalid Executor DID"
		return resp
	}
	didCryptoLib, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Executor DID, " + err.Error()
		return resp
	}
	//check the smartcontract token from the DB base
	_, err = c.w.GetSmartContractToken(executeReq.SmartContractToken)
	if err != nil {
		c.log.Error("Failed to retrieve smart contract Token details from storage", err)
		resp.Message = err.Error()
		return resp
	}

	//get the gensys block of the amrt contract token
	tokenType := c.TokenType(SmartContractString)
	gensysBlock := c.w.GetGenesisTokenBlock(executeReq.SmartContractToken, tokenType)
	if gensysBlock == nil {
		c.log.Debug("Gensys block is empty - Smart contract Token chain not synced")
		resp.Message = "Gensys block is empty - Smart contract Token chain not synced"
		return resp
	}

	//fetch smartcontract value from the gensys block
	smartContractValue, err := gensysBlock.GetSmartContractValue(executeReq.SmartContractToken)
	if err != nil {
		c.log.Error("Failed to retrieve smart contract Token Value , ", err)
		resp.Message = err.Error()
		return resp
	}
	if smartContractValue == 0 {
		c.log.Error("smart contract Token Value cannot be 0, ")
		resp.Message = "smart contract Token Value cannot be 0, "
		return resp
	}

	smartContractInfoArray := make([]contract.TokenInfo, 0)
	smartContractInfo := contract.TokenInfo{
		Token:      executeReq.SmartContractToken,
		TokenType:  c.TokenType("sc"),
		TokenValue: smartContractValue,
		OwnerDID:   gensysBlock.GetDeployerDID(),
	}
	smartContractInfoArray = append(smartContractInfoArray, smartContractInfo)

	//create teh consensuscontract
	consensusContractDetails := &contract.ContractType{
		Type:       contract.SmartContractDeployType,
		PledgeMode: contract.POWPledgeMode,
		TotalRBTs:  smartContractValue,
		TransInfo: &contract.TransInfo{
			ExecutorDID:        did,
			Comment:            executeReq.Comment,
			SmartContractToken: executeReq.SmartContractToken,
			TransTokens:        smartContractInfoArray,
			SmartContractData:  executeReq.SmartContractData,
		},
		ReqID: reqID,
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

	consensusContractBlock := consensusContract.GetBlock()
	if consensusContractBlock == nil {
		c.log.Error("failed to create consensus contract block")
		resp.Message = "failed to create consensus contract block"
		return resp
	}
	conensusRequest := &ConensusRequest{
		ReqID:              uuid.New().String(),
		Type:               executeReq.QuorumType,
		ExecuterPeerID:     c.peerID,
		ContractBlock:      consensusContract.GetBlock(),
		SmartContractToken: executeReq.SmartContractToken,
		Mode:               SmartContractExecuteMode,
	}

	txnDetails, _, err := c.initiateConsensus(conensusRequest, consensusContract, didCryptoLib)

	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)

	txnDetails.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(txnDetails)
	tokens := make([]string, 0)
	tokens = append(tokens, executeReq.SmartContractToken)
	explorerTrans := &ExplorerTrans{
		TID:         txnDetails.TransactionID,
		ExecutorDID: did,
		TrasnType:   conensusRequest.Type,
		TokenIDs:    tokens,
		QuorumList:  conensusRequest.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
		//BlockHash:   txnDetails.BlockID,
	}
	c.ec.ExplorerTransaction(explorerTrans)
	/* newEvent := model.NewContractEvent{
		Contract:          executeReq.SmartContractToken,
		Did:               did,
		Type:              ExecuteType,
		ContractBlockHash: "",
	}

	err = c.publishNewEvent(&newEvent)
	if err != nil {
		c.log.Error("Failed to publish smart contract executed info")
	} */

	c.log.Info("Smart Contract Token Executed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("Smart Contract Token Executed successfully in %v", dif)
	resp.Message = msg
	return resp
}
