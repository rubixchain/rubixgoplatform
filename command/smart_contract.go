package command

func (cmd *Command) PublishContract() {
	basicResponse, err := cmd.c.PublishNewEvent(cmd.smartContractToken, cmd.did, cmd.newContractBlock)

	if err != nil {
		cmd.log.Error("Failed to publish event", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to publish event", "msg", basicResponse.Message)
		return
	}
	message, status := cmd.SignatureResponse(basicResponse)

	if !status {
		cmd.log.Error("Failed to publish event, " + message)
		return
	}
	cmd.log.Info("New event published successfully")
}
func (cmd *Command) SubscribeContract() {

	basicResponse, err := cmd.c.SubscribeContract(cmd.smartContractToken)

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
