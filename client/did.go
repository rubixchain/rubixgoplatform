package client

import (
	"encoding/json"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/server"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Client) GetAllDIDs() (*model.GetAccountInfo, error) {
	var ac model.GetAccountInfo
	err := c.sendJSONRequest("GET", server.APIGetAllDID, nil, nil, &ac)
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
	}
	jd, err := json.Marshal(&cfg)
	if err != nil {
		c.log.Error("Failed to parse json data", "err", err)
		return "Failed to parse json data", false
	}
	fields := make(map[string]string)
	fields[server.DIDConfigField] = string(jd)
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
	var response model.BasicResponse
	err = c.sendMutiFormRequest("POST", server.APICreateDID, fields, files, &response)
	if err != nil {
		c.log.Error("Invalid response from the node", "err", err)
		return "Invalid response from the node, " + err.Error(), false
	}
	if !response.Status {
		c.log.Error("Failed to create DID", "message", response.Message)
		return "Failed to create DID, " + response.Message, false
	}
	c.log.Info("DID Created successfully")
	return "DID Created successfully", true
}

func (c *Client) SignatureResponse(sr *did.SignRespData) (*model.BasicResponse, error) {
	var br model.BasicResponse
	err := c.sendJSONRequest("POST", server.APISignatureResponse, nil, sr, &br)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func (c *Client) RegisterDID(didStr string) (*model.BasicResponse, error) {
	m := make(map[string]interface{})
	m["did"] = didStr
	var rm model.BasicResponse
	err := c.sendJSONRequest("POST", server.APIRegisterDID, nil, &m, &rm)
	if err != nil {
		return nil, err
	}
	return &rm, nil
}
