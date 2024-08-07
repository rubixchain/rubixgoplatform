package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) generateSmartContractToken() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get DID")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.binaryCodePath == "" {
		cmd.log.Error("Please provide Binary code file")
		return
	}
	if cmd.rawCodePath == "" {
		cmd.log.Error("Please provide Raw code file")
		return
	}
	if cmd.schemaFilePath == "" {
		cmd.log.Error("Please provide Schema file")
		return
	}
	smartContractTokenRequest := core.GenerateSmartContractRequest{
		BinaryCode: cmd.binaryCodePath,
		RawCode:    cmd.rawCodePath,
		SchemaCode: cmd.schemaFilePath,
		DID:        cmd.did,
	}

	request := client.SmartContractRequest{
		BinaryCode: smartContractTokenRequest.BinaryCode,
		RawCode:    smartContractTokenRequest.RawCode,
		SchemaCode: smartContractTokenRequest.SchemaCode,
		DID:        smartContractTokenRequest.DID,
	}

	basicResponse, err := cmd.c.GenerateSmartContractToken(&request)
	if err != nil {
		cmd.log.Error("Failed to generate smart contract token", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to generate smart contract token", "err", err)
		return
	}
	cmd.log.Info("Smart contract token generated successfully")

}

func (cmd *Command) fetchSmartContract() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract token id cannot be empty")
		fmt.Print("Enter SC Token Id : ")
		_, err := fmt.Scan(&cmd.smartContractToken)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)

	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}
	smartContractTokenRequest := core.FetchSmartContractRequest{
		SmartContractToken: cmd.smartContractToken,
	}

	request := client.FetchSmartContractRequest{
		SmartContractToken: smartContractTokenRequest.SmartContractToken,
	}

	basicResponse, err := cmd.c.FetchSmartContract(&request)
	if err != nil {
		cmd.log.Error("Failed to fetch smart contract token", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to fetch smart contract token", "err", err)
		return
	}
	cmd.log.Info("Smart contract token fetched successfully")
}
func (cmd *Command) PublishContract() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract token id cannot be empty")
		fmt.Print("Enter SC Token Id : ")
		_, err := fmt.Scan(&cmd.smartContractToken)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.publishType < 1 || cmd.publishType > 2 {
		cmd.log.Error("Invalid publish type")
		return
	}
	basicResponse, err := cmd.c.PublishNewEvent(cmd.smartContractToken, cmd.did, cmd.publishType, cmd.newContractBlock)

	if err != nil {
		cmd.log.Error("Failed to publish new event", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to publish new event", "msg", basicResponse.Message)
		return
	}
	message, status := cmd.SignatureResponse(basicResponse)

	if !status {
		cmd.log.Error("Failed to publish new event, " + message)
		return
	}
	cmd.log.Info("New event published successfully")
}
func (cmd *Command) SubscribeContract() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract token id cannot be empty")
		fmt.Print("Enter SC Token Id : ")
		_, err := fmt.Scan(&cmd.smartContractToken)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}

	basicResponse, err := cmd.c.SubscribeContract(cmd.smartContractToken)

	if err != nil {
		cmd.log.Error("Failed to subscribe contract", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to subscribe contract", "msg", basicResponse.Message)
		return
	}
	message, status := cmd.SignatureResponse(basicResponse)

	if !status {
		cmd.log.Error("Failed to subscribe contract, " + message)
		return
	}
	cmd.log.Info("New event subscribed successfully")
}

func (cmd *Command) deploySmartcontract() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract token id cannot be empty")
		fmt.Print("Enter SC Token Id : ")
		_, err := fmt.Scan(&cmd.smartContractToken)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.deployerAddr)
	if !strings.HasPrefix(cmd.deployerAddr, "bafybmi") || len(cmd.deployerAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid deployer DID")
		return
	}
	if cmd.rbtAmount < 0.001 {
		cmd.log.Error("Invalid RBT amount. Minimum RBT amount should be 0.001")
		return
	}
	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type")
		return
	}
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
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract token id cannot be empty")
		fmt.Print("Enter SC Token Id : ")
		_, err := fmt.Scan(&cmd.smartContractToken)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}

	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.executorAddr)
	if !strings.HasPrefix(cmd.executorAddr, "bafybmi") || len(cmd.executorAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid executer DID")
		return
	}
	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type")
		return
	}
	if cmd.smartContractData == "" {
		fmt.Print("Enter Data to be executed : ")
		_, err := fmt.Scan(&cmd.smartContractData)
		if err != nil {
			cmd.log.Error("Failed to get data")
			return
		}
	}
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
