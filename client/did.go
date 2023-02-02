package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) GetAllDIDs() (*model.GetAccountInfo, error) {
	var ac model.GetAccountInfo
	err := c.sendJSONRequest("GET", server.APIGetAllDID, nil, &ac)
	if err != nil {
		return nil, err
	}
	return &ac, nil
}
