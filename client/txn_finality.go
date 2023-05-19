package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) GetPendingTxn() (*model.PendingTxnIds, error) {
	var result model.PendingTxnIds
	err := c.sendJSONRequest("GET", server.APIGetPendingTxn, nil, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) InitiateTxnFinality(txnId string) (*model.BasicResponse, error) {
	q := make(map[string]string)
	q["txnID"] = txnId
	var result model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIInitiateTxnFinality, q, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
