package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) GenerateTestRBT(numTokens int, didStr string) (*model.BasicResponse, error) {
	m := model.RBTGenerateRequest{
		NumberOfTokens: numTokens,
		DID:            didStr,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIGenerateTestToken, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}
