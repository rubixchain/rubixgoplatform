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

func (cmd *Command) PinRBT() {
	rt := model.RBTPinRequest{
		PinningNode: cmd.pinningAddress,
		Sender:      cmd.senderAddr,
		TokenCount:  cmd.rbtAmount,
		Type:        cmd.transType,
		Comment:     cmd.transComment,
	}

	br, err := cmd.c.PinRBT(&rt)
	if err != nil {
		cmd.log.Error("Failed to Pin the Token", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to Pin RBT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("RBT Pinned successfully")
}

func (cmd *Command) SelfTransferRBT() {
	rt := model.RBTTransferRequest{
		Sender:   cmd.senderAddr,
		Receiver: cmd.senderAddr,
		Type:     cmd.transType,
	}

	br, err := cmd.c.TransferRBT(&rt)
	if err != nil {
		cmd.log.Error("Failed to self RBT transfer", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to self transfer RBT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("Self RBT transfer successful")
}
