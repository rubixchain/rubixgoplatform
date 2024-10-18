package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) AddPeer(peerDetail *wallet.DIDPeerMap) (string, bool) {

	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddPeerDetails, nil, &peerDetail, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}
