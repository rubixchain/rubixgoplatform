package command

func (cmd *Command) RunUnpledge() {
	msg, status := cmd.c.RunUnpledge()
	cmd.log.Info("Unpledging of pledged tokens has started")
	if !status {

		cmd.log.Error(msg)
		return
	}

	cmd.log.Info(msg)
}

func (cmd *Command) UnpledgePOWBasedPledgedTokens() {
	cmd.log.Info("Unpledging of POW-based pledged tokens has started")
	msg, status := cmd.c.UnpledgePOWBasedPledgedTokens()
	if !status {
		cmd.log.Error(msg)
		return
	}

	cmd.log.Info(msg)
}
