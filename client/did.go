package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/util"
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

func (c *Client) CreateDID(cfg *did.DIDCreate) (string, bool) {
	if cfg.Type < did.BasicDIDMode && cfg.Type > did.WalletDIDMode {
		return "Invalid DID mode", false
	}
	switch cfg.Type {
	case did.BasicDIDMode:
		if cfg.ImgFile == "" {
			c.log.Error("Image file requried")
			return "Image file requried", false
		}
		if !strings.Contains(cfg.ImgFile, did.ImgFileName) {
			util.Filecopy(cfg.ImgFile, did.ImgFileName)
			cfg.ImgFile = did.ImgFileName
		}
		cfg.DIDImgFileName = ""
		cfg.PubImgFile = ""
		cfg.PubKeyFile = ""
	case did.StandardDIDMode:
		if cfg.ImgFile == "" {
			c.log.Error("Image file requried")
			return "Image file requried", false
		}
		if cfg.PubImgFile == "" {
			c.log.Error("Public key file requried")
			return "Public key file requried", false
		}
		if !strings.Contains(cfg.ImgFile, did.ImgFileName) {
			util.Filecopy(cfg.ImgFile, did.ImgFileName)
			cfg.ImgFile = did.ImgFileName
		}
		if !strings.Contains(cfg.PubKeyFile, did.PubKeyFileName) {
			util.Filecopy(cfg.PubKeyFile, did.PubKeyFileName)
			cfg.PubKeyFile = did.PubKeyFileName
		}
		cfg.DIDImgFileName = ""
		cfg.PubImgFile = ""
	case did.WalletDIDMode:
		if cfg.DIDImgFileName == "" {
			c.log.Error("DID image file requried")
			return "DID image file requried", false
		}
		if cfg.PubImgFile == "" {
			c.log.Error("DID public share image file requried")
			return "DID public share image file requried", false
		}
		if cfg.PubKeyFile == "" {
			c.log.Error("Public key file requried")
			return "Public key file requried", false
		}
		if !strings.Contains(cfg.DIDImgFileName, did.DIDImgFileName) {
			util.Filecopy(cfg.DIDImgFileName, did.DIDImgFileName)
			cfg.DIDImgFileName = did.DIDImgFileName
		}
		if !strings.Contains(cfg.PubImgFile, did.PubShareFileName) {
			util.Filecopy(cfg.PubImgFile, did.PubShareFileName)
			cfg.PubImgFile = did.PubShareFileName
		}
		if !strings.Contains(cfg.PubKeyFile, did.PubKeyFileName) {
			util.Filecopy(cfg.PubKeyFile, did.PubKeyFileName)
			cfg.PubKeyFile = did.PubKeyFileName
		}
		cfg.ImgFile = ""
	case did.ChildDIDMode:
		cfg.ImgFile = ""
		cfg.DIDImgFileName = ""
		cfg.PubImgFile = ""
		cfg.PubKeyFile = ""
	}
	jd, err := json.Marshal(&cfg)
	if err != nil {
		c.log.Error("Failed to parse json data", "err", err)
		return "Failed to parse json data", false
	}
	fields := make(map[string]string)
	fields[setup.DIDConfigField] = string(jd)
	files := make(map[string]string)
	if cfg.ImgFile != "" {
		files["image"] = cfg.ImgFile
	}
	if cfg.DIDImgFileName != "" {
		files["did_image"] = cfg.DIDImgFileName
	}
	if cfg.PubImgFile != "" {
		files["pub_image"] = cfg.PubImgFile
	}
	if cfg.PubKeyFile != "" {
		files["pub_key"] = cfg.PubKeyFile
	}
	var dr model.DIDResponse
	err = c.sendMutiFormRequest("POST", setup.APICreateDID, nil, fields, files, &dr)
	if err != nil {
		c.log.Error("Invalid response from the node", "err", err)
		return "Invalid response from the node, " + err.Error(), false
	}
	if !dr.Status {
		c.log.Error("Failed to create DID", "message", dr.Message)
		return "Failed to create DID, " + dr.Message, false
	}
	c.log.Info("DID Created successfully")
	return dr.Result.DID, true
}

func (c *Client) SetupDID(dc *did.DIDCreate) (string, bool) {
	if dc.Type < did.BasicDIDMode && dc.Type > did.WalletDIDMode {
		return "Invalid DID mode", false
	}
	if !strings.Contains(dc.PubImgFile, did.PubShareFileName) ||
		!strings.Contains(dc.DIDImgFileName, did.DIDImgFileName) ||
		!strings.Contains(dc.PubKeyFile, did.PubKeyFileName) ||
		!strings.Contains(dc.QuorumPubKeyFile, did.QuorumPubKeyFileName) ||
		!strings.Contains(dc.QuorumPrivKeyFile, did.QuorumPvtKeyFileName) {
		return "Required files are missing", false
	}
	switch dc.Type {
	case did.BasicDIDMode:
		if !strings.Contains(dc.PrivImgFile, did.PvtShareFileName) ||
			!strings.Contains(dc.PrivKeyFile, did.PvtKeyFileName) {
			return "Required files are missing", false
		}
	case did.StandardDIDMode:
		if !strings.Contains(dc.PrivImgFile, did.PvtShareFileName) {
			return "Required files are missing", false
		}
	}
	jd, err := json.Marshal(&dc)
	if err != nil {
		c.log.Error("Failed to parse json data", "err", err)
		return "Failed to parse json data", false
	}
	fields := make(map[string]string)
	fields[setup.DIDConfigField] = string(jd)
	files := make(map[string]string)
	if dc.PubImgFile != "" {
		files["pub_image"] = dc.PubImgFile
	}
	if dc.DIDImgFileName != "" {
		files["did_image"] = dc.DIDImgFileName
	}
	if dc.PrivImgFile != "" {
		files["priv_image"] = dc.PrivImgFile
	}
	if dc.PubKeyFile != "" {
		files["pub_key"] = dc.PubKeyFile
	}
	if dc.PrivKeyFile != "" {
		files["priv_key"] = dc.PrivKeyFile
	}
	if dc.QuorumPubKeyFile != "" {
		files["quorum_pub_key"] = dc.QuorumPubKeyFile
	}
	if dc.QuorumPrivKeyFile != "" {
		files["quorum_priv_key"] = dc.QuorumPrivKeyFile
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
	m := make(map[string]interface{})
	m["did"] = didStr
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", setup.APIRegisterDID, nil, &m, &rm)
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
