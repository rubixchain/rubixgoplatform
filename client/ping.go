package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) Ping(peerID string) (string, bool) {
	q := make(map[string]string)
	q["peerID"] = peerID
	var rm model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIPing, q, nil, &rm, 2*time.Minute)
	if err != nil {
		return "Ping failed, " + err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) CheckQuorumStatus(quorumAddress string) (string, bool) {
	q := make(map[string]string)
	q["quorumAddress"] = quorumAddress
	var rm model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APICheckQuorumStatus, q, nil, &rm, 2*time.Minute)
	if err != nil {
		return "Check quorum failed, " + err.Error(), false
	}
	return rm.Message, rm.Status
}
