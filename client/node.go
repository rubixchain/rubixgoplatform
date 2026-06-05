package client

import (
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (c *Client) Shutdown() (string, bool) {
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIShutdown, nil, nil, &rm)
	if err != nil {
		return "Failed to shutdown, " + err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) PeerID() (string, bool) {
	var rm models.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIPeerID, nil, nil, &rm)
	if err != nil {
		return "Failed to fetch peer ID of node, error: " + err.Error(), false
	}
	return rm.Message, rm.Status
}
