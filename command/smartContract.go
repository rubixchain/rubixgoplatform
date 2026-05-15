package command

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/types/models"
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
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.binaryCodePath == "" {
		cmd.log.Error("Please provide Binary code file")
		return
	}
	// Preflight check: binary file must be .wasm
	if strings.ToLower(filepath.Ext(cmd.binaryCodePath)) != ".wasm" {
		cmd.log.Error("Binary code file must be a .wasm file")
		return
	}
	if cmd.rawCodePath == "" {
		cmd.log.Error("Please provide Raw code file")
		return
	}
	// Preflight check: raw code file must be .rs
	if strings.ToLower(filepath.Ext(cmd.rawCodePath)) != ".rs" {
		cmd.log.Error("Raw code file must be a .rs file")
		return
	}
	smartContractTokenRequest := core.GenerateSmartContractRequest{
		BinaryCode: cmd.binaryCodePath,
		RawCode:    cmd.rawCodePath,
		DID:        cmd.did,
	}

	request := client.SmartContractRequest{
		BinaryCode: smartContractTokenRequest.BinaryCode,
		RawCode:    smartContractTokenRequest.RawCode,
		DID:        smartContractTokenRequest.DID,
	}

	basicResponse, err := cmd.c.GenerateSmartContractToken(&request)
	if err != nil {
		cmd.log.Error("Failed to generate smart contract token", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to generate smart contract token", "err", basicResponse.Message)
		return
	}
	cmd.log.Info(fmt.Sprintf("Smart contract token %v generated successfully", basicResponse.Result))

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
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)

	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !isAlphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}

	basicResponse, err := cmd.c.FetchSmartContract(cmd.smartContractToken)
	if err != nil {
		cmd.log.Error("Failed to fetch smart contract token", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to fetch smart contract token", "err", basicResponse.Message)
		return
	}
	cmd.log.Info("Smart contract token fetched successfully")
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
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !isAlphanumeric {
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

func (cmd *Command) DeploySmartContract() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract id cannot be empty")
		fmt.Print("Enter smart contract Id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get smart contract")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract")
		return
	}
	if cmd.deployerAddr == "" {
		cmd.log.Info("Deployer address cannot be empty")
		fmt.Print("Enter Deployer DID : ")
		_, err := fmt.Scan(&cmd.deployerAddr)
		if err != nil {
			cmd.log.Error("Failed to get Deployer DID")
			return
		}
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.deployerAddr)
	if !strings.HasPrefix(cmd.deployerAddr, "bafybmi") || len(cmd.deployerAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid deployer DID")
		return
	}

	smartContractInfo := models.SmartContractInfo{
		SmartContractId: cmd.smartContractToken,
		Value:           cmd.smartContractValue,
		Data:            cmd.smartContractData,
	}
	sctDeployRequest := models.TransactionRequest{
		Initiator: cmd.deployerAddr,
		Tokens: models.TransactionTokenDetails{
			SmartContract: []models.SmartContractInfo{smartContractInfo},
		},
		Memo: cmd.transComment,
	}

	br, err := cmd.c.InitiateTransaction(&sctDeployRequest)
	if err != nil {
		cmd.log.Error("Failed smart contract deployment", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to deploy smart contract", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("smart contract deployed successfully")
}

func (cmd *Command) ExecuteSmartContract() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract id cannot be empty")
		fmt.Print("Enter smart contract Id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get smart contract")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)
	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid smart contract")
		return
	}
	if cmd.executorAddr == "" {
		cmd.log.Info("Executor address cannot be empty")
		fmt.Print("Enter Executor DID : ")
		_, err := fmt.Scan(&cmd.executorAddr)
		if err != nil {
			cmd.log.Error("Failed to get Executor DID")
			return
		}
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.executorAddr)
	if !strings.HasPrefix(cmd.executorAddr, "bafybmi") || len(cmd.executorAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid Executor DID")
		return
	}

	smartContractInfo := models.SmartContractInfo{
		SmartContractId: cmd.smartContractToken,
		Data:            cmd.smartContractData,
	}
	sctExecutorRequest := models.TransactionRequest{
		Initiator: cmd.executorAddr,
		Tokens: models.TransactionTokenDetails{
			SmartContract: []models.SmartContractInfo{smartContractInfo},
		},
		Memo: cmd.transComment,
	}

	br, err := cmd.c.InitiateTransaction(&sctExecutorRequest)
	if err != nil {
		cmd.log.Error("Failed smart contract execution", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to execute smart contract", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("smart contract executed successfully")
}
