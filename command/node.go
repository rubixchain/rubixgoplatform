package command

import (
	"fmt"
	"os"
)

func (cmd *Command) ShutDownCmd() {
	msg, status := cmd.c.Shutdown()
	if !status {
		cmd.log.Error("Failed to shutdown", "msg", msg)
		return
	}
	cmd.log.Info("Shutdown initiated successfully, " + msg)
}

func (cmd *Command) peerIDCmd() {
	msg, status := cmd.c.PeerID()
	if !status {
		cmd.log.Error("Failed to fetch peer ID of the node", "msg", msg)
		return
	}
	_, err := fmt.Fprint(os.Stdout, msg, "\n")
	if err != nil {
		cmd.log.Error(err.Error())
	}
}
