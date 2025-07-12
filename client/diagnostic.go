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

func (c *Client) DumpFTTokenChain(token string, blockID string) (*model.TCDumpReply, error) {
	dr := &model.TCDumpRequest{
		Token:   token,
		BlockID: blockID,
	}
	var drep model.TCDumpReply
	err := c.sendJSONRequest("POST", setup.APIDumpFTTokenChainBlock, nil, dr, &drep)
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

func (c *Client) DumpNFTTokenChain(token string, blockID string) (*model.TCDumpReply, error) {
	q := make(map[string]string)
	q["nft"] = token
	q["blockId"] = blockID
	var drep model.TCDumpReply
	err := c.sendJSONRequest("GET", setup.APIDumpNFTTokenChain, q, nil, &drep)
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

func (c *Client) GetNFTTokenData(token string, latest bool) (*model.NFTDataReply, error) {
	getReq := &model.SmartContractTokenChainDataReq{
		Token:  token,
		Latest: latest,
	}
	var nftDataReply model.NFTDataReply
	err := c.sendJSONRequest("GET", setup.APIGetNFTTokenChainData, nil, getReq, &nftDataReply)
	if err != nil {
		return nil, err
	}
	return &nftDataReply, nil

}

func (c *Client) RemoveTokenChainBlock(token string, latest bool, tokenTypeStr string) (*model.TCRemoveReply, error) {
	removeReq := &model.TCRemoveRequest{
		Token:  token,
		Latest: latest,
		TokenTypeString: tokenTypeStr,
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
