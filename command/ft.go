package command

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
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
	} else {
		ftInfo, err := util.ExtractResult[[]types.FTBalance](info.Result)
		if err != nil {
			cmd.log.Error("failed to parse ft balance")
			return
		}
		if len(ftInfo) == 0 {
			cmd.log.Info("No FTs found, DID ", cmd.did)
			return
		}
		cmd.log.Info("Successfully got FT information", "DID ", cmd.did)

		for _, ft := range ftInfo {
			fmt.Printf("FT name: %s, Creator DID: %s, FT Value: %f, FT Count: %d\n", ft.FTName, ft.CreatorDID, ft.FTValue, ft.FTCount)
		}
	}
}
