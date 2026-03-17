package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) AddQuorum(quorumDid string) (string, bool) {
	if quorumDid == "" {
		c.log.Error("Quorum list required")
		return "Quorum list required", false
	}

	req := make(map[string]interface{})
	req["did"] = quorumDid

	var resp model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddQuorum, nil, &req, &resp)
	if err != nil {
		c.log.Error("Failed to add quorum list", "err", err)
		return "Failed to add quorum list, " + err.Error(), false
	}

	if !resp.Status {
		c.log.Error("Failed to add quorum list", "msg", resp.Message)
		return "Failed to add quorum list, " + resp.Message, false
	}
	return "Quorum list added successfully", true
}

func (c *Client) GettAllQuorum() (*model.QuorumListResponse, error) {
	var rm model.QuorumListResponse
	err := c.sendJSONRequest("GET", setup.APIGetAllQuorum, nil, nil, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) RemoveAllQuorum() (string, bool) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIRemoveAllQuorum, nil, nil, &rm)
	if err != nil {
		return "Failed to remove quorum, " + err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) SetupQuorum(did string, pwd string, privPwd string) (string, bool) {
	m := model.QuorumSetup{
		DID:             did,
		Password:        pwd,
		PrivKeyPassword: privPwd,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APISetupQuorum, nil, &m, &rm)
	if err != nil {
		return "Failed to setup quorum, " + err.Error(), false
	}
	return rm.Message, rm.Status
}
