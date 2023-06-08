package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) PeerBalance(peerID string, did string) (string, bool) {
	q := make(map[string]string)
	q["peerID"] = peerID
	q["did"] = did
	var rm model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIGetPeerBalance, q, nil, &rm, 2*time.Minute)
	if err != nil {
		return "Failed to get Balance" + err.Error(), false
	}
	// rm.Message = "Balance for PeerID : " + peerID + " and DID : " + did + " is = "
	return rm.Message, rm.Status
}
