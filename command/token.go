package command

import (
	"fmt"
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
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) < 59 {
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
