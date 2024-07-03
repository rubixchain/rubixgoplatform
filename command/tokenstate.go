package command

import "fmt"

func (cmd *Command) GetPledgedTokenDetails() {
	info, err := cmd.c.GetPledgedTokenDetails()
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	fmt.Printf("Response : %v\n", info)
	if !info.Status {
		cmd.log.Error("Failed to get account info", "message", info.Message)
	} else {
		cmd.log.Info("Successfully got the pledged token states info")
		fmt.Println("DID	", "Pledged Token	", "Token State")
		for _, i := range info.PledgedTokenStateDetails {
			fmt.Println(i.DID, "	", i.TokensPledged, "	", i.TokenStateHash)
		}
	}
}

//command will take token hash, check for ipfs pinning, if no, ignore, if yes, get token detail, unpledge.

func (cmd *Command) CheckPinnedState() {
	info, err := cmd.c.GetPinnedInfo(cmd.TokenState)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	fmt.Printf("Response : %v\n", info)
	if !info.Status {
		cmd.log.Debug("Pin not available", "message", info.Message)
	} else {
		cmd.log.Info("Token State is Pinned")
	}
}
