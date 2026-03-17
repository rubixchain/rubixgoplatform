package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (c *Client) AddPeer(peerDetail *models.DID) (string, bool) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddPeerDetails, nil, &peerDetail, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}
