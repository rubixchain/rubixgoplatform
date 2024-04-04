package core

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	DTUserIDField        string = "UserID"
	DTUserInfoField      string = "UserInfo"
	DTFileInfoField      string = "FileInfo"
	DTFileHashField      string = "FileHash"
	DTFileURLField       string = "FileURL"
	DTFileTransInfoField string = "FileTransInfo"
	DTCommiterDIDField   string = "CommitterDID"
	DTBatchIDField       string = "BatchID"
)

type DataTokenReq struct {
	DID        string
	Fields     map[string][]string
	FileNames  []string
	FolderName string
}

func (c *Core) CreateDataToken(reqID string, dr *DataTokenReq) {
	c.log.Debug("Create data token")
	defer os.RemoveAll(dr.FolderName)
	br := c.createDataToken(reqID, dr)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to create data token, failed to get did channel")
		return
	}
	dc.OutChan <- br
}

func (c *Core) createDataToken(reqID string, dr *DataTokenReq) *model.BasicResponse {
	c.log.Debug("initating data token")

	defer os.RemoveAll(dr.FolderName)
	br := model.BasicResponse{
		Status: false,
	}
	userID, ok := dr.Fields[DTUserIDField]
	if !ok {
		c.log.Error("Failed to create data token, user ID missing")
		br.Message = "Failed to create data token, user ID missing"
		return &br
	}
	rt := rac.RacType{
		Type:        c.TokenType(DataString),
		DID:         dr.DID,
		TotalSupply: 1,
		CreatorID:   userID[0],
	}
	c.log.Debug("Creating data token", "rt", rt)
	c.log.Debug("DID names", "DID names", dr.DID)
	userInfo, ok := dr.Fields[DTUserInfoField]
	if ok {
		rt.CreatorInput = userInfo[0]
	}
	comDid := dr.DID
	cdid, ok := dr.Fields[DTCommiterDIDField]
	if ok {
		comDid = cdid[0]
	}
	bid := comDid
	bids, ok := dr.Fields[DTBatchIDField]
	if ok {
		bid = bids[0]
	}
	fileInfo, fok := dr.Fields[DTFileInfoField]
	if fok {
		var fi map[string]map[string]string
		err := json.Unmarshal([]byte(fileInfo[0]), &fi)
		if err != nil {
			c.log.Error("Failed to create data token, invalid file info")
			br.Message = "Failed to create data token, invalid file info"
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
	c.log.Debug("reqID is ", "reqID", reqID)
	c.log.Debug("dr.DID is ", "dr.DID", dr.DID)
	dc, err := c.SetupDID(reqID, dr.DID)
	if err != nil {
		c.log.Error("Failed to create data token, failed to setup did", "err", err)
		br.Message = "Failed to create data token, failed to setup did"
		return &br
	}
	for _, file := range dr.FileNames {
		fn := strings.TrimPrefix(file, dr.FolderName+"/")
		fb, err := ioutil.ReadFile(file)
		if err != nil {
			c.log.Error("Failed to create data token, failed to read file", "err", err)
			br.Message = "Failed to create data token, failed to read file"
			return &br
		}
		hb := util.CalculateHash(fb, "SHA3-256")
		fbr := bytes.NewBuffer(fb)
		fileUrl, err := c.ipfs.Add(fbr)
		if err != nil {
			c.log.Error("Failed to create data token, failed to add file to ipfs", "err", err)
			br.Message = "Failed to create data token, failed to add file to ipfs"
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
		c.log.Error("Failed to create data token, failed to create rac block", "err", err)
		br.Message = "Failed to create data token, failed to create rac block"
		return &br
	}
	err = dtb[0].UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to create data token, failed to update signature", "err", err)
		br.Message = "Failed to create data token, failed to update signature"
		return &br
	}
	rtb := dtb[0].GetBlock()
	td := util.HexToStr(rtb)
	fr := bytes.NewBuffer([]byte(td))
	dt, err := c.ipfs.Add(fr)
	if err != nil {
		c.log.Error("Failed to create data token, failed to add rac token to ipfs", "err", err)
		br.Message = "Failed to create data token, failed to add rac token to ipfs"
		return &br
	}
	err = c.w.CreateDataToken(&wallet.DataToken{TokenID: dt, DID: dr.DID, CommitterDID: comDid, BatchID: bid})
	if err != nil {
		c.log.Error("Failed to create data token, write failed", "err", err)
		br.Message = "Failed to create data token, write failed"
		return &br
	}
	dtm := make(map[string]interface{})
	dtm[dr.DID] = dt
	ti := contract.TokenInfo{
		Token:     dt,
		TokenType: c.TokenType(DataString),
		OwnerDID:  dr.DID,
	}
	tis := make([]contract.TokenInfo, 0)
	tis = append(tis, ti)
	ts := &contract.TransInfo{
		SenderDID:   comDid,
		Comment:     "created data token at : " + time.Now().String(),
		TransTokens: tis,
	}
	st := &contract.ContractType{
		Type:      contract.SCDataTokenType,
		TransInfo: ts,
		ReqID:     reqID,
	}
	sc := contract.CreateNewContract(st)
	if sc == nil {
		c.log.Error("Failed to create data token, failed to create smart contract", "err", err)
		br.Message = "Failed to create data token, failed to create smart contract"
		return &br
	}
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to create data token, smart contract signature failed", "err", err)
		br.Message = "Failed to create data token, smart contract signature failed"
		return &br
	}
	gb := &block.GenesisBlock{
		Type: block.TokenGeneratedType,
		Info: []block.GenesisTokenInfo{
			{Token: dt},
		},
	}
	bti := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     dt,
				TokenType: c.TokenType(DataString),
			},
		},
		Comment: "Data token generated at : " + time.Now().String(),
	}
	tcb := &block.TokenChainBlock{
		TransactionType: block.TokenGeneratedType,
		TokenOwner:      dr.DID,
		SmartContract:   sc.GetBlock(),
		GenesisBlock:    gb,
		TransInfo:       bti,
	}
	ctcb := make(map[string]*block.Block)
	ctcb[dt] = nil
	blk := block.CreateNewBlock(ctcb, tcb)
	if blk == nil {
		c.log.Error("Failed to create data token, unable to create token chain")
		br.Message = "Failed to create data token, unable to create token chain"
		return &br
	}
	err = blk.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to create data token, failed to update signature", "err", err)
		br.Message = "Failed to create data token, failed to update signature"
		return &br
	}
	err = c.w.AddTokenBlock(dt, blk)
	if err != nil {
		c.log.Error("Failed to create data token, failed to add token chan block", "err", err)
		br.Message = "Failed to create data token, failed to add token chan block"
		return &br
	}
	br.Status = true
	br.Message = dt
	return &br
}

