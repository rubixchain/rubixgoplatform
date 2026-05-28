package client

import (
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (c *Client) AddBootStrap(peers []string) (string, bool) {
	m := models.BootStrapPeers{
		Peers: peers,
	}
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIAddBootStrap, nil, &m, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) RemoveBootStrap(peers []string) (string, bool) {
	m := models.BootStrapPeers{
		Peers: peers,
	}
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRemoveBootStrap, nil, &m, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) RemoveAllBootStrap() (string, bool) {
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRemoveAllBootStrap, nil, nil, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) GetAllBootStrap() ([]string, string, bool) {
	var rm models.BootStrapResponse
	err := c.sendJSONRequest("GET", setup.APIGetAllBootStrap, nil, nil, &rm)
	if err != nil {
		return nil, err.Error(), false
	}
	return rm.Result.Peers, rm.Message, rm.Status
}
