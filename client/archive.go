package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) RecoverArchive(recoverArchiveRequest *model.RecoverArchiveReq) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRecoverArchive, nil, recoverArchiveRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Execute Smart Contract", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}
