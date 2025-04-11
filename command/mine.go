package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) MineRBTs() {
	// Ensure logger is initialized
	if cmd.log == nil {
		fmt.Println("Error: Logger is not initialized")
		return
	}

	// Ensure Client instance is initialized
	if cmd.c == nil {
		cmd.log.Error("Core instance is not initialized")
		return
	}

	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter DID: ")
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

	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type. TransType should be 1 or 2")
		return
	}

	// Creating mining request
	miningReq := &model.MiningRequest{
		MinerDid: cmd.did,
		Type:     cmd.transType,
	}
	// Ensure MineRBTs method is valid
	br, err := cmd.c.MineRBTs(miningReq)
	if err != nil {
		cmd.log.Info("Cannot mine RBTs: ", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to Mine RBTs", "msg", msg)
		return
	}
	cmd.log.Info(msg)
}
