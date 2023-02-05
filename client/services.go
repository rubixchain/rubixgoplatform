package client

import (
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) SetupService(scfg *config.ServiceConfig) (string, bool) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APISetupService, nil, scfg, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}
