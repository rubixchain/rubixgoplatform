package command

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
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
