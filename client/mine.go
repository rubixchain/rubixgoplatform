package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) MineRBTs(miningReq *model.MiningRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIMineRBTs, nil, &miningReq, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil

}
