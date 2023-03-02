package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) MigrateNode(m *core.MigrateRequest) (*model.BasicResponse, error) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIMigrateNode, nil, m, &rm, 1*time.Hour)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) LockToknes(ts []string) (*model.BasicResponse, error) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APILockTokens, nil, ts, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}
