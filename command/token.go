package command

import (
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) GenerateTestRBT() {
	rt := model.RBTGenerateRequest{
		NumberOfTokens: cmd.numTokens,
		DID:            cmd.did,
	}
	c, r, err := cmd.basicClient("POST", server.APIGenerateTestToken, &rt)
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
		cmd.log.Error("Failed to transfer RBT", "message", dresp.Message)
		return
	}
	if cmd.forcePWD {
		cmd.log.Error("Failed to transfer RBT", "message", dresp.Message)
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
		cmd.log.Error("Failed to generate RBT", "message", response.Message)
		return
	}
	cmd.log.Info("Test RBT generated successfully")
}
