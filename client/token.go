package client

import (
	"time"

	"strconv"

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

func (c *Client) GetPledgedTokenDetails() (*model.TokenStateResponse, error) {
	var tr model.TokenStateResponse
	err := c.sendJSONRequest("GET", setup.APIGetPledgedTokenDetails, nil, nil, &tr, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to get pledged token details", "err", err)
		return nil, err
	}
	return &tr, nil
}

func (c *Client) GetPinnedInfo(TokenStateHash string) (*model.BasicResponse, error) {
	m := make(map[string]string)
	m["tokenstatehash"] = TokenStateHash
	var br model.BasicResponse
	err := c.sendJSONRequest("DELETE", setup.APICheckPinnedState, m, nil, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed to get Pins", "err", err)
		return nil, err
	}
	return &br, nil
}

func (c *Client) ValidateTokenchain(userDID string, smartContractChainValidation bool, token string, blockCount int) (*model.BasicResponse, error) {
	q := make(map[string]string)
	q["did"] = userDID
	q["token"] = token
	q["blockcount"] = strconv.Itoa(blockCount)
	q["SCChainValidation"] = strconv.FormatBool(smartContractChainValidation)

	var br model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIValidateTokenChain, q, nil, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) GenerateFaucetTestRBT(numTokens int, didStr string) (*model.BasicResponse, error) {
	m := model.FaucetRBTGenerateRequest{
		TokenCount: numTokens,
		DID:        didStr,
	}
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIGenerateFaucetTestToken, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) FaucetTokenCheck(token string, did string) (*model.BasicResponse, error) {
	m := make(map[string]string)
	m["token"] = token
	m["did"] = did
	var rm model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIFaucetTokenCheck, m, nil, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}
func (c *Client) ValidateToken(token string) (*model.BasicResponse, error) {
	q := make(map[string]string)
	q["token"] = token

	var br model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIValidateToken, q, nil, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}
