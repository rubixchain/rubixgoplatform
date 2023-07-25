package client

import (
	"encoding/json"
	"io/ioutil"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) AddQuorum(quorumList string) (string, bool) {
	if quorumList == "" {
		c.log.Error("Quorum list required")
		return "Quorum list required", false
	}
	qlb, err := ioutil.ReadFile(quorumList)
	if err != nil {
		c.log.Error("Invalid file", "err", err)
		return "Invalid file, failed to add quorum list", false
	}
	var ql []core.QuorumData
	err = json.Unmarshal(qlb, &ql)
	if err != nil {
		c.log.Error("Invalid file, failed to add quorum list", "err", err)
		return "Invalid file, failed to add quorum list", false
	}
	var resp model.BasicResponse
	err = c.sendJSONRequest("POST", setup.APIAddQuorum, nil, &ql, &resp)
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
