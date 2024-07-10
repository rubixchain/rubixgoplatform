package command

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) RecoverTokens() {
	rt := model.RBTRecoverRequest{
		PinningNode: cmd.pinningAddress,
		Sender:      cmd.senderAddr,
		TokenCount:  cmd.rbtAmount,
	}

	br, err := cmd.c.RecoverRBT(&rt)
	if err != nil {
		cmd.log.Error("Failed to Recover the Tokens", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to recover RBT: " + br.Message)
	} else {
		cmd.log.Info("Recovered RBT: " + br.Message)
	}
}
