package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type NFTReq struct {
	DID        string
	NumTokens  int
	Fields     map[string][]string
	FileNames  []string
	FolderName string
}

type NFTSale struct {
	Token  string  `json:"token"`
	Amount float64 `json:"amount"`
}

type NFTSaleReq struct {
	Type   int       `json:"type"`
	DID    string    `json:"did"`
	Tokens []NFTSale `json:"tokens"`
}

func (c *Core) CreateNFT(reqID string, nr *NFTReq) {
	defer os.RemoveAll(nr.FolderName)
	br := c.createNFT(reqID, nr)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to create NFT, failed to get did channel")
		return
	}
	dc.OutChan <- br
}

func (c *Core) createNFT(reqID string, nr *NFTReq) *model.BasicResponse {
	defer os.RemoveAll(nr.FolderName)
	br := model.BasicResponse{
		Status: false,
	}

	dc, err := c.SetupDID(reqID, nr.DID)
	if err != nil {
		c.log.Error("Failed to create NFT, failed to setup did", "err", err)
		br.Message = "Failed to create NFT, failed to setup did"
		return &br
	}
	userID, ok := nr.Fields[DTUserIDField]
	if !ok {
		c.log.Error("Failed to create NFT, user ID missing")
		br.Message = "Failed to create NFT, user ID missing"
		return &br
	}
	rt := rac.RacType{
		Type:        c.TokenType(NFTString),
		DID:         nr.DID,
		TotalSupply: uint64(nr.NumTokens),
		CreatorID:   userID[0],
	}
	userInfo, ok := nr.Fields[DTUserInfoField]
	if ok {
		rt.CreatorInput = userInfo[0]
	}
	fileInfo, fok := nr.Fields[DTFileInfoField]
	if fok {
		var fi map[string]map[string]string
		err := json.Unmarshal([]byte(fileInfo[0]), &fi)
		if err != nil {
			c.log.Error("Failed to create NFT, invalid file info")
			br.Message = "Failed to create NFT, invalid file info"
			return &br
		}
		for k, v := range fi {
			ch, ok := v[DTFileHashField]
			if ok {
				if rt.ContentHash == nil {
					rt.ContentHash = make(map[string]string)
				}
				rt.ContentHash[k] = ch
			}
			cu, ok := v[DTFileURLField]
			if ok {
				if rt.ContentURL == nil {
					rt.ContentURL = make(map[string]string)
				}
				rt.ContentURL[k] = cu
			}
			ti, ok := v[DTFileTransInfoField]
			if ok {
				if rt.TransInfo == nil {
					rt.TransInfo = make(map[string]string)
				}
				rt.TransInfo[k] = ti
			}
		}
	}

	for _, file := range nr.FileNames {
		fn := strings.TrimPrefix(file, nr.FolderName+"/")
		fb, err := ioutil.ReadFile(file)
		if err != nil {
			c.log.Error("Failed to create NFT, failed to read file", "err", err)
			br.Message = "Failed to create NFT, failed to read file"
			return &br
		}
		hb := util.CalculateHash(fb, "SHA3-256")
		fbr := bytes.NewBuffer(fb)
		fileUrl, err := c.ipfs.Add(fbr)
		if err != nil {
			c.log.Error("Failed to create NFT, failed to add file to ipfs", "err", err)
			br.Message = "Failed to create NFT, failed to add file to ipfs"
			return &br
		}
		if rt.ContentHash == nil {
			rt.ContentHash = make(map[string]string)
		}
		rt.ContentHash[fn] = util.HexToStr(hb)
		if rt.ContentURL == nil {
			rt.ContentURL = make(map[string]string)
		}
		rt.ContentURL[fn] = fileUrl
	}

	dtb, err := rac.CreateRac(&rt)
	if err != nil {
		c.log.Error("Failed to create NFT, failed to create rac block", "err", err)
		br.Message = "Failed to create NFT, failed to create rac block"
		return &br
	}
	nfts := make([]string, 0)
	for _, b := range dtb {
		err = b.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to create NFT, failed to update signature", "err", err)
			br.Message = "Failed to create NFT, failed to update signature"
			return &br
		}
		rtb := b.GetBlock()
		td := util.HexToStr(rtb)
		fr := bytes.NewBuffer([]byte(td))
		nt, err := c.ipfs.Add(fr)
		if err != nil {
			c.log.Error("Failed to create NFT, failed to add rac token to ipfs", "err", err)
			br.Message = "Failed to create NFT, failed to add rac token to ipfs"
			return &br
		}
		nfts = append(nfts, nt)
	}
	bgti := make([]block.GenesisTokenInfo, 0)
	btt := make([]block.TransTokens, 0)
	ctcb := make(map[string]*block.Block)
	for _, nt := range nfts {
		ctcb[nt] = nil
		bgti = append(bgti, block.GenesisTokenInfo{Token: nt})
		btt = append(btt, block.TransTokens{Token: nt, TokenType: c.TokenType(NFTString)})
	}
	gb := &block.GenesisBlock{
		Type: block.TokenGeneratedType,
		Info: bgti,
	}
	bti := &block.TransInfo{
		Tokens:  btt,
		Comment: "NFT generated at : " + time.Now().String(),
	}
	tcb := &block.TokenChainBlock{
		TransactionType: block.TokenGeneratedType,
		TokenOwner:      nr.DID,
		GenesisBlock:    gb,
		TransInfo:       bti,
	}
	blk := block.CreateNewBlock(ctcb, tcb)
	if blk == nil {
		c.log.Error("Failed to create NFT, unable to create token chain")
		br.Message = "Failed to create NFT, unable to create token chain"
		return &br
	}
	err = blk.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to create NFT, failed to update signature", "err", err)
		br.Message = "Failed to create NFT, failed to update signature"
		return &br
	}
	msg := ""
	for _, nt := range nfts {
		err = c.w.AddTokenBlock(nt, blk)
		if err != nil {
			c.log.Error("Failed to create NFT, failed to add token chain block", "err", err)
			br.Message = "Failed to create NFT, failed to add token chain block"
			return &br
		}
		err = c.w.CreateNFT(&wallet.NFT{TokenID: nt, DID: nr.DID})
		if err != nil {
			c.log.Error("Failed to create NFT, write failed", "err", err)
			br.Message = "Failed to create NFT, write failed"
			return &br
		}
		if msg != "" {
			msg = msg + ","
		}
		msg = msg + nt
	}
	br.Status = true
	br.Message = msg
	return &br
}

