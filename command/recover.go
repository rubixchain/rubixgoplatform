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
	cmd.log.Info("The message response from RecoverRBT function", br.Message)
	cmd.log.Info("The status in RecoverRBT function", br.Status)
	cmd.log.Info("RBT Recovered successfully")
}
