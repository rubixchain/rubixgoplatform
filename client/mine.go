package client

import (
	// "fmt"
	// "time"

	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) MineRBTs(didStr string) (*model.BasicResponse, error) {
	m := make(map[string]string)
	m["did"] = didStr
	var br model.BasicResponse
	fmt.Println("clinet side API is calling below")
	err := c.sendJSONRequest("POST", setup.APIMineRBTs, nil, &m, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil

}
