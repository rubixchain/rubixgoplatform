package command

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) createFT() {
	if cmd.did == "" {
		cmd.log.Error("Failed to create FT, DID is required to create FT")
		return
	}

	br, err := cmd.c.CreateFT(cmd.did, cmd.ftName, cmd.ftCount, cmd.rbtAmount)

	if err != nil {
		cmd.log.Error("Failed to create FT", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to create FT", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to create FT, " + msg)
		return
	}
	cmd.log.Info("FT created successfully")
}

func (cmd *Command) transferFT() {
	transferFtReq := model.TransferFTReq{
		Receiver: cmd.receiverAddr,
		Sender:   cmd.senderAddr,
		FTName:   cmd.ftName,
		FTCount:  cmd.ftCount,
		Type:     cmd.transType,
		Comment:  cmd.transComment,
	}

	br, err := cmd.c.TransferFT(&transferFtReq)
	if err != nil {
		cmd.log.Error("Failed FT transfer", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to trasnfer FT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("FT transfered successfully")
}

func (cmd *Command) getFTinfo() {
	info, err := cmd.c.GetFTInfo(cmd.did)
	if err != nil {
		cmd.log.Error("Unable to get FT info, Invalid response from the node", "err", err)
		return
	}
	if !info.Status {
		cmd.log.Error("Failed to get FT info", "message", info.Message)
	} else {
		cmd.log.Info("Successfully got FT information")
		fmt.Printf("")
		for _, result := range info.FTInfo {
			fmt.Printf("%+v\n", result)
		}
	}
}
