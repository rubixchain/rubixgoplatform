package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
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

func (c *Core) MigrateNode(reqID string, m *MigrateRequest, didDir string) error {
	rubixDir := "~/Rubix/"
	if runtime.GOOS == "windows" {
		rubixDir = "C:/Rubix/"
	}
	rb, err := ioutil.ReadFile(rubixDir + "DATA/DID.json")
	if err != nil {
		c.log.Error("Failed to migrate, invalid file", "err", err)
		return fmt.Errorf("Unable to find DID.json file")
	}
	var d []DIDJson
	err = json.Unmarshal(rb, &d)
	if err != nil {
		c.log.Error("Failed to migrate, invalid parsing", "err", err)
		return fmt.Errorf("Invalid DID.json file, unable to parse")
	}
	c.log.Debug("Node DID: " + d[0].DID)
	didCreate := did.DIDCreate{
		Dir:            didDir,
		Type:           m.DIDType,
		PrivPWD:        m.PrivPWD,
		QuorumPWD:      m.QuorumPWD,
		DIDImgFileName: rubixDir + "DATA/" + d[0].DID + "/DID.png",
		PubImgFile:     rubixDir + "DATA/" + d[0].DID + "/PublicShare.png",
	}

	_, err = os.Stat(didCreate.DIDImgFileName)
	if err != nil {
		c.log.Error("Failed to migrate, missing DID.png file", "err", err)
		return fmt.Errorf("Failed to migrate, missing DID.png file")
	}
	_, err = os.Stat(didCreate.PubImgFile)
	if err != nil {
		c.log.Error("Failed to migrate, missing PublicShare.png file", "err", err)
		return fmt.Errorf("Failed to migrate, missing PublicShare.png file")
	}
	did, err := c.d.MigrateDID(&didCreate)
	if err != nil {
		c.log.Error("Failed to migrate, failed in creation of new DID address", "err", err, "msg", did)
		return fmt.Errorf("Failed to migrate, failed in creation of new DID address")
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
		return fmt.Errorf("Failed to create did in the wallet")
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		c.log.Error("Failed to setup did crypto", "err", err)
		return fmt.Errorf("Failed to setup did crypto")
	}

	tokens, err := util.GetAllFiles(rubixDir + "Wallet/TOKENS/")
	if err != nil {
		c.log.Error("Failed to migrate, failed to read token files", "err", err)
		return fmt.Errorf("Failed to migrate, failed to read token files")
	}
	for _, t := range tokens {
		tb, err := os.Open(rubixDir + "Wallet/TOKENS/" + t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read token files", "err", err)
			return fmt.Errorf("Failed to migrate, failed to read token files")
		}
		tid, err := c.ipfs.Add(tb)
		if err != nil {
			c.log.Error("Failed to migrate, failed to add token file", "err", err)
			return fmt.Errorf("Failed to migrate, failed to add token file")
		}
		if t != tid {
			c.log.Error("Failed to migrate, token hash is not matching", "err", err)
			return fmt.Errorf("Failed to migrate, token hash is not matching")
		}

		fb, err := os.Open(rubixDir + "Wallet/TOKENCHAINS/" + t + ".json")
		if err != nil {
			c.log.Error("Failed to migrate, failed to read token chain files", "err", err)
			return fmt.Errorf("Failed to migrate, failed to read token chain files")
		}
		tcid, err := c.ipfs.Add(fb)
		if err != nil {
			c.log.Error("Failed to migrate, failed to add token chain file", "err", err)
			return fmt.Errorf("Failed to migrate, failed to add token chain file")
		}
		ctcb := make(map[string]*block.Block)
		ctcb[t] = nil
		ntcb := &block.TokenChainBlock{
			BlockType:       block.TokenBlockType,
			TransactionType: block.TokenMigratedType,
			MigratedBlockID: tcid,
			TokenID:         t,
			TokenOwner:      did,
			Comment:         "Token migrated at : " + time.Now().String(),
		}
		//ctcb := make
		blk := block.CreateNewBlock(ctcb, ntcb)
		if blk == nil {
			c.log.Error("Failed to migrate, failed to create new token chain block")
			return fmt.Errorf("Failed to migrate, failed to create new token chain block")
		}
		h, err := blk.GetHash()
		if err != nil {
			c.log.Error("Failed to migrate, failed to get hash", "err", err)
			return fmt.Errorf("Failed to migrate, failed to get hash")
		}
		sb, err := dc.PvtSign([]byte(h))
		if err != nil {
			c.log.Error("Failed to migrate, failed to get did signature", "err", err)
			return fmt.Errorf("Failed to migrate, failed to get did signature")
		}
		err = blk.UpdateSignature(did, util.HexToStr(sb))
		if err != nil {
			c.log.Error("Failed to migrate, failed to update did signature", "err", err)
			return fmt.Errorf("Failed to migrate, failed to update did signature")
		}

		tk, err := ioutil.ReadFile(rubixDir + "Wallet/TOKENS/" + t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read token files", "err", err)
			return fmt.Errorf("Failed to migrate, failed to read token files")
		}
		bid, err := blk.GetBlockID(t)
		if err != nil {
			c.log.Error("Failed to migrate, failed to get block id", "err", err)
			return fmt.Errorf("Failed to migrate, failed to get block id")
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
			return fmt.Errorf("Failed to migrate, failed to add token chain block")
		}
		err = c.w.CreateToken(tkn)
		if err != nil {
			c.log.Error("Failed to migrate, failed to add token to wallet", "err", err)
			return fmt.Errorf("Failed to migrate, failed to add token to wallet")
		}
	}
	c.log.Debug("Number of tokens", "tokens", len(tokens))
	c.log.Info("did")
	return nil
}
