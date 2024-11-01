package command

import (
	"fmt"
	"regexp"
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
	isAlphanumericSender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.senderAddr)
	isAlphanumericReceiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.receiverAddr)
	if !isAlphanumericSender || !isAlphanumericReceiver {
		cmd.log.Error("Invalid sender or receiver address. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.senderAddr, "bafybmi") || len(cmd.senderAddr) != 59 || !strings.HasPrefix(cmd.receiverAddr, "bafybmi") || len(cmd.receiverAddr) != 59 {
		cmd.log.Error("Invalid sender or receiver DID")
		return
	}

	transferFtReq := model.TransferFTReq{
		Receiver:   cmd.receiverAddr,
		Sender:     cmd.senderAddr,
		FTName:     cmd.ftName,
		FTCount:    cmd.ftCount,
		Type:       cmd.transType,
		Comment:    cmd.transComment,
		CreatorDID: cmd.creatorDID,
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
	cmd.log.Info("FT transferred successfully")
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
		var creatorDIDs []string
		for _, result := range info.FTInfo {
			ftNames = append(ftNames, result.FTName)
			ftCounts = append(ftCounts, fmt.Sprintf("%d", result.FTCount))
			creatorDIDs = append(creatorDIDs, result.CreatorDID)
		}
		maxNameLength := 0
		for _, name := range ftNames {
			if len(name) > maxNameLength {
				maxNameLength = len(name)
			}
		}
		for i, name := range ftNames {
			fmt.Printf("%-*s: %s (CreatorDID: %s)\n", maxNameLength, name, ftCounts[i], creatorDIDs[i])
		}
	}
}
