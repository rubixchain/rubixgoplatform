package command

import (
	"fmt"
	"time"

	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/did"
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
	var model model.RBTInfo
	err = jsonutil.DecodeJSONFromReader(resp.Body, &model)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !model.Status {
		cmd.log.Error("Failed to get account info", "message", model.Message)
	} else {
		cmd.log.Info("Successfully got the account information")
		fmt.Printf("Whole RBT : %5d, Locked Whole RBT : %5d, Pledged Whole RBT : %5d\n", model.WholeRBT, model.LockedWholeRBT, model.PledgedWholeRBT)
		fmt.Printf("Part RBT  : %5d, Locked Part RBT  : %5d, Pledged Part RBT  : %5d\n", model.PartRBT, model.LockedPartRBT, model.PledgedPartRBT)
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
	c, r, err := cmd.basicClient("POST", server.APIInitiateRBTTransfer, &rt)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer resp.Body.Close()

	var dresp did.SignResponse
	err = jsonutil.DecodeJSONFromReader(resp.Body, &dresp)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !dresp.Status {
		cmd.log.Error("Failed to trasnfer RBT", "message", dresp.Message)
		return
	}
	if cmd.forcePWD {
		cmd.log.Error("Failed to trasnfer RBT", "message", dresp.Message)
		return
	}
	sr := did.SignRespData{
		ID:       dresp.Result.ID,
		Mode:     dresp.Result.Mode,
		Password: cmd.privPWD,
	}

	c, r, err = cmd.basicClient("POST", server.APISignatureResponse, &sr)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	sresp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer sresp.Body.Close()

	var response model.BasicResponse
	err = jsonutil.DecodeJSONFromReader(sresp.Body, &response)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to trasnfer RBT", "message", response.Message)
		return
	}

	cmd.log.Info("RBT transfered successfully")
}
