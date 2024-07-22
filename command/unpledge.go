package command

func (cmd *Command) RunUnpledge() {
	msg, status := cmd.c.RunUnpledge()
	if !status {
		cmd.log.Error(msg)
		return
	}

	cmd.log.Info(msg)
}

func (cmd *Command) UnpledgePOWBasedPledgedTokens() {
	msg, status := cmd.c.UnpledgePOWBasedPledgedTokens()
	if !status {
		cmd.log.Error(msg)
		return
	}

	cmd.log.Info(msg)
}
