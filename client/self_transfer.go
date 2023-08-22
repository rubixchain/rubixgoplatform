package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) SelfTransferRBT(rt *model.RBTSelfTransferRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIInitiateRBTSelfTransfer, nil, rt, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed Self RBT Transfer", "err", err)
		return nil, err
	}
	return &br, nil
}
