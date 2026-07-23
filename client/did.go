package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	model "github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (c *Client) GetAllDIDs() (*model.BasicResponse, error) {
	var ac model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APIGetAllDID, nil, nil, &ac)
	if err != nil {
		return nil, err
	}
	return &ac, nil
}

func (c *Client) CreateDID(cfg *types.DIDCreate) (string, bool) {
	var dr model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APICreateDID, nil, cfg, &dr)
	if err != nil {
		c.log.Error("Invalid response from the node", "err", err)
		return "Invalid response from the node, " + err.Error(), false
	}
	if !dr.Status {
		c.log.Error("Failed to create DID", "message", dr.Message)
		return "Failed to create DID, " + dr.Message, false
	}
	didResult, err := util.ExtractResult[model.DIDResult](dr.Result)
	if err != nil {
		return "Failed to parse DID result, " + err.Error(), false
	}
	return didResult.DID, true
}

func (c *Client) SetupDID(dc *types.DIDCreate) (string, bool) {

	if !strings.Contains(dc.PubKey, constants.PubKeyFileName) ||
		!strings.Contains(dc.PrivKey, constants.PvtKeyFileName) ||
		!strings.Contains(dc.Mnemonic, constants.MnemonicFileName) {
		return "Required files are missing", false
	}

	jd, err := json.Marshal(&dc)
	if err != nil {
		c.log.Error("Failed to parse json data", "err", err)
		return "Failed to parse json data", false
	}
	fields := make(map[string]string)
	fields[setup.DIDConfigField] = string(jd)
	files := make(map[string]string)

	if dc.PubKey != "" {
		files["pub_key"] = dc.PubKey
	}
	if dc.PrivKey != "" {
		files["priv_key"] = dc.PrivKey
	}

	var br model.BasicResponse
	err = c.sendMutiFormRequest("POST", setup.APISetupDID, nil, fields, files, &br)
	if err != nil {
		c.log.Error("Invalid response from the node", "err", err)
		return "Invalid response from the node, " + err.Error(), false
	}
	if !br.Status {
		c.log.Error("Failed to setup DID", "message", br.Message)
		return "Failed to setup DID, " + br.Message, false
	}
	c.log.Info("DID setup successfully")
	return br.Result.(string), true
}

func (c *Client) SignatureResponse(sr *types.SignRespData, timeout ...time.Duration) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APISignatureResponse, nil, sr, &br, timeout...)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) RegisterDID(didStr string) (*model.BasicResponse, error) {
	pathParams := make(map[string]string)
	pathParams["did"] = didStr
	endpoint, err := ensweb.SubstitutePathParams(setup.APIRegisterDID, pathParams)
	if err != nil {
		return nil, err
	}
	var rm model.BasicResponse
	err = c.sendJSONRequest("POST", endpoint, nil, nil, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

// RecoverWalletFromFullnode triggers fullnode-backed wallet recovery. A request
// with only a DID does a full recovery; the mode, filters, self_test, and
// all_dids fields refine it. The node proves DID ownership via the async
// signature challenge, so the caller must drive cmd.SignatureResponse on the
// returned response (same flow as RegisterDID).
func (c *Client) RecoverWalletFromFullnode(req *types.RecoverWalletAdvancedRequest) (*model.BasicResponse, error) {
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRecoverWalletFromFullnode, nil, req, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}

func (c *Client) GetRBTBalance(didStr string) (*model.BasicResponse, error) {
	pathParams := make(map[string]string)
	pathParams["did"] = didStr
	var info model.BasicResponse
	endpoint, err := ensweb.SubstitutePathParams(setup.APIGetRbtByDid, pathParams)
	if err != nil {
		return nil, err
	}
	err = c.sendJSONRequest("GET", endpoint, nil, nil, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) GetDIDBalance(didStr string) (*model.BasicResponse, error) {
	pathParams := make(map[string]string)
	pathParams["did"] = didStr
	var info model.BasicResponse
	endpoint, err := ensweb.SubstitutePathParams(setup.APIGetDIDBalance, pathParams)
	if err != nil {
		return nil, err
	}
	err = c.sendJSONRequest("GET", endpoint, nil, nil, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// Arbitrary signature
func (c *Client) ArbitrarySignature(didStr, msg string) (*model.BasicResponse, error) {
	signData := &model.ArbitrarySignRequest{
		SignerDID: didStr,
		MsgToSign: msg,
	}
	var resp model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIArbitrarySign, nil, signData, &resp)
	if err != nil {
		return &resp, err
	}
	return &resp, nil
}

// signature verification
func (c *Client) SignVerification(signerDID, signedMsg, signature string) (string, error) {
	verificationData := make(map[string]string)
	verificationData["signer_did"] = signerDID
	verificationData["signed_msg"] = signedMsg
	verificationData["signature"] = signature
	var resp model.BasicResponse
	err := c.sendJSONRequest("GET", setup.APISignVerification, verificationData, nil, &resp)
	if err != nil {
		return "arbitrary sign failed", err
	}
	result := fmt.Sprintf("sign verification Status : %v, Message : %v", resp.Status, resp.Message)
	return result, nil
}

