package command

import (
	"encoding/json"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/core"
)

func (cmd *Command) MigrateNodeCmd() {
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
	r := core.MigrateRequest{
		DIDType:   cmd.didType,
		PrivPWD:   cmd.privPWD,
		QuorumPWD: cmd.quorumPWD,
	}
	br, err := cmd.c.MigrateNode(&r, cmd.timeout)
	if err != nil {
		cmd.log.Error("Failed to migrate node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to migrate node", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br, cmd.timeout)
	if !status {
		cmd.log.Error("Failed to migrate node, " + msg)
		return
	}
	cmd.log.Info("Node migrated successfully, " + msg)
}

func (cmd *Command) LockedTokensCmd() {
	fb, err := ioutil.ReadFile(cmd.tokenList)
	if err != nil {
		cmd.log.Error("Failed to read token list", "err", err)
		return
	}
	var ts []string
	err = json.Unmarshal(fb, &ts)
	if err != nil {
		cmd.log.Error("Invalid token list", "err", err)
		return
	}
	br, err := cmd.c.LockToknes(ts)
	if err != nil {
		cmd.log.Error("Failed to lock tokens", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to lock tokens", "msg", br.Message)
		return
	}
	cmd.log.Info("Tokens lokced sucessfully")
}
