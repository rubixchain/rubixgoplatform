package command

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) TransferRBT() {
	if cmd.senderAddr == "" {
		cmd.log.Info("Sender address cannot be empty")
		fmt.Print("Enter Sender DID : ")
		_, err := fmt.Scan(&cmd.senderAddr)
		if err != nil {
			cmd.log.Error("Failed to get Sender DID")
			return
		}
	}
	if cmd.receiverAddr == "" {
		cmd.log.Info("Receiver address cannot be empty")
		fmt.Print("Enter Receiver DID : ")
		_, err := fmt.Scan(&cmd.receiverAddr)
		if err != nil {
			cmd.log.Error("Failed to get Receiver DID")
			return
		}
	}
	if strings.Contains(cmd.senderAddr, ".") || strings.Contains(cmd.receiverAddr, ".") {
		cmd.log.Error("Invalid sender or receiver address. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.senderAddr, "bafybmi") || len(cmd.senderAddr) < 59 || !strings.HasPrefix(cmd.receiverAddr, "bafybmi") || len(cmd.receiverAddr) < 59 {
		cmd.log.Error("Invalid sender or receiver DID")
		return
	}
	if cmd.rbtAmount == 0.0 || cmd.rbtAmount < 0.00001 {
		cmd.log.Error("Invalid RBT amount")
		return
	}
	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type")
		return
	}
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
