package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) RecoverRBT(rt *model.RBTRecoverRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRecoverRBT, nil, rt, &br, time.Minute*60)
	if err != nil {
		c.log.Error("Failed to Recover RBT from the pinning node", "err", err)
		return nil, err
	}
	return &br, nil
}
