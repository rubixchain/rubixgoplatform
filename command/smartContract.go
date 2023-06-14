package command

import (
	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
)

func (cmd *Command) generateSmartContractToken() {
	smartContractTokenRequest := core.GenerateSmartContractRequest{
		BinaryCode: cmd.binaryCodePath,
		RawCode:    cmd.rawCodePath,
		YamlCode:   cmd.yamlFilePath,
		DID:        cmd.did,
	}

	request := client.SmartContractRequest{
		BinaryCode: smartContractTokenRequest.BinaryCode,
		RawCode:    smartContractTokenRequest.RawCode,
		YamlCode:   smartContractTokenRequest.YamlCode,
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
