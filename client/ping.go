package client

import (
	"fmt"
	"strings"
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
	fmt.Println("Input address is " + quorumAddress)
	q := make(map[string]string)
	// Split the string into two parts based on a delimiter
	parts := strings.Split(quorumAddress, ".")
	if len(parts) != 2 {
		// Handle the case where the string doesn't contain exactly two parts
		return "Invalid quorumAddress format", false
	}
	// Assign the first part to "peerID" and the second part to "dID"
	q["peerID"] = parts[0]
	q["did"] = parts[1]
	fmt.Println("Peerid " + q["peerID"] + " did is " + q["did"])
	var rm model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APICheckQuorumStatus, q, nil, &rm, 2*time.Minute)
	if err != nil {
		return "CHeck quorum failed, " + err.Error(), false
	}
	return rm.Message, rm.Status
}
