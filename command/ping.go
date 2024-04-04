package command

func (cmd *Command) ping() {
	msg, status := cmd.c.Ping(cmd.peerID)
	if !status {
		cmd.log.Error("Ping failed", "message", msg)
	} else {
		cmd.log.Info("Ping response received successfully", "message", msg)
	}
}
