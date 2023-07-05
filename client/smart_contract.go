package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) PublishNewEvent(contract string, did string, block string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	newContract := model.NewContractEvent{
		Contract:          contract,
		Did:               did,
		ContractBlockHash: block,
	}
	err := c.sendJSONRequest("POST", server.APIPublishContract, nil, &newContract, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
func (c *Client) SubscribeContract(contract string) (*model.BasicResponse, error) {
	var response model.BasicResponse
	newSubscription := model.NewSubscription{
		Contract: contract,
	}
	err := c.sendJSONRequest("POST", server.APISubscribecontract, nil, &newSubscription, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
