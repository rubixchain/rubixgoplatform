package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) RunUnpledge() (string, bool) {
	var resp model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRunUnpledge, nil, struct{}{}, &resp)
	if err != nil {
		return err.Error(), false
	}

	return resp.Message, resp.Status
}
