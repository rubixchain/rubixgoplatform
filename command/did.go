package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/util"
	"github.com/rubixchain/rubixgoplatform/server"
)

func (cmd *Command) CreateDID() {

	if cmd.didType < did.BasicDIDMode && cmd.didType > did.WalletDIDMode {
		cmd.log.Error("Invalid DID mode")
		return
	}

	cfg := did.DIDCreate{
		Type:      cmd.didType,
		Secret:    cmd.didSecret,
		PrivPWD:   cmd.privPWD,
		QuorumPWD: cmd.quorumPWD,
	}
	switch cmd.didType {
	case did.BasicDIDMode:
		if cmd.imgFile == "" {
			cmd.log.Error("Image file required")
			return
		}
		if !strings.Contains(cmd.imgFile, did.ImgFileName) {
			util.Filecopy(cmd.imgFile, did.ImgFileName)
			cmd.imgFile = did.ImgFileName
		}
		cmd.didImgFile = ""
		cmd.pubImgFile = ""
		cmd.pubKeyFile = ""
	case did.StandardDIDMode:
		if cmd.imgFile == "" {
			cmd.log.Error("Image file required")
			return
		}
		if cmd.pubKeyFile == "" {
			cmd.log.Error("Public key file required")
			return
		}
		if !strings.Contains(cmd.imgFile, did.ImgFileName) {
			util.Filecopy(cmd.imgFile, did.ImgFileName)
			cmd.imgFile = did.ImgFileName
		}
		if !strings.Contains(cmd.pubKeyFile, did.PubKeyFileName) {
			util.Filecopy(cmd.pubKeyFile, did.PubKeyFileName)
			cmd.pubKeyFile = did.PubKeyFileName
		}
		cmd.didImgFile = ""
		cmd.pubImgFile = ""
	case did.WalletDIDMode:
		if cmd.didImgFile == "" {
			cmd.log.Error("DID image file required")
			return
		}
		if cmd.pubImgFile == "" {
			cmd.log.Error("DID public share image file required")
			return
		}
		if cmd.pubKeyFile == "" {
			cmd.log.Error("Public key file required")
			return
		}
		if !strings.Contains(cmd.didImgFile, did.DIDImgFileName) {
			util.Filecopy(cmd.didImgFile, did.DIDImgFileName)
			cmd.didImgFile = did.DIDImgFileName
		}
		if !strings.Contains(cmd.pubImgFile, did.PubShareFileName) {
			util.Filecopy(cmd.pubImgFile, did.PubShareFileName)
			cmd.pubImgFile = did.PubShareFileName
		}
		if !strings.Contains(cmd.pubKeyFile, did.PubKeyFileName) {
			util.Filecopy(cmd.pubKeyFile, did.PubKeyFileName)
			cmd.pubKeyFile = did.PubKeyFileName
		}
		cmd.imgFile = ""
	}
	jd, err := json.Marshal(&cfg)
	if err != nil {
		cmd.log.Error("Failed to marshal dat", "err", err)
		return
	}
	fields := make(map[string]string)
	fields[server.DIDConfigField] = string(jd)
	files := make(map[string]string)
	if cmd.imgFile != "" {
		files["image"] = cmd.imgFile
	}
	if cmd.didImgFile != "" {
		files["did_image"] = cmd.didImgFile
	}
	if cmd.pubImgFile != "" {
		files["pub_image"] = cmd.pubImgFile
	}
	if cmd.pubKeyFile != "" {
		files["pub_key"] = cmd.pubKeyFile
	}
	c, r, err := cmd.multiformClient("POST", server.APICreateDID, fields, files)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var response server.Response
	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to create DID", "message", response.Message)
		return
	}
	cmd.log.Info("DID Created successfully")
}

func (cmd *Command) GetAllDID() {
	c, r, err := cmd.basicClient("GET", server.APIGetAllDID, nil)
	if err != nil {
		cmd.log.Error("Failed to create http client", "err", err)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		cmd.log.Error("Failed to get response from the node", "err", err)
		return
	}
	defer resp.Body.Close()
	var response server.GetDIDResponse
	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to get DIDs", "message", response.Message)
		return
	}
	fmt.Printf("Response : %v\n", response)
	for i := range response.Result {
		fmt.Printf("Address : %s\n", response.Result[i])
	}
	cmd.log.Info("Got all DID successfully")
}
