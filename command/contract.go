package command

import "github.com/rubixchain/rubixgoplatform/core/model"

func (cmd *Command) PublishContract() {
	nc := model.NewContractEvent{
		Contract:          cmd.contract,
		Did:               cmd.did,
		ContractBlockHash: cmd.contractBlock,
	}

	br, err := cmd.c.PublishNewEvent(&nc)

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
	ns := model.NewSubcription{
		Contract: cmd.contract,
	}
	br, err := cmd.c.SubscribeContract(&ns)

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
