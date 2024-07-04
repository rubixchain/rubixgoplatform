package command

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core"
)

func (cmd *Command) UpgradeTokensCmd() {
	fmt.Println("Upgrade tokens cmd triggered")
	if cmd.forcePWD {
		pwd, err := getpassword("Set private key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		npwd, err := getpassword("Re-enter private key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		if pwd != npwd {
			cmd.log.Error("Password mismatch")
			return
		}
		cmd.privPWD = pwd
	}
	if cmd.forcePWD {
		pwd, err := getpassword("Set quorum key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		npwd, err := getpassword("Re-enter quorum key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		if pwd != npwd {
			cmd.log.Error("Password mismatch")
			return
		}
		cmd.quorumPWD = pwd
	}

	r := core.UpgradeRequest{
		DIDType:   cmd.didType,
		PrivPWD:   cmd.privPWD,
		QuorumPWD: cmd.quorumPWD,
		DID:       cmd.did,
	}

	fmt.Println("Upgrade tokens cmd triggered with request: ", r)

	br, err := cmd.c.UpgradeTokensClient(&r, cmd.timeout)
	if err != nil {
		cmd.log.Error("Failed to upgrade node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to upgrade node", "msg", br.Message)
		return
	}
	cmd.log.Info("Node upgrade successfully, ")

}
