package command

import (
	"strings"
)

func (cmd *Command) ping() {
	msg, status := cmd.c.Ping(cmd.peerID)
	if !status {
		cmd.log.Error("Ping failed", "message", msg)
	} else {
		cmd.log.Info("Ping response received successfully", "message", msg)
	}
}

func (cmd *Command) checkQuorumStatus() {
	msg, _ := cmd.c.CheckQuorumStatus(cmd.quorumAddr)
	//Verification with "status" pending !
	if strings.Contains(msg, "Quorum is setup") {
		cmd.log.Info("Quorum is setup in", cmd.quorumAddr, "message", msg)
	} else {
		cmd.log.Error("Quorum is not setup in ", cmd.quorumAddr, " message ", msg)
	}
}
