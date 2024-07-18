package command

import (
	"github.com/rubixchain/rubixgoplatform/client"
)

func (cmd *Command) createFT() {
	if cmd.did == "" {
		cmd.log.Error("Failed to create FT, DID is required to create FT")
		return
	}

	ftReq := client.CreateFTReq{
		DID:        cmd.did,
		FTName:     cmd.ftName,
		FTCount:    cmd.ftCount,
		TokenCount: int(cmd.rbtAmount),
	}
	br, err := cmd.c.CreateFT(&ftReq)
	if err != nil {
		cmd.log.Error("Failed to create FT", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to create NFT", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to create FT, " + msg)
		return
	}
	cmd.log.Info("FT created successfully")
}
