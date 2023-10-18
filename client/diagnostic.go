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
