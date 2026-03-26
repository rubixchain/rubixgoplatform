package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (c *Client) GetDIDChallenge(did string) (string, error) {
	q := make(map[string]string)
	q["did"] = did
	var resp model.DIDAccessResponse
	err := c.sendJSONRequest("GET", setup.APIGetDIDChallenge, q, nil, &resp)
	if err != nil {
		return "", err
	}
	if !resp.Status {
		return "", fmt.Errorf(resp.Message)
	}
	return resp.Token, nil
}

func (c *Client) GetDIDAccess(req *model.GetDIDAccess) (string, error) {
	var resp model.DIDAccessResponse
	err := c.sendJSONRequest("POST", setup.APIGetDIDAccess, nil, req, &resp)
	if err != nil {
		return "", err
	}
	if !resp.Status {
		return "", fmt.Errorf(resp.Message)
	}
	return resp.Token, nil
}

func (c *Client) GetAllDIDs() (*model.GetAccountInfo, error) {
	var ac model.GetAccountInfo
	err := c.sendJSONRequest("GET", setup.APIGetAllDID, nil, nil, &ac)
	if err != nil {
		return nil, err
	}
	return &ac, nil
}

func (c *Client) CreateDID(cfg *types.DIDCreate) (string, bool) {
	var dr model.DIDResponse
	err := c.sendJSONRequest("POST", setup.APICreateDID, nil, cfg, &dr)
	if err != nil {
		c.log.Error("Invalid response from the node", "err", err)
		return "Invalid response from the node, " + err.Error(), false
	}
	if !dr.Status {
		c.log.Error("Failed to create DID", "message", dr.Message)
		return "Failed to create DID, " + dr.Message, false
	}
	c.log.Info(fmt.Sprintf("DID %v Created successfully", dr.Result.DID))
	return dr.Result.DID, true
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

func (c *Client) SignatureResponse(sr *did.SignRespData, timeout ...time.Duration) (*model.BasicResponse, error) {
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

func (c *Client) GetAccountInfo(didStr string) (*model.GetAccountInfo, error) {
	m := make(map[string]string)
	m["did"] = didStr
	var info model.GetAccountInfo
	err := c.sendJSONRequest("GET", setup.APIGetAccountInfo, m, nil, &info)
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

func (c *Client) RemoveStaleDID(didStr string) (*model.BasicResponse, error) {
	m := make(map[string]interface{})
	m["did"] = didStr
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRemoveStaleDID, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}
