package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) CreateFT(did string, ftName string, ftCount int, wholeToken float64) (*model.BasicResponse, error) {
	createFTReq := model.CreateFTReq{
		DID:        did,
		FTName:     ftName,
		FTCount:    ftCount,
		TokenCount: wholeToken,
	}
	var basicresponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APICreateFT, nil, &createFTReq, &basicresponse)
	if err != nil {
		return nil, err
	}
	return &basicresponse, nil
}
