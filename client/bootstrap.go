package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) AddBootStrap(peers []string) (string, bool) {
	m := model.BootStrapPeers{
		Peers: peers,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIAddBootStrap, nil, &m, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) RemoveBootStrap(peers []string) (string, bool) {
	m := model.BootStrapPeers{
		Peers: peers,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIRemoveBootStrap, nil, &m, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) RemoveAllBootStrap() (string, bool) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIRemoveAllBootStrap, nil, nil, &rm)
	if err != nil {
		return err.Error(), false
	}
	return rm.Message, rm.Status
}

func (c *Client) GetAllBootStrap() ([]string, string, bool) {
	var rm model.BootStrapResponse
	err := c.sendJSONRequest("GET", server.APIGetAllBootStrap, nil, nil, &rm)
	if err != nil {
		return nil, err.Error(), false
	}
	return rm.Result.Peers, rm.Message, rm.Status
}
