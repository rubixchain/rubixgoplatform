package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) MigrateNode(m *core.MigrateRequest, timeout ...time.Duration) (*model.BasicResponse, error) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIMigrateNode, nil, m, &rm, timeout...)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) LockToknes(ts []string, timeout ...time.Duration) (*model.BasicResponse, error) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APILockTokens, nil, ts, &rm, timeout...)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}