func (c *Core) CommitDataToken(reqID string, did string, batchID string) {
	br := c.commitDataToken(reqID, did, batchID)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to create data token, failed to get did channel")
		return
	}
	dc.OutChan <- br
}

func (c *Core) finishDataCommit(br *model.BasicResponse, dts []wallet.DataToken) {
	if br.Status {
		c.w.CommitDataToken(dts)
	} else {
		c.w.ReleaseDataToken(dts)
	}
}

func (c *Core) commitDataToken(reqID string, did string, batchID string) *model.BasicResponse {
	st := time.Now()
	dt, err := c.w.GetDataToken(batchID)
	br := &model.BasicResponse{
		Status: false,
	}
	defer c.finishDataCommit(br, dt)
	if err != nil {
		c.log.Error("Commit data token failed, failed to get data token", "err", err)
		br.Message = "Commit data token failed, failed to get data token"
		return br
	}
	tsi := &contract.TransInfo{
		SenderDID:   did,
		TransTokens: make([]contract.TokenInfo, 0),
	}
	dts := make(map[string]string)
	for i := range dt {
		dts[dt[i].DID] = dt[i].TokenID
		ti := contract.TokenInfo{
			Token:     dt[i].TokenID,
			TokenType: token.DataTokenType,
			OwnerDID:  dt[i].DID,
		}
		tsi.TransTokens = append(tsi.TransTokens, ti)
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		br.Message = "Failed to setup DID, " + err.Error()
		return br
	}
	sct := &contract.ContractType{
		Type:       contract.SCDataTokenCommitType,
		PledgeMode: contract.POWPledgeMode,
		TransInfo:  tsi,
		ReqID:      reqID,
	}
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		br.Message = err.Error()
		return br
	}
	cr := &ConensusRequest{
		ReqID:         uuid.New().String(),
		Type:          QuorumTypeTwo,
		Mode:          DTCommitMode,
		SenderPeerID:  c.peerID,
		ContractBlock: sc.GetBlock(),
	}
	td, pl, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		br.Message = "Consensus failed" + err.Error()
		return br
	}
	et := time.Now()
	dif := et.Sub(st)

	etrans := &ExplorerDataTrans{
		TID:          td.TransactionID,
		CommitterDID: did,
		TrasnType:    2,
		DataTokens:   dts,
		QuorumList:   pl,
		TokenTime:    float64(dif.Milliseconds()),
	}
	c.ec.ExplorerDataTransaction(etrans)
	c.log.Debug("dts is ", "dts", dts)

	br.Status = true
	br.Message = "Data committed successfully"
	return br
}

func (c *Core) CheckDataToken(dt string) bool {
	err := c.ipfs.Get(dt, c.cfg.DirPath)
	if err != nil {
		c.log.Error("failed to get the data token")
		return false
	}
	rb, err := ioutil.ReadFile(c.cfg.DirPath + dt)
	if err != nil {
		c.log.Error("failed to read the data token file")
		return false
	}
	b, err := rac.InitRacBlock(rb, nil)
	if err != nil {
		c.log.Error("failed to read the data token file")
		return false
	}
	did := b.GetDID()
	dc, err := c.SetupForienDID(did)
	if err != nil {
		c.log.Error("failed to setup did crypto")
		return false
	}
	err = b.VerifySignature(dc)
	if err != nil {
		c.log.Error("failed to verify did signature", "err", err)
		return false
	}
	return true
}

func (c *Core) GetDataTokens(did string) []wallet.DataToken {
	dt, err := c.w.GetDataTokenByDID(did)
	if err != nil {
		c.log.Error("failed to get data tokens", "err", err)
		return nil
	}
	return dt
}
