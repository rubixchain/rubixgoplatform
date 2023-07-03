package command

import "github.com/rubixchain/rubixgoplatform/core/model"

func (cmd *Command) deploySmartcontract() {
	deployRequest := model.DeploySmartContractRequest{
		SmartContractToken: cmd.token,
		DeployerAddress:    cmd.deployerAddr,
		RBTAmount:          cmd.rbtAmount,
		QuorumType:         cmd.transType,
		Comment:            cmd.transComment,
	}
	response, err := cmd.c.DeploySmartContract(&deployRequest)
	if err != nil {
		cmd.log.Error("Failed to deploy Smart contract, Token ", cmd.token, "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(response)
	if !status {
		cmd.log.Error("Failed to deploy Smart contract, Token ", cmd.token, "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("Smart Contract Deployed successfully")
}
