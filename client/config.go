package client

import (
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) SetupDB(sc *config.StorageConfig) (string, bool) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APISetupDB, nil, sc, &br)
	if err != nil {
		return "Failed to setup DB, " + err.Error(), false
	}
	return br.Message, br.Status
}
