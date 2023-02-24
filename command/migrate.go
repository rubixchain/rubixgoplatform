package command

import (
	"encoding/json"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/core"
)

func (cmd *Command) MigrateNodeCmd() {
	r := core.MigrateRequest{
		DIDType:   cmd.didType,
		PrivPWD:   cmd.privPWD,
		QuorumPWD: cmd.quorumPWD,
	}
	br, err := cmd.c.MigrateNode(&r)
	if err != nil {
		cmd.log.Error("Failed to migrate node", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to migrate node", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)
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
