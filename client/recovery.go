package client

import (
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	model "github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// StartRecovery starts the account recovery of did. The response asks for
// the DID password, so the caller drives the signature flow on it
// (cmd.SignatureResponse).
func (c *Client) StartRecovery(did string) (*model.BasicResponse, error) {
	req := model.StartRecoveryRequest{DID: did}
	var rm model.BasicResponse
	if err := c.sendJSONRequest("POST", setup.APIStartRecovery, nil, &req, &rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

// GetRecoverySession returns the node's view of the recovery started by
// requestID.
func (c *Client) GetRecoverySession(requestID string) (*types.RecoverySessionResponse, error) {
	endpoint, err := ensweb.SubstitutePathParams(setup.APIGetRecoverySession, map[string]string{"request_txn_id": requestID})
	if err != nil {
		return nil, err
	}
	var rm types.RecoverySessionResponse
	if err := c.sendJSONRequest("GET", endpoint, nil, nil, &rm); err != nil {
		return nil, err
	}
	return &rm, nil
}
