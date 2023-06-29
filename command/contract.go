package command

func (cmd *Command) PublishContract() {
	basicResponse, err := cmd.c.PublishNewEvent(cmd.contract, cmd.did, cmd.contractBlock)

	if err != nil {
		cmd.log.Error("Failed to publish new event at command", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to publish new event br status false", "msg", basicResponse.Message)
		return
	}
	message, status := cmd.SignatureResponse(basicResponse)

	if !status {
		cmd.log.Error("Failed to publish new event sr status false, " + message)
		return
	}
	cmd.log.Info("New event published successfully")
}
func (cmd *Command) SubscribeContract() {

	basicResponse, err := cmd.c.SubscribeContract(cmd.contract)

	if err != nil {
		cmd.log.Error("Failed to subscribe contract", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to subscribe contract", "msg", basicResponse.Message)
		return
	}
	message, status := cmd.SignatureResponse(basicResponse)

	if !status {
		cmd.log.Error("Failed to subscribe contract, " + message)
		return
	}
	cmd.log.Info("New event subscribed successfully")
}
