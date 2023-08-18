package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) GenerateTestRBT(numTokens int, didStr string) (*model.BasicResponse, error) {
	m := model.RBTGenerateRequest{
		NumberOfTokens: numTokens,
		DID:            didStr,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIGenerateTestToken, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) GetAllTokens(didStr string, tokenType string) (*model.TokenResponse, error) {
	q := make(map[string]string)
	q["type"] = tokenType
	q["did"] = didStr
	var tr model.TokenResponse
	err := c.sendJSONRequest("GET", setup.APIGetAllTokens, q, nil, &tr)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}
