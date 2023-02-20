package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
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

func (c *Core) removeDIDMap(did string) {
	// curl --location --request DELETE '13.76.134.226:9090/remove/<did>'
	ec, err := ensweb.NewClient(&config.Config{ServerAddress: "13.76.134.226", ServerPort: "9090", Production: "false"}, c.log)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	req, err := ec.JSONRequest("DELETE", "remove/"+did, nil)
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
	dc.OutChan <- br
}

func (c *Core) migrateNode(reqID string, m *MigrateRequest, didDir string) error {
	rubixDir := "~/Rubix/"
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
	c.log.Debug("Node DID: " + d[0].DID)
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

	c.removeDIDMap(d[0].DID)

	c.ec.ExplorerMapDID(d[0].DID, did)

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
	var wg sync.WaitGroup
	as := make([]ArbitaryStatus, len(c.arbitaryAddr))
	for i, a := range c.arbitaryAddr {
		wg.Add(1)
		go func(idx int, addr string) {
			defer wg.Done()
			p, err := c.getPeer(addr)
			if err != nil {
				return
			}
			as[idx].p = p
		}(i, a)
	}
	wg.Wait()
	for i, a := range c.arbitaryAddr {
		if as[i].p == nil {
			c.log.Error("Failed to migrate, failed to connect arbitary peer", "err", err, "peer", a)
			return fmt.Errorf("failed to migrate, failed to connect arbitary peer")
		}
	}
	defer func() {
		for i := range c.arbitaryAddr {
			as[i].p.Close()
		}
	}()
	for _, t := range tokens {
		tk, err := ioutil.ReadFile(rubixDir + "Wallet/TOKENS/" + t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read token files", "err", err)
			return fmt.Errorf("failed to migrate, failed to read token files")
		}
		tl, tn, err := token.ValidateWholeToken(string(tk))
		if err != nil {
			c.log.Error("Failed to migrate, invalid token", "err", err)
			return fmt.Errorf("failed to migrate, invalid token")
		}
		tb, err := os.Open(rubixDir + "Wallet/TOKENS/" + t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read token files", "err", err)
			return fmt.Errorf("failed to migrate, failed to read token files")
		}
		tid, err := c.ipfs.Add(tb)
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
		st := &contract.ContractType{
			Type:            contract.SCDIDMigrateType,
			OwnerDID:        did,
			MigratedToken:   t,
			MigratedTokenID: tcid,
			Comment:         "Migrating Token",
		}
		sc := contract.CreateNewContract(st)
		err = sc.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to migrate, failed to update signature", "err", err)
			return fmt.Errorf("failed to migrate, failed to update signature")
		}
		ctcb := make(map[string]*block.Block)
		ctcb[t] = nil
		ntcb := &block.TokenChainBlock{
			BlockType:       block.TokenBlockType,
			TokenLevel:      tl,
			TokenNumber:     tn,
			TransactionType: block.TokenMigratedType,
			MigratedBlockID: tcid,
			TokenID:         t,
			TokenOwner:      did,
			Contract:        sc.GetBlock(),
			Comment:         "Token migrated at : " + time.Now().String(),
		}
		c.log.Info("Block map", "tl", tl, "tn", tn)
		//ctcb := make
		blk := block.CreateNewBlock(ctcb, ntcb)
		if blk == nil {
			c.log.Error("Failed to migrate, failed to create new token chain block")
			return fmt.Errorf("failed to migrate, failed to create new token chain block")
		}
		sr := &SignatureRequest{
			TokenChainBlock: blk.GetBlock(),
		}
		for i := range c.arbitaryAddr {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				c.getArbitrationSignature(&as[idx], sr)
			}(i)
		}
		wg.Wait()
		for i, a := range c.arbitaryAddr {
			if !as[i].status {
				c.log.Error("Failed to migrate, failed to get arbitary signature", "addr", a)
				return fmt.Errorf("failed to migrate, failed to get arbitary signature")
			}
			err = blk.ReplaceSignature(as[i].p.GetPeerDID(), as[i].sig)
			if err != nil {
				c.log.Error("Failed to migrate, failed to update arbitary signature", "addr", a)
				return fmt.Errorf("failed to migrate, failed to update arbitary signature")
			}
		}
		bid, err := blk.GetBlockID(t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to get block id", "err", err)
			return fmt.Errorf("failed to migrate, failed to get block id")
		}
		tkn := &wallet.Token{
			TokenID:      t,
			TokenDetails: string(tk),
			DID:          did,
			TokenChainID: bid,
			TokenStatus:  wallet.TokenIsFree,
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
		ok, err := c.w.Pin(t, wallet.Owner, did)
		if err != nil || !ok {
			c.log.Error("Failed to migrate, failed to pin token", "err", err)
			return fmt.Errorf("failed to migrate, failed to pin token")
		}
	}
	c.log.Debug("Number of tokens", "tokens", len(tokens))
	c.log.Info("did")
	return nil
}
