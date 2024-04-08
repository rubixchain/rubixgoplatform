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
		c.log.Error("Failed to Recover Archive", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}

func (c *Client) Archive(archiveRequest *model.RecoverArchiveReq) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIArchive, nil, archiveRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Archive DID", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}
