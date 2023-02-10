package command

import "github.com/rubixchain/rubixgoplatform/core"

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
