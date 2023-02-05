package command

import (
	"fmt"
	"time"

	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) GetAccountInfo() {
	c, r, err := cmd.basicClient("GET", server.APIGetAccountInfo, nil)
	if err != nil {
		cmd.log.Error("Failed to get new client", "err", err)
		return
	}
	q := r.URL.Query()
	q.Add("did", cmd.did)
	r.URL.RawQuery = q.Encode()
	resp, err := c.Do(r, time.Minute)
	if err != nil {
		cmd.log.Error("Failed to response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var info model.GetAccountInfo
	err = jsonutil.DecodeJSONFromReader(resp.Body, &info)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	fmt.Printf("Response : %v\n", info)
	if !info.Status {
		cmd.log.Error("Failed to get account info", "message", info.Message)
	} else {
		cmd.log.Info("Successfully got the account information")
		fmt.Printf("Whole RBT : %5d, Locked Whole RBT : %5d, Pledged Whole RBT : %5d\n", info.AccountInfo[0].WholeRBT, info.AccountInfo[0].LockedWholeRBT, info.AccountInfo[0].PledgedWholeRBT)
		fmt.Printf("Part RBT  : %5d, Locked Part RBT  : %5d, Pledged Part RBT  : %5d\n", info.AccountInfo[0].PartRBT, info.AccountInfo[0].LockedPartRBT, info.AccountInfo[0].PledgedPartRBT)
	}
}

func (cmd *Command) TransferRBT() {
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
