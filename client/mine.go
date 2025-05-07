package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) MineRBT(miningReq *model.MiningRequest) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIMineRBT, nil, &miningReq, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil

}
