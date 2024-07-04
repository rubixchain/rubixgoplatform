package core

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) createFungibleToken(dc did.DIDCrypto, did string, tkns []string, ftNum int, ftName string) ([]wallet.Token, error) {
	if dc == nil {
		return nil, fmt.Errorf("did crypto is not initialised")
	}

	fts := make([]string, ftNum)
	var gp []string
	var totalValue float64

	for _, tkn := range tkns {
		t, err := c.w.GetToken(tkn, wallet.TokenIsFree)
		if err != nil || t == nil {
			return nil, fmt.Errorf("failed to get token %s or token does not exist", tkn)
		}
		totalValue += t.TokenValue

		b := c.w.GetGenesisTokenBlock(tkn, c.TokenType(RBTString))
		p, gp2, err := b.GetParentDetials(tkn)
		if err != nil {
			c.log.Error("failed to get parent detials for token", "token", tkn, "err", err)
			return nil, err
		}
		if gp2 == nil {
			gp2 = make([]string, 0)
		}
		if p != "" {
			gp2 = append(gp2, p)
		}
		gp = append(gp, gp2...)

		t.TokenStatus = wallet.TokenIsBurnt
		err = c.w.UpdateToken(t)
		if err != nil {
			c.log.Error("FT creation failed, failed to update token status for token", "token", tkn, "err", err)
			return nil, err
		}
	}

	for i := 0; i < ftNum; i++ {
		rt := &rac.RacType{
			Type:        c.RACFTType(),
			DID:         did,
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
			FTSymbol:    fmt.Sprintf("%s_%d", ftName, i),
			FTInfo: &rac.RacFTInfo{
				Parents: strings.Join(tkns, ","),
				FTNum:   i,
				FTName:  ftName,
			},
		}

		rb, err := rac.CreateRac(rt)
		if err != nil {
			c.log.Error("failed to create rac block", "err", err)
			return nil, err
		}
		// expect one block
		if len(rb) != 1 {
			return nil, fmt.Errorf("failed to create rac block")
		}
		err = rb[0].UpdateSignature(dc)
		if err != nil {
			c.log.Error("failed to update did signature", "err", err)
			return nil, err
		}
		rtb := rb[0].GetBlock()
		td := util.HexToStr(rtb)
		fr := bytes.NewBuffer([]byte(td))
		pt, err := c.w.Add(fr, did, wallet.AddFunc)
		if err != nil {
			c.log.Error("Failed to create FT, failed to add rac token to ipfs", "err", err)
			return nil, err
		}
		fts[i] = pt
		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     pt,
					TokenType: c.TokenType(FTString),
				},
			},
			Comment: fmt.Sprintf("FT %s generated at : %s", rt.FTSymbol, time.Now().String()),
		}
		tcb := &block.TokenChainBlock{
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      did,
			TransInfo:       bti,
			GenesisBlock: &block.GenesisBlock{
				Info: []block.GenesisTokenInfo{
					{
						Token:         pt,
						ParentID:      strings.Join(tkns, ","),
						GrandParentID: gp,
					},
				},
			},
			TokenValue: 1,
		}
		ctcb := make(map[string]*block.Block)
		ctcb[pt] = nil
		b := block.CreateNewBlock(ctcb, tcb)
		if b == nil {
			return nil, fmt.Errorf("failed to create new block")
		}
		err = b.UpdateSignature(dc)
		if err != nil {
			c.log.Error("FT creation failed, failed to update signature", "err", err)
			return nil, err
		}
		err = c.w.AddTokenBlock(pt, b)
		if err != nil {
			c.log.Error("Failed to create FT, failed to add token chan block", "err", err)
			return nil, err
		}
	}

	bti := &block.TransInfo{
		Tokens:  make([]block.TransTokens, 0, len(tkns)),
		Comment: "Tokens burnt at : " + time.Now().String(),
	}
	for _, tkn := range tkns {
		bti.Tokens = append(bti.Tokens, block.TransTokens{
			Token:     tkn,
			TokenType: c.TokenType(RBTString),
		})
	}
	tcb := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner:      did,
		TransInfo:       bti,
		TokenValue:      totalValue,
		ChildTokens:     fts,
	}
	ctcb := make(map[string]*block.Block)
	for _, tkn := range tkns {
		ctcb[tkn] = c.w.GetLatestTokenBlock(tkn, c.TokenType(RBTString))
	}
	b := block.CreateNewBlock(ctcb, tcb)
	if b == nil {
		return nil, fmt.Errorf("failed to create new block")
	}
	err := b.UpdateSignature(dc)
	if err != nil {
		c.log.Error("FT creation failed, failed to update signature", "err", err)
		return nil, err
	}
	for _, tkn := range tkns {
		err = c.w.AddTokenBlock(tkn, b)
		if err != nil {
			c.log.Error("FT creation failed, failed to add token block", "token", tkn, "err", err)
			return nil, err
		}
	}

	nft := make([]wallet.Token, 0)
	for i := range fts {
		ptkn := &wallet.Token{
			TokenID:       fts[i],
			ParentTokenID: strings.Join(tkns, ","),
			TokenValue:    1,
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
		}
		err = c.w.CreateToken(ptkn)
		if err != nil {
			c.log.Error("FT creation failed, failed to create token", "err", err)
			return nil, err
		}
		nft = append(nft, *ptkn)
	}

	return nft, nil
}
