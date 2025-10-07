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

func (cmd *Command) fixFTCreator() {
	cmd.log.Info("Starting FT Creator fix utility...")
	
	// Call the fix FT creator API
	br, err := cmd.c.FixFTCreator()
	if err != nil {
		cmd.log.Error("Failed to fix FT creators", "err", err)
		return
	}
	
	if !br.Status {
		cmd.log.Error("Failed to fix FT creators", "message", br.Message)
		return
	}
	
	cmd.log.Info("FT Creator fix completed successfully", "message", br.Message)
	
	// Display results if any
	if br.Result != nil {
		if results, ok := br.Result.([]interface{}); ok && len(results) > 0 {
			cmd.log.Info("Fixed tokens count", "count", len(results))
			// Show first few examples
			shown := 0
			for _, r := range results {
				if shown >= 5 {
					break
				}
				if result, ok := r.(map[string]interface{}); ok {
					if result["Success"] == true {
						cmd.log.Info("Fixed token",
							"token", result["TokenID"],
							"old_creator", result["OldCreator"],
							"new_creator", result["NewCreator"])
						shown++
					}
				}
			}
			if len(results) > 5 {
				cmd.log.Info("... and more", "total", len(results))
			}
		}
	}
}

func (cmd *Command) getFTCreatorStats() {
	cmd.log.Info("Getting FT Creator statistics...")
	
	// Call the get FT creator stats API
	br, err := cmd.c.GetFTCreatorStats()
	if err != nil {
		cmd.log.Error("Failed to get FT creator stats", "err", err)
		return
	}
	
	if !br.Status {
		cmd.log.Error("Failed to get FT creator stats", "message", br.Message)
		return
	}
	
	// Display statistics
	if stats, ok := br.Result.(map[string]interface{}); ok {
		fmt.Println("\n=== FT Creator Statistics ===")
		fmt.Printf("Total FT Tokens: %v\n", stats["total_tokens"])
		fmt.Printf("Tokens with Peer ID as Creator: %v (%v)\n", 
			stats["peer_id_creators"], stats["peer_id_percentage"])
		fmt.Printf("Tokens with DID as Creator: %v\n", stats["did_creators"])
		fmt.Printf("Tokens with Empty Creator: %v\n", stats["empty_creators"])
		fmt.Printf("Unique Creators: %v\n", stats["unique_creators"])
		
		// Display top creators
		if topCreators, ok := stats["top_creators"].([]interface{}); ok && len(topCreators) > 0 {
			fmt.Println("\n=== Top Creators ===")
			for i, creator := range topCreators {
				if c, ok := creator.(map[string]interface{}); ok {
					creatorID := c["creator"].(string)
					count := c["count"]
					isPeerID := c["is_peer_id"].(bool)
					
					// Truncate long IDs for display
					displayID := creatorID
					if len(displayID) > 50 {
						displayID = displayID[:47] + "..."
					}
					
					typeStr := "DID"
					if isPeerID {
						typeStr = "PeerID"
					}
					
					fmt.Printf("%d. %s (%s): %v tokens\n", i+1, displayID, typeStr, count)
				}
			}
		}
		
		// Recommendation
		if peerIDCount, ok := stats["peer_id_creators"].(float64); ok && peerIDCount > 0 {
			fmt.Println("\n⚠️  Warning: Found", int(peerIDCount), "tokens with peer ID as creator.")
			fmt.Println("Run 'rubixgoplatform fix-ft-creator' to fix these tokens.")
		}
	}
}
