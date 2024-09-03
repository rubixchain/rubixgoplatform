package command

import (
	"fmt"
	"regexp"
	"strings"
)

func (cmd *Command) GenerateTestRBT() {
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
	if cmd.numTokens <= 0 {
		cmd.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return
	}

	br, err := cmd.c.GenerateTestRBT(cmd.numTokens, cmd.did)

	if err != nil {
		cmd.log.Error("Failed to generate RBT", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to generate RBT", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to generate test RBT, " + msg)
		return
	}
	cmd.log.Info("Test RBT generated successfully")
}

func (cmd *Command) ValidateTokenchain() {
	if cmd.did == "" {
		cmd.log.Info("Tokenchain-validator did cannot be empty")
		fmt.Print("Enter tokenchain-validator DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get tokenchain-validator DID")
			return
		}
	}
	br, err := cmd.c.ValidateTokenchain(cmd.did, cmd.smartContractChainValidation, cmd.token, cmd.blockCount)
	if err != nil {
		cmd.log.Error("failed to validate token chain", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("failed to validate token chain", "msg", br.Message)
		return
	}

	cmd.log.Info("Tokenchain validated successfully", "msg", br.Message)
}
