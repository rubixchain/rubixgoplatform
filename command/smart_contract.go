package command

import "github.com/rubixchain/rubixgoplatform/core/model"

func (cmd *Command) deploySmartcontract() {
	deployRequest := model.DeploySmartContractRequest{
		SmartContractToken: cmd.smartContractToken,
		DeployerAddress:    cmd.deployerAddr,
		RBTAmount:          cmd.rbtAmount,
		QuorumType:         cmd.transType,
		Comment:            cmd.transComment,
	}
	response, err := cmd.c.DeploySmartContract(&deployRequest)
	if err != nil {
		cmd.log.Error("Failed to deploy Smart contract, Token ", cmd.smartContractToken, "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(response)
	if !status {
		cmd.log.Error("Failed to deploy Smart contract, Token ", cmd.smartContractToken, "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("Smart Contract Deployed successfully")
}

func (cmd *Command) executeSmartcontract() {
	executorRequest := model.ExecuteSmartContractRequest{
		SmartContractToken: cmd.smartContractToken,
		ExecutorAddress:    cmd.executorAddr,
		QuorumType:         cmd.transType,
		Comment:            cmd.transComment,
		SmartContractData:  cmd.smartContractData,
	}
	response, err := cmd.c.ExecuteSmartContract(&executorRequest)
	if err != nil {
		cmd.log.Error("Failed to execute Smart contract, Token ", cmd.smartContractToken, "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(response)
	if !status {
		cmd.log.Error("Failed to execute Smart contract, Token ", cmd.smartContractToken, "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("Smart Contract executed successfully")
}
