package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) DeploySmartContract(deployRequest *model.DeploySmartContractRequest) (*model.BasicResponse, error) {
	var basicResponse model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIDeploySmartContract, nil, deployRequest, &basicResponse, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to Deploy Smart Contract", "err", err)
		return nil, err
	}
	return &basicResponse, nil
}
