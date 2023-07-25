package client

import (
	"path"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

type DataTokenReq struct {
	DID          string
	UserID       string
	UserInfo     string
	FileInfo     string
	Files        []string
	CommitterDID string
	BatchID      string
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
	if dt.BatchID != "" {
		fields[core.DTBatchIDField] = dt.BatchID
	}

	for _, fn := range dt.Files {
		fuid := path.Base(fn)
		files[fuid] = fn
	}
	var br model.BasicResponse
	q := make(map[string]string)
	q["did"] = dt.DID
	err := c.sendMutiFormRequest("POST", setup.APICreateDataToken, q, fields, files, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) CommitDataToken(did string, batchID string) (*model.BasicResponse, error) {
	var br model.BasicResponse
	q := make(map[string]string)
	q["did"] = did
	q["batchID"] = batchID
	err := c.sendMutiFormRequest("POST", setup.APICommitDataToken, q, nil, nil, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) CheckDataToken(dt string) bool {
	var br model.BasicResponse
	q := make(map[string]string)
	q["data_token"] = dt
	err := c.sendMutiFormRequest("GET", setup.APICheckDataToken, q, nil, nil, &br)
	if err != nil {
		c.log.Error("failed to check data token", "err", err)
		return false
	}
	if !br.Status {
		c.log.Error("failed to check data token", "msg", br.Message)
		return false
	}
	return true
}
