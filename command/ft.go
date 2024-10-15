package command

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) createFT() {
	if cmd.did == "" {
		cmd.log.Error("Failed to create FT, DID is required to create FT")
		return
	}

	if cmd.ftCount < 1 {
		cmd.log.Error("Invalid FT count, minimum FT count is 1")
		return
	}

	if strings.TrimSpace(cmd.ftName) == "" {
		cmd.log.Error("FT Name can't be empty")
		return
	}
	br, err := cmd.c.CreateFT(cmd.did, cmd.ftName, cmd.ftCount, cmd.rbtAmount)
	if strings.Contains(fmt.Sprint(err), "no records found") || strings.Contains(br.Message, "no records found") {
		cmd.log.Error("Failed to create FT, No RBT available to create FT")
		return
	}
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
		cmd.log.Error("Failed to transfer FT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("FT transfered successfully")
}

func (cmd *Command) getFTinfo() {
	info, err := cmd.c.GetFTInfo(cmd.did)
	if strings.Contains(fmt.Sprint(err), "DID does not exist") {
		cmd.log.Error("Failed to get FT info, DID does not exist")
		return
	}
	if err != nil {
		cmd.log.Error("Unable to get FT info, Invalid response from the node", "err", err)
		return
	}
	if !info.Status {
		cmd.log.Error("Failed to get FT info", "message", info.Message)
	} else if len(info.FTInfo) == 0 {
		cmd.log.Info("No FTs found")
	} else {
		cmd.log.Info("Successfully got FT information")
		var ftNames []string
		var ftCounts []string
		for _, result := range info.FTInfo {
			ftNames = append(ftNames, result.FTName)
			ftCounts = append(ftCounts, fmt.Sprintf("%d", result.FTCount))
		}
		maxNameLength := 0
		for _, name := range ftNames {
			if len(name) > maxNameLength {
				maxNameLength = len(name)
			}
		}
		// Print the output
		for i, name := range ftNames {
			fmt.Printf("%-*s: %s", maxNameLength, name, ftCounts[i])
			if i < len(ftNames)-1 {
				fmt.Print(", ")
			}
		}
	}
}
