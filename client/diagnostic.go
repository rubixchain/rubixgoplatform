package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) DumpTokenChain(token string, blockID string) (*model.TCDumpReply, error) {
	dr := &model.TCDumpRequest{
		Token:   token,
		BlockID: blockID,
	}
	var drep model.TCDumpReply
	err := c.sendJSONRequest("POST", setup.APIDumpTokenChainBlock, nil, dr, &drep)
	if err != nil {
		return nil, err
	}
	return &drep, nil
}

func (c *Client) DumpSmartContractTokenChain(token string, blockID string) (*model.TCDumpReply, error) {
	dr := &model.TCDumpRequest{
		Token:   token,
		BlockID: blockID,
	}
	var drep model.TCDumpReply
	err := c.sendJSONRequest("POST", setup.APIDumpSmartContractTokenChainBlock, nil, dr, &drep)
	if err != nil {
		return nil, err
	}
	return &drep, nil
}

func (c *Client) GetSmartContractTokenData(token string, latest bool) (*model.SmartContractDataReply, error) {
	getReq := &model.SmartContractTokenChainDataReq{
		Token:  token,
		Latest: latest,
	}
	var sctDataReply model.SmartContractDataReply
	err := c.sendJSONRequest("POST", setup.APIGetSmartContractTokenData, nil, getReq, &sctDataReply)
	if err != nil {
		return nil, err
	}
	return &sctDataReply, nil

}
func (c *Client) RemoveTokenChainBlock(token string, latest bool) (*model.TCRemoveReply, error) {
	removeReq := &model.TCRemoveRequest{
		Token:  token,
		Latest: latest,
	}
	var removeReply model.TCRemoveReply
	err := c.sendJSONRequest("POST", setup.APIRemoveTokenChainBlock, nil, removeReq, &removeReply)
	if err != nil {
		return nil, err
	}
	return &removeReply, nil
}

func (c *Client) ReleaseAllLockedTokens() (*model.BasicResponse, error) {

	var response model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIReleaseAllLockedTokens, nil, nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
