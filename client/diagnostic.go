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
