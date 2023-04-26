package client

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (c *Client) GetTxnByID(txnID string) (*model.TxnDetails, error) {
	q := make(map[string]string)
	q["txnID"] = txnID
	var td model.TxnDetails
	err := c.sendJSONRequest("GET", server.APIGetTxnByTxnID, q, nil, &td)
	if err != nil {
		return nil, err
	}
	return &td, nil
}

func (c *Client) GetTxnByDID(DID string, role string) (*model.TxnDetails, error) {
	q := make(map[string]string)
	q["DID"] = DID
	q["Role"] = role
	var td model.TxnDetails
	err := c.sendJSONRequest("GET", server.APIGetTxnByDID, q, nil, &td)
	if err != nil {
		return nil, err
	}
	return &td, nil
}

func (c *Client) GetTxnByComment(comment string) (*model.TxnDetails, error) {
	q := make(map[string]string)
	q["Comment"] = comment
	var td model.TxnDetails
	err := c.sendJSONRequest("GET", server.APIGetTxnByComment, q, nil, &td)
	if err != nil {
		return nil, err
	}
	return &td, nil
}
