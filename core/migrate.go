package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	BatchSize int = 1000
)

type MigrateRequest struct {
	DIDType   int    `json:"did_type"`
	PrivPWD   string `json:"priv_pwd"`
	QuorumPWD string `json:"quorum_pwd"`
}

type DIDJson struct {
	PeerID string `json:"peerid"`
	DID    string `json:"didHash"`
	Wallet string `json:"walletHash"`
}

func (c *Core) removeDIDMap(peerID string) {
	// curl --location --request DELETE '13.76.134.226:9090/remove/<did>'
	ec, err := ensweb.NewClient(&config.Config{ServerAddress: "13.76.134.226", ServerPort: "9090", Production: "false"}, c.log)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	req, err := ec.JSONRequest("DELETE", "/remove/"+peerID, nil)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	resp, err := ec.Do(req)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Error("Failed to remove old did map", "status", resp.StatusCode)
		return
	}
	c.log.Info("Removed old did & peer map")
}

func (c *Core) IsArbitaryMode() bool {
	return c.arbitaryMode
}

func (c *Core) LockTokens(ts []string) *model.BasicResponse {
	br := &model.BasicResponse{
		Status: false,
	}
	err := c.srv.AddLockedTokens(ts)
	if err != nil {
		br.Message = "Failed to lock tokens, " + err.Error()
		return br
	}
	br.Status = true
	br.Message = "All tokens are locked successfully"
	return br
}

