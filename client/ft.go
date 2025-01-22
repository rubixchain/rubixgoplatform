package client

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

func (c *Client) CreateFT(did string, ftName string, ftSymbol string, ftCount int, wholeToken int) (*model.BasicResponse, error) {
	createFTReq := model.CreateFTReq{
		DID:        did,
		FTName:     ftName,
		FTSymbol:   ftSymbol,
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

func (c *Client) TransferFT(rt *model.TransferFTReq) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIInitiateFTTransfer, nil, rt, &br, time.Minute*2)
	if err != nil {
		c.log.Error("Failed FT Transfer", "err", err)
		return nil, err
	}
	return &br, nil
}

func (c *Client) GetFTInfo(didStr string) (*model.GetFTInfo, error) {
	m := make(map[string]string)
	m["did"] = didStr
	var info model.GetFTInfo
	err := c.sendJSONRequest("GET", setup.APIGetFTInfo, m, nil, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
