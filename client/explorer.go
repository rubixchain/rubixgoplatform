package client

import (
	"regexp"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) AddExplorer(links []string) (string, bool) {
	m := model.ExplorerLinks{
		Links: links,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddExplorer, nil, &m, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) RemoveExplorer(links []string) (string, bool) {
	m := model.ExplorerLinks{
		Links: links,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRemoveExplorer, nil, &m, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) GetAllExplorer() ([]string, string, bool) {
	var rm model.ExplorerResponse
	err := c.sendJSONRequest("GET", setup.APIGetAllExplorer, nil, nil, &rm)
	if err != nil {
		return nil, err.Error(), false
	}
	return rm.Result.Links, rm.Message, rm.Status
}

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
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddPeerDetailsFromExplorer, q, nil, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) AddUserAPIKey(did string, apiKey string) (string, bool) {
	q := make(map[string]string)
	q["did"] = did
	q["apiKey"] = apiKey
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddUserAPIKey, q, nil, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}
