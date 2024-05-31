package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) AddPeer(peer_detail *wallet.DIDPeerMap) (string, bool) {

	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddPeerDetails, nil, &peer_detail, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}