func (c *Core) GetAllNFT(did string) model.NFTTokens {
	resp := model.NFTTokens{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got all NFTs successfully",
		},
		Tokens: make([]model.NFTStatus, 0),
	}
	tkns := c.w.GetAllNFT(did)
	for _, tkn := range tkns {
		resp.Tokens = append(resp.Tokens, model.NFTStatus{Token: tkn.TokenID, TokenStatus: tkn.TokenStatus})
	}
	return resp
}

func (c *Core) AddNFTSaleContract(reqID string, sr *NFTSaleReq) {
	br := c.addNFTSaleContract(reqID, sr)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to add NFT for sale, failed to get did channel")
		return
	}
	dc.OutChan <- br
}

func (c *Core) addNFTSaleContract(reqID string, sr *NFTSaleReq) *model.BasicResponse {
	resp := &model.BasicResponse{
		Status: false,
	}
	did := sr.DID
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	nts := make([]wallet.NFT, 0)
	for _, t := range sr.Tokens {
		nt, err := c.w.GetNFT(did, t.Token, true)
		if err == nil {
			nt.TokenValue = t.Amount
			nts = append(nts, *nt)
		}
	}

	// // release the locked tokens before exit
	// defer c.w.ReleaseTokens(wt)

	wta := make([]string, 0)
	for i := range nts {
		wta = append(wta, nts[i].TokenID)
	}
	totalAmount := float64(0)
	tis := make([]contract.TokenInfo, 0)
	for i := range nts {
		tts := NFTString
		tt := c.TokenType(tts)
		blk := c.w.GetLatestTokenBlock(nts[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		bid, err := blk.GetBlockID(nts[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		totalAmount = totalAmount + nts[i].TokenValue
		ti := contract.TokenInfo{
			Token:      nts[i].TokenID,
			TokenType:  tt,
			TokenValue: nts[i].TokenValue,
			OwnerDID:   nts[i].DID,
			BlockID:    bid,
		}
		tis = append(tis, ti)
	}
	sct := &contract.ContractType{
		Type:       contract.SCNFTSaleContractType,
		PledgeMode: contract.POWPledgeMode,
		TotalRBTs:  totalAmount,
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			Comment:     "Putting sale contract for NFTs",
			TransTokens: tis,
		},
		ReqID: reqID,
	}
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	cr := &ConensusRequest{
		ReqID:         uuid.New().String(),
		Type:          sr.Type,
		SenderPeerID:  c.peerID,
		ContractBlock: sc.GetBlock(),
	}
	_, _, err = c.initiateConsensus(cr, sc, dc)
	c.log.Info("NFTs sale contract added successfully")
	resp.Status = true
	msg := fmt.Sprintf("NFTs sale contract added successfully")
	resp.Message = msg
	return resp
}
