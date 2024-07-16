package command

func (cmd *Command) RunUnpledge() {
	msg, status := cmd.c.RunUnpledge()
	if !status {
		cmd.log.Error(msg)
		return
	}

	cmd.log.Debug(msg)
}