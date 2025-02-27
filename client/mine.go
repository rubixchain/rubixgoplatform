package client

import (
	// "fmt"
	// "time"

	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) MineRBTs(miningReq *model.MiningRequest) (*model.BasicResponse, error) {
	// m := make(map[string]string)
	// m["did"] = miningReq.MinerDid
	var br model.BasicResponse
	fmt.Println("client side API is calling below")
	err := c.sendJSONRequest("POST", setup.APIMineRBTs, nil, &miningReq, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil

}
