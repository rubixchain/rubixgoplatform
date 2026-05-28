package client

import (
	"regexp"

	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (c *Client) AddPeerDetailsFromExplorer(did string) (string, bool) {
	if did == "" {
		return "DID cannot be empty", false
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !isAlphanumeric {
		return "Invalid DID. Please provide valid DID", false
	}
	q := make(map[string]string)
	q["did"] = did
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddPeerDetailsFromExplorer, q, nil, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

