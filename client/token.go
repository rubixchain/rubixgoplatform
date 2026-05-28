package client

import (
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (c *Client) GenerateLocalRBT(numTokens int, didStr string, startIndex int) (*models.BasicResponse, error) {
	m := models.GenerateLocalRBTRequest{
		NumberOfTokens: numTokens,
		DID:            didStr,
		StartIndex:     startIndex,
	}
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIGenerateLocalRBT, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) GenerateMainnetRBT(numTokens int, didStr string, startIndex int) (*models.BasicResponse, error) {
	m := models.GenerateLocalRBTRequest{
		NumberOfTokens: numTokens,
		DID:            didStr,
		StartIndex:     startIndex,
	}
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIGenerateMainnetRBT, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) GetAllTokens(didStr string, tokenType string) (*models.TokenResponse, error) {
	q := make(map[string]string)
	q["type"] = tokenType
	q["did"] = didStr
	var tr models.TokenResponse
	err := c.sendJSONRequest("GET", setup.APIGetAllTokens, q, nil, &tr)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

func (c *Client) GenerateFaucetTestRBT(numTokens int, didStr string) (*models.BasicResponse, error) {
	m := models.FaucetRBTGenerateRequest{
		TokenCount: numTokens,
		DID:        didStr,
	}
	var rm models.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIGenerateFaucetTestToken, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

