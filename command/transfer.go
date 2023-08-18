package command

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
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
		fmt.Printf("RBT : %10.3f, Locked RBT : %10.3f, Pledged RBT : %10.3f\n", info.AccountInfo[0].RBTAmount, info.AccountInfo[0].LockedRBT, info.AccountInfo[0].PledgedRBT)
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
