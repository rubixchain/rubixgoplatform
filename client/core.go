package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) GetNodeStatus() bool {
	var br model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APINodeStatus, nil, nil, &br)
	if err != nil {
		c.log.Error("Failed to get node status", "err", err)
		return false
	}
	if !br.Status {
		c.log.Error("Failed to get node status", "msg", br.Message)
		return false
	}
	return true
}
