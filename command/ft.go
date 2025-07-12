package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) createFT() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to DID")
			return
		}
	}
	if strings.TrimSpace(cmd.ftName) == "" {
		cmd.log.Error("FT Name can't be empty")
		return
	}
	switch {
	case cmd.ftCount <= 0:
		cmd.log.Error("number of tokens to create must be greater than zero")
		return
	case cmd.rbtAmount <= 0:
		cmd.log.Error("number of whole tokens must be a positive integer")
		return
	case cmd.ftCount > int(cmd.rbtAmount*1000):
		cmd.log.Error("max allowed FT count is 1000 for 1 RBT")
		return
	}
	if cmd.rbtAmount != float64(int(cmd.rbtAmount)) {
		cmd.log.Error("rbtAmount must be a positive integer")
		return
	}
	br, err := cmd.c.CreateFT(cmd.did, cmd.ftName, cmd.ftCount, int(cmd.rbtAmount), cmd.ftNumStartIndex)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "no records found") || strings.Contains(br.Message, "no records found") {
			cmd.log.Error("Failed to create FT, No RBT available to create FT")
			return
		}
		cmd.log.Error("Failed to create FT", "err", err)
		return
	}

	msg, status := cmd.SignatureResponse(br)
	if !status || !br.Status {
		cmd.log.Error("Failed to create FT, " + msg + ", Response message: " + br.Message)
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
	// Validating sender & receiver address
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
	// Validating creator DID
	if cmd.creatorDID != "" {
		isAlphanumericCreatorDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.creatorDID)
		if !isAlphanumericCreatorDID || !strings.HasPrefix(cmd.senderAddr, "bafybmi") {
			cmd.log.Error("Invalid creator DID. Please provide valid DID")
			return
		}
	}
	if cmd.ftCount < 1 {
		cmd.log.Error("Input transaction amount is less than minimum FT transaction amount")
		return
	}
	if cmd.ftName == "" {
		cmd.log.Error("FT name cannot be empty")
	}
	if cmd.transType != 0 && cmd.transType != 1 && cmd.transType != 2 {
		cmd.log.Error("Quorum type should be either 1 or 2")
		return
	}
	transferFtReq := model.TransferFTReq{
		Receiver:   cmd.receiverAddr,
		Sender:     cmd.senderAddr,
		FTName:     cmd.ftName,
		FTCount:    cmd.ftCount,
		QuorumType: cmd.transType,
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
	// cmd.log.Info("FT transferred successfully")
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
		cmd.log.Error("Failed to get FT info of DID " + cmd.did, "message", info.Message)
	} else if len(info.FTInfo) == 0 {
		cmd.log.Info("No FTs found for DID " + cmd.did)
	} else {
		cmd.log.Info("Successfully got FT information of DID " + cmd.did)
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
