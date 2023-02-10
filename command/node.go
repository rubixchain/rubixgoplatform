package command

func (cmd *Command) ShutDownCmd() {
	msg, status := cmd.c.Shutdown()
	if !status {
		cmd.log.Error("Failed to shutdown", "msg", msg)
		return
	}
	cmd.log.Info("Shutdown initiated successfully, " + msg)
}
