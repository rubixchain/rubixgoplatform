package core

import (
	"fmt"
	"os"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/token"
)

func (c *Core) Unpledge(t string, file string) error {
	tokenType := token.RBTTokenType
	if c.testNet {
		tokenType = token.TestTokenType
	}
	b := c.w.GetLatestTokenBlock(t, tokenType)
	if b == nil {
		c.log.Error("Failed to unpledge invalid tokne chain block")
		return fmt.Errorf("Failed to unpledge invalid tokne chain block")
	}
	f, err := os.Open(file)
	if err != nil {
		c.log.Error("Failed to unpledge, unable to open file", "err", err)
		return err
	}
	id, err := c.ipfs.Add(f)
	if err != nil {
		f.Close()
		c.log.Error("Failed to add file to ipfs", "err", err)
		return err
	}
	f.Close()
	os.Remove(file)
	ctcb := make(map[string]*block.Block)
	tsb := make([]block.TransTokens, 0)

	ts := block.TransTokens{
		Token:      t,
		TokenType:  tokenType,
		UnplededID: id,
	}
	did := b.GetOwner()
	dc, ok := c.qc[did]
	if !ok {
		c.log.Error("Failed to get quorum did crypto")
		return fmt.Errorf("failed to get quorum did crypto")
	}
	tsb = append(tsb, ts)
	ctcb[t] = b
	tcb := block.TokenChainBlock{
		TransactionType: block.TokenUnpledgedType,
		TokenOwner:      did,
		TransInfo: &block.TransInfo{
			Comment: "Token is un pledged at " + time.Now().String(),
			Tokens:  tsb,
		},
	}
	nb := block.CreateNewBlock(ctcb, &tcb)
	if nb == nil {
		c.log.Error("Failed to create new token chain block")
		return fmt.Errorf("failed to create new token chain block")
	}
	err = nb.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update the signature", "err", err)
		return fmt.Errorf("failed to update the signature")
	}
	err = c.w.CreateTokenBlock(nb)
	if err != nil {
		c.log.Error("Failed to update token chain block", "err", err)
		return err
	}
	err = c.w.UnpledgeWholeToken(did, t, tokenType)
	if err != nil {
		c.log.Error("Failed to update un pledge token", "err", err)
		return err
	}
	return nil
}
