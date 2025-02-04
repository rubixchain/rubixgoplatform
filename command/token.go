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

func (cmd *Command) ValidateToken() {
	if cmd.token == "" {
		cmd.log.Info("Token cannot be empty")
		fmt.Print("Enter Token : ")
		_, err := fmt.Scan(&cmd.token)
		if err != nil {
			cmd.log.Error("Failed to get tokenhash")
			return
		}
	}
	br, err := cmd.c.ValidateToken(cmd.token)
	if err != nil {
		cmd.log.Error("failed to validate token", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("failed to validate token %s", cmd.token, "msg", br.Message)
		return
	}
	cmd.log.Info("Token %s validated successfully ", cmd.token, "msg", br.Message)

}

func (cmd *Command) TokensSanityCheck() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter did : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get did")
			return
		}
	}
	br, err := cmd.c.TokensSanityCheck(cmd.did)
	if err != nil {
		cmd.log.Error("failed to verify tokens of given did", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("failed to verify tokens ", "msg ", br.Message, ", result ", br.Result)
		return
	}
	cmd.log.Info("Tokens verified successfully ", "msg ", br.Message, ", invalid tokens ", br.Result)
}
