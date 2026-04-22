package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (c *Client) CreateFT(did string, ftName string, ftCount int, wholeToken int, ftNumStartIndex int) (*model.BasicResponse, error) {
	createFTReq := types.CreateFTReq{
		DID:             did,
		FTName:          ftName,
		FTCount:         ftCount,
		TokenCount:      wholeToken,
		FTNumStartIndex: ftNumStartIndex,
	}
	var basicresponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APICreateFT, nil, &createFTReq, &basicresponse)
	if err != nil {
		return nil, err
	}
	return &basicresponse, nil
}

func (c *Client) TransferFT(rt *model.TransferFTReq) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIInitiateFTTransfer, nil, rt, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed FT Transfer", "err", err)
		return nil, err
	}
	return &br, nil
}

func (c *Client) GetFTInfo(didStr string) (*model.BasicResponse, error) {
	pathParams := make(map[string]string)
	pathParams["did"] = didStr
	endpoint, err := ensweb.SubstitutePathParams(setup.APIGetFtByDid, pathParams)
	if err != nil {
		return nil, err
	}
	var info model.BasicResponse
	err = c.sendJSONRequest("GET", endpoint, nil, nil, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) FixFTCreator() (*model.BasicResponse, error) {
	var basicresponse model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIFixFTCreator, nil, nil, &basicresponse, time.Minute*5)
	if err != nil {
		return nil, err
	}
	return &basicresponse, nil
}

func (c *Client) GetFTCreatorStats() (*model.BasicResponse, error) {
	var basicresponse model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIGetFTCreatorStats, nil, nil, &basicresponse)
	if err != nil {
		return nil, err
	}
	return &basicresponse, nil
}
