package command

import (
	"fmt"
	"regexp"
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
	is_alphanumeric_sender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	is_alphanumeric_receiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !is_alphanumeric_sender || !is_alphanumeric_receiver {
		cmd.log.Error("Invalid sender or receiver address. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.senderAddr, "bafybmi") || len(cmd.senderAddr) != 59 || !strings.HasPrefix(cmd.receiverAddr, "bafybmi") || len(cmd.receiverAddr) != 59 {
		cmd.log.Error("Invalid sender or receiver DID")
		return
	}
	if cmd.rbtAmount < 0.00001 {
		cmd.log.Error("Invalid RBT amount. RBT amount should be atlease 0.00001")
		return
	}
	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type. TransType should be 1 or 2")
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
