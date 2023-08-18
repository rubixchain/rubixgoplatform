package command

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) TransferRBT() {
	rt := model.RBTTransferRequest{
		Receiver:   cmd.receiverAddr,
		Sender:     cmd.senderAddr,
		TokenCount: cmd.rbtAmount,
		Type:       cmd.transType,
		Comment:    cmd.transComment,
	}

	br, err := cmd.c.TransferRBT(&rt)
	if err != nil {
		cmd.log.Error("Failed RBT transfer", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to trasnfer RBT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("RBT transfered successfully")
}
