package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) PublishNewEvent(contract string, did string, block string) (*model.BasicResponse, error) {
	var req model.BasicResponse
	nc := model.NewContractEvent{
		Contract:          contract,
		Did:               did,
		ContractBlockHash: block,
	}
	err := c.sendJSONRequest("POST", server.APIPublishEvent, nil, &nc, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}
func (c *Client) SubscribeContract(contract string) (*model.BasicResponse, error) {
	var req model.BasicResponse
	ns := model.NewSubcription{
		Contract: contract,
	}
	err := c.sendJSONRequest("POST", server.APISubscribecontract, nil, &ns, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}
