package command

import (
	"fmt"
	"strings"
)

func (cmd *Command) ping() {
	if cmd.peerID == "" {
		cmd.log.Error("PeerID cannot be empty. Please use flag peerId")
		return
	}
	if !strings.HasPrefix(cmd.peerID, "12D3KooW") || len(cmd.peerID) < 52 {
		cmd.log.Error("Invalid PeerID")
		return
	}
	msg, status := cmd.c.Ping(cmd.peerID)
	if !status {
		cmd.log.Error("Ping failed", "message", msg)
	} else {
		cmd.log.Info("Ping response received successfully", "message", msg)
	}
}

func (cmd *Command) checkQuorumStatus() {
	if cmd.quorumAddr == "" {
		cmd.log.Info("Quorum Address cannot be empty")
		fmt.Print("Enter Quorum Address : ")
		_, err := fmt.Scan(&cmd.quorumAddr)
		if err != nil {
			cmd.log.Error("Failed to get Quorum Address")
			return
		}
	}
	msg, _ := cmd.c.CheckQuorumStatus(cmd.quorumAddr)
	//Verification with "status" pending !
	if strings.Contains(msg, "Quorum is setup") {
		cmd.log.Info("Quorum is setup in", cmd.quorumAddr, "message", msg)
	} else {
		cmd.log.Error("Quorum is not setup in ", cmd.quorumAddr, " message ", msg)
	}
}
