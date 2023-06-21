package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) PublishNewEvent(nc *model.NewContractEvent) (*model.BasicResponse, error) {
	var req model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIPublishEvent, nil, nc, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}
func (c *Client) SubscribeContract(ns *model.NewSubcription) (*model.BasicResponse, error) {
	var req model.BasicResponse
	err := c.sendJSONRequest("POST", server.APISubscribecontract, nil, ns, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}
