package client

import (
	"fmt"
	"path"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

type CreateNFTReq struct {
	DID       string
	NumTokens int
	UserID    string
	UserInfo  string
	FileInfo  string
	Files     []string
}

func (c *Client) CreateNFT(nt *CreateNFTReq) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	files := make(map[string]string)
	if nt.UserID != "" {
		fields[core.DTUserIDField] = nt.UserID
	}
	if nt.UserInfo != "" {
		fields[core.DTUserInfoField] = nt.UserInfo
	}
	if nt.FileInfo != "" {
		fields[core.DTFileInfoField] = nt.FileInfo
	}
	for _, fn := range nt.Files {
		fuid := path.Base(fn)
		files[fuid] = fn
	}
	var br model.BasicResponse
	q := make(map[string]string)
	q["did"] = nt.DID
	if nt.NumTokens > 0 {
		q["numTokens"] = fmt.Sprintf("%d", nt.NumTokens)
	}
	err := c.sendMutiFormRequest("POST", setup.APICreateNFT, q, fields, files, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) GetAllNFTs(did string) (*model.NFTTokens, error) {
	q := make(map[string]string)
	q["did"] = did
	var tkns model.NFTTokens
	err := c.sendJSONRequest("GET", setup.APIGetAllNFT, q, nil, &tkns)
	if err != nil {
		return nil, err
	}
	return &tkns, nil
}
