package command

func (cmd *Command) getPeerBalance() {
	msg, status := cmd.c.PeerBalance(cmd.peerID, cmd.did)
	if !status {
		cmd.log.Error("Ping failed", "message", msg)
	} else {
		cmd.log.Info("Token Balance retrieved successfully", "message", msg)
	}

}
