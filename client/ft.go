package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

type CreateFTReq struct {
	DID        string
	FTName     string
	FTCount    int
	TokenCount int
}

func (c *Client) CreateFT(ftreq *CreateFTReq) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	q := make(map[string]string)
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APICreateNFT, q, fields, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}
