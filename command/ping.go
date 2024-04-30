package command

import (
	"fmt"
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
	fmt.Println("checkQuorumStatus triggered")
	msg, status := cmd.c.CheckQuorumStatus(cmd.quorumAddr)
	fmt.Println("cmd msg is ", msg)
	fmt.Println("cmd status is ", status)
	if strings.Contains(msg, "Quorum is setup") {
		cmd.log.Info("Quorum is setup in", cmd.quorumAddr, "message", msg)
	} else {
		cmd.log.Error("Quorum is not setup in ", cmd.quorumAddr, " message ", msg)
	}
}
