package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) Shutdown() (string, bool) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIShutdown, nil, nil, &rm)
	if err != nil {
		return "Failed to shutdown, " + err.Error(), false
	}
	return rm.Message, rm.Status
}