func (c *Core) MigrateNode(reqID string, m *MigrateRequest, didDir string) {
	err := c.migrateNode(reqID, m, didDir)
	br := model.BasicResponse{
		Status:  true,
		Message: "DID migrated successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) migrateNode(reqID string, m *MigrateRequest, didDir string) error {
	rubixDir := os.Getenv("HOME") + "/Rubix/"
	if runtime.GOOS == "windows" {
		rubixDir = "C:/Rubix/"
	}
	rb, err := ioutil.ReadFile(rubixDir + "DATA/DID.json")
	if err != nil {
		c.log.Error("Failed to migrate, invalid file", "err", err)
		return fmt.Errorf("unable to find DID.json file")
	}
	var d []DIDJson
	err = json.Unmarshal(rb, &d)
	if err != nil {
		c.log.Error("Failed to migrate, invalid parsing", "err", err)
		return fmt.Errorf("invalid DID.json file, unable to parse")
	}
	c.log.Info("Node DID: " + d[0].DID)
	didCreate := did.DIDCreate{
		Dir:            didDir,
		Type:           m.DIDType,
		PrivPWD:        m.PrivPWD,
		QuorumPWD:      m.QuorumPWD,
		DIDImgFileName: rubixDir + "DATA/" + d[0].DID + "/DID.png",
		PubImgFile:     rubixDir + "DATA/" + d[0].DID + "/PublicShare.png",
		PrivImgFile:    rubixDir + "DATA/" + d[0].DID + "/PrivateShare.png",
	}

	_, err = os.Stat(didCreate.DIDImgFileName)
	if err != nil {
		c.log.Error("Failed to migrate, missing DID.png file", "err", err)
		return fmt.Errorf("failed to migrate, missing DID.png file")
	}
	_, err = os.Stat(didCreate.PubImgFile)
	if err != nil {
		c.log.Error("Failed to migrate, missing PublicShare.png file", "err", err)
		return fmt.Errorf("failed to migrate, missing PublicShare.png file")
	}
	did, err := c.d.MigrateDID(&didCreate)
	if err != nil {
		c.log.Error("Failed to migrate, failed in creation of new DID address", "err", err, "msg", did)
		return fmt.Errorf("failed to migrate, failed in creation of new DID address")
	}

	dt := wallet.DIDType{
		DID:    did,
		DIDDir: didCreate.Dir,
		Type:   didCreate.Type,
		Config: didCreate.Config,
	}

	err = c.w.CreateDID(&dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		return fmt.Errorf("failed to create did in the wallet")
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		c.log.Error("Failed to setup did crypto", "err", err)
		return fmt.Errorf("failed to setup did crypto")
	}

	tokens, err := util.GetAllFiles(rubixDir + "Wallet/TOKENS/")
	if err != nil {
		c.log.Error("Failed to migrate, failed to read token files", "err", err)
		return fmt.Errorf("failed to migrate, failed to read token files")
	}
	h := sha256.New()
	h.Write([]byte(d[0].DID))
	ha := h.Sum(nil)
	addr := int(ha[0]) % len(c.arbitaryAddr)
	c.log.Info("Conneting to node : " + c.arbitaryAddr[addr])
	p, err := c.getPeer(c.arbitaryAddr[addr])
	if err != nil {
		c.log.Error("Failed to migrate, failed to connect arbitary peer", "err", err, "peer", c.arbitaryAddr[addr])
		return fmt.Errorf("failed to migrate, failed to connect arbitary peer")
	}
	defer p.Close()
	if !c.checkDIDMigrated(p, d[0].DID) {
		c.log.Error("Failed to migrate, unable to migrate did")
		return fmt.Errorf("failed to migrate, unable to migrate did")
	}
	st := time.Now()
	numTokens := len(tokens)
	index := 0
	invalidTokens := make([]string, 0)
	for {
		tis := make([]contract.TokenInfo, 0)
		gtis := make([]block.GenesisTokenInfo, 0)
		tts := make([]block.TransTokens, 0)
		batchIndex := 0
		stime := time.Now()
		c.log.Info("Starting the batch")
		for {
			t := tokens[index]
			tk, err := ioutil.ReadFile(rubixDir + "Wallet/TOKENS/" + t)
			if err != nil {
				c.log.Error("Failed to migrate, failed to read token files", "err", err)
				return fmt.Errorf("failed to migrate, failed to read token files")
			}
			tl, tn, err := token.ValidateWholeToken(string(tk))
			if err != nil {
				c.log.Info("Invalid token skipping : " + t)
				invalidTokens = append(invalidTokens, t)
				index++
				continue
			}
			tb, err := os.Open(rubixDir + "Wallet/TOKENS/" + t)
			if err != nil {
				c.log.Error("Failed to migrate, failed to read token files", "err", err)
				return fmt.Errorf("failed to migrate, failed to read token files")
			}
			tid, err := c.ipfs.Add(tb, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				c.log.Error("Failed to migrate, failed to add token file", "err", err)
				return fmt.Errorf("failed to migrate, failed to add token file")
			}
			if t != tid {
				c.log.Error("Failed to migrate, token hash is not matching", "err", err)
				return fmt.Errorf("failed to migrate, token hash is not matching")
			}
			fb, err := os.Open(rubixDir + "Wallet/TOKENCHAINS/" + t + ".json")
			if err != nil {
				c.log.Error("Failed to migrate, failed to read token chain files", "err", err)
				return fmt.Errorf("failed to migrate, failed to read token chain files")
			}
			tcid, err := c.ipfs.Add(fb)
			if err != nil {
				c.log.Error("Failed to migrate, failed to add token chain file", "err", err)
				return fmt.Errorf("failed to migrate, failed to add token chain file")
			}
			gti := block.GenesisTokenInfo{
				Token:           t,
				TokenLevel:      tl,
				TokenNumber:     tn,
				MigratedBlockID: tcid,
			}
			ti := contract.TokenInfo{
				Token:     t,
				TokenType: token.RBTTokenType,
				OwnerDID:  did,
			}
			tt := block.TransTokens{
				Token:     t,
				TokenType: token.RBTTokenType,
			}
			gtis = append(gtis, gti)
			tis = append(tis, ti)
			tts = append(tts, tt)
			index++
			batchIndex++
			if batchIndex == BatchSize || index == numTokens {
				break
			}
		}
		etime := time.Now()
		dtime := etime.Sub(stime)
		c.log.Info("Starting the signature", "duration", dtime)
		stime = time.Now()
		ts := &contract.TransInfo{
			Comment:     "Migrating Token at : " + time.Now().String(),
			TransTokens: tis,
		}
		st := &contract.ContractType{
			Type:      contract.SCDIDMigrateType,
			TransInfo: ts,
		}
		sc := contract.CreateNewContract(st)
		err = sc.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to migrate, failed to update signature", "err", err)
			return fmt.Errorf("failed to migrate, failed to update signature")
		}
		dtime = etime.Sub(stime)
		c.log.Info("Signature done", "duration", dtime)
		gb := &block.GenesisBlock{
			Type: block.TokenMigratedType,
			Info: gtis,
		}
		ctcb := make(map[string]*block.Block)
		ntcb := &block.TokenChainBlock{
			TokenType:       token.RBTTokenType,
			TransactionType: block.TokenMigratedType,
			TokenOwner:      did,
			GenesisBlock:    gb,
			SmartContract:   sc.GetBlock(),
			TransInfo: &block.TransInfo{
				Tokens: tts,
			},
		}
		//ctcb := make
		blk := block.CreateNewBlock(ctcb, ntcb)
		if blk == nil {
			c.log.Error("Failed to migrate, failed to create new token chain block")
			return fmt.Errorf("failed to migrate, failed to create new token chain block")
		}
		sr := &SignatureRequest{
			TokenChainBlock: blk.GetBlock(),
		}
		sig, ok := c.getArbitrationSignature(p, sr)
		if !ok {
			c.log.Error("Failed to migrate, failed to get signature")
			return fmt.Errorf("failed to migrate, failed to get signature")
		}
		err = blk.ReplaceSignature(p.GetPeerDID(), sig)
		if err != nil {
			c.log.Error("Failed to migrate, failed to update arbitary signature")
			return fmt.Errorf("failed to migrate, failed to update arbitary signature")
		}
		for _, ti := range tis {
			t := ti.Token
			tkn := &wallet.Token{
				TokenID:     t,
				DID:         did,
				TokenValue:  1,
				TokenStatus: wallet.TokenIsFree,
			}
			err = c.w.AddTokenBlock(t, blk)
			if err != nil {
				c.log.Error("Failed to migrate, failed to add token chain block", "err", err)
				return fmt.Errorf("failed to migrate, failed to add token chain block")
			}
			err = c.w.CreateToken(tkn)
			if err != nil {
				c.log.Error("Failed to migrate, failed to add token to wallet", "err", err)
				return fmt.Errorf("failed to migrate, failed to add token to wallet")
			}
		}
		if index >= numTokens {
			break
		}
	}
	if len(invalidTokens) > 0 {
		fp, err := os.Open("invalidtokens.txt")
		if err == nil {
			for i := range invalidTokens {
				fp.WriteString(invalidTokens[i])
			}
			fp.Close()
		}
	}
	creditFiles, err := util.GetAllFiles(rubixDir + "Wallet/WALLET_DATA/Credits/")
	if err != nil {
		c.log.Error("Failed to migrate, failed to read credit files", "err", err)
		return fmt.Errorf("failed to migrate, failed to credit token files")
	}
	for _, cf := range creditFiles {
		cb, err := ioutil.ReadFile(rubixDir + "Wallet/WALLET_DATA/Credits/" + cf)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read credit file", "err", err)
			return fmt.Errorf("failed to migrate, failed to credit token file")
		}
		var cs []CreditSignature
		err = json.Unmarshal(cb, &cs)
		if err != nil {
			c.log.Error("Failed to migrate, failed to parse credit file", "err", err)
			return fmt.Errorf("failed to migrate, failed to parse credit file")
		}
		var ncs CreditScore
		ncs.Credit = make([]CreditSignature, 0)
		for _, s := range cs {
			sig := util.ConvertBitString(s.Signature)
			if sig == nil {
				c.log.Error("Failed to migrate, failed to parse credit signature")
				return fmt.Errorf("failed to migrate, failed to parse credit signature")
			}
			ns := CreditSignature{
				DID:       s.DID,
				Hash:      s.Hash,
				Signature: util.HexToStr(sig),
			}
			ncs.Credit = append(ncs.Credit, ns)
		}
		if len(ncs.Credit) > 0 {
			jb, err := json.Marshal(&ncs)
			if err != nil {
				c.log.Error("Failed to migrate, failed to marshal credit", "err", err)
				return fmt.Errorf("failed to migrate, failed to marshal credit")
			}
			err = c.w.StoreCredit(did, base64.StdEncoding.EncodeToString(jb))
			if err != nil {
				c.log.Error("Failed to migrate, failed to store credit", "err", err)
				return fmt.Errorf("failed to migrate, failed to store credit")
			}
		}
	}
	if !c.mapMigratedDID(p, d[0].DID, did) {
		c.log.Error("Failed to migrate, failed to store credit", "err", err)
		return fmt.Errorf("failed to migrate, failed to store credit")
	}
	for i := 0; i < numTokens; i++ {
		t := tokens[i]
		tb, err := os.Open(rubixDir + "Wallet/TOKENS/" + t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read token files", "err", err)
			return fmt.Errorf("failed to migrate, failed to read token files")
		}
		_, err = c.ipfs.Add(tb)
		if err != nil {
			c.log.Error("Failed to migrate, failed to add token file", "err", err)
			return fmt.Errorf("failed to migrate, failed to add token file")
		}
		ok, err := c.w.Pin(t, wallet.OwnerRole, did)
		if err != nil || !ok {
			c.log.Error("Failed to migrate, failed to pin token", "err", err)
			return fmt.Errorf("failed to migrate, failed to pin token")
		}
	}
	et := time.Now()
	dif := et.Sub(st)
	c.log.Info("Tokens signatures completed", "duration", dif)
	st = time.Now()
	c.removeDIDMap(d[0].PeerID)
	c.ec.ExplorerMapDID(d[0].DID, did, c.peerID)
	dif = et.Sub(st)
	c.log.Info("Tokens migration completed", "duration", dif)
	c.log.Info(fmt.Sprintf("Old DID=%s migrated to New DID=%s", d[0].DID, did))
	c.log.Info(fmt.Sprintf("Number of tokens migrated =%d", len(tokens)))
	c.log.Info(fmt.Sprintf("Number of credits migrated =%d", len(creditFiles)))
	c.log.Info("Migration done successfully")
	return nil
}
