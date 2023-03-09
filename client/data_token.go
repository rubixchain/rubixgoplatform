package client

import (
	"path"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

type DataTokenReq struct {
	DID          string
	UserID       string
	UserInfo     string
	FileInfo     string
	Files        []string
	CommitterDID string
}

func (c *Client) CreateDataToken(dt *DataTokenReq) (*model.BasicResponse, error) {
	fields := make(map[string]string)
	files := make(map[string]string)
	if dt.UserID != "" {
		fields[core.DTUserIDField] = dt.UserID
	}
	if dt.UserInfo != "" {
		fields[core.DTUserInfoField] = dt.UserInfo
	}
	if dt.FileInfo != "" {
		fields[core.DTFileInfoField] = dt.FileInfo
	}
	if dt.CommitterDID != "" {
		fields[core.DTCommiterDIDField] = dt.CommitterDID
	}

	for _, fn := range dt.Files {
		fuid := path.Base(fn)
		files[fuid] = fn
	}
	var br model.BasicResponse
	q := make(map[string]string)
	q["did"] = dt.DID
	err := c.sendMutiFormRequest("POST", server.APICreateDataToken, q, fields, files, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) CommitDataToken(did string) (*model.BasicResponse, error) {
	var br model.BasicResponse
	q := make(map[string]string)
	q["did"] = did
	err := c.sendMutiFormRequest("POST", server.APICommitDataToken, q, nil, nil, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}
