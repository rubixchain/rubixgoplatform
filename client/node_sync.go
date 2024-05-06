package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) NodeSync(did string) (*model.NodeSyncResponse, error) {
	nodeSyncRequest := &model.NodeSyncRequest{
		Did: did,
	}
	var nodeSyncResponse model.NodeSyncResponse
	err := c.sendJSONRequest("POST", setup.APINodeSync, nil, nodeSyncRequest, &nodeSyncResponse)
	if err != nil {
		return nil, err
	}
	return &nodeSyncResponse, nil
}
