package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (cmd *Command) createNFT() {
	if cmd.did == "" {
		cmd.log.Error("Failed to create NFT, DID is required to create NFT")
		return
	}
	nt := client.CreateNFTReq{
		NumTokens: cmd.numTokens,
		DID:       cmd.did,
		UserID:    cmd.userID,
		UserInfo:  cmd.userInfo,
	}
	if cmd.fileMode {
		nt.Files = make([]string, 0)
		nt.Files = append(nt.Files, cmd.file)
	} else {
		fd, err := ioutil.ReadFile(cmd.file)
		if err != nil {
			cmd.log.Error("Failed to read file", "err", err)
			return
		}
		hb := util.CalculateHash(fd, "SHA3-256")
		fi := make(map[string]map[string]string)
		fn := path.Base(cmd.file)
		info := make(map[string]string)
		info[core.DTFileHashField] = util.HexToStr(hb)
		fi[fn] = info
		jb, err := json.Marshal(fi)
		if err != nil {
			cmd.log.Error("Failed to marshal json input", "err", err)
			return
		}
		nt.FileInfo = string(jb)
	}
	br, err := cmd.c.CreateNFT(&nt)
	if err != nil {
		cmd.log.Error("Failed to create NFT", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to create NFT", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to create NFT, " + msg)
		return
	}
	cmd.log.Info(fmt.Sprintf("Data Token : %s", msg))
	cmd.log.Info("NFT created successfully")
}

func (cmd *Command) getAllNFTs() {
	if cmd.did == "" {
		cmd.log.Error("Failed to get NFTs, DID is required to get NFTs")
		return
	}
	tkns, err := cmd.c.GetAllNFTs(cmd.did)
	if err != nil {
		cmd.log.Error("Failed to get NFTs, " + err.Error())
		return
	}
	for _, tkn := range tkns.Tokens {
		fmt.Printf("NFT : %s, Status : %d\n", tkn.Token, tkn.TokenStatus)
	}
	cmd.log.Info("Got all NFTs successfully")
}
