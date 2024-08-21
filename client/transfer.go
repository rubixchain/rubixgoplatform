package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) TransferRBT(rt *model.RBTTransferRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIInitiateRBTTransfer, nil, rt, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed RBT Transfer", "err", err)
		return nil, err
	}
	return &br, nil
}

func (c *Client) SelfTransferRBT(rt *model.RBTTransferRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APISelfTransfer, nil, rt, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed RBT Transfer", "err", err)
		return nil, err
	}
	return &br, nil
}

func (c *Client) PinRBT(rt *model.RBTPinRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIInitiatePinRBT, nil, rt, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Pin RBT as a service", "err", err)
		return nil, err
	}
	return &br, nil
}
