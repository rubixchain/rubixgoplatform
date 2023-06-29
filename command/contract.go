package command

func (cmd *Command) PublishContract() {
	br, err := cmd.c.PublishNewEvent(cmd.contract, cmd.did, cmd.contractBlock)

	if err != nil {
		cmd.log.Error("Failed to publish new event at command", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to publish new event br status false", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to publish new event sr status false, " + msg)
		return
	}
	cmd.log.Info("New event published successfully")
}
func (cmd *Command) SubscribeContract() {

	br, err := cmd.c.SubscribeContract(cmd.contract)

	if err != nil {
		cmd.log.Error("Failed to subscribe contract", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to subscribe contract", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to subscribe contract, " + msg)
		return
	}
	cmd.log.Info("New event subscribed successfully")
}
