package core

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) relaseToken(release *bool, token string) {
	if *release {
		c.w.ReleaseToken(token)
	}
}
func MinDecimalValue(num int) float64 {
	return math.Pow(10, float64(-num))
}
func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}
func Ceilround(num float64) int {
	return int(math.Ceil(num))
}
func floatPrecision(num float64, precision int) float64 {
	precision = MaxDecimalPlaces
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}
func CeilfloatPrecision(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(Ceilround(num*output)) / output
}

func (c *Core) GetTokens(dc did.DIDCrypto, did string, value float64, trnxMode int) ([]wallet.Token, error) {
	// Get all possible whole tokens
	wholeValue := int(value)
	var err error
	fv := float64(wholeValue)
	rem := value - fv
	rem = floatPrecision(rem, MaxDecimalPlaces)
	remWhole := 0
	wt := make([]wallet.Token, 0)
	if wholeValue != 0 {
		wt, remWhole, err = c.w.GetWholeTokens(did, wholeValue, trnxMode)
		if err != nil {
			c.log.Error("failed to get token", "err", err)
			return nil, err
		}
		rem = rem + float64(remWhole)
	}
	if rem == 0 {
		return wt, nil
	}

	// After getting the whole tokens, if there is some required RBT value is left in decimals
	// get the tokens with value less than or equal to or equal to the remainder value
	pt, err := c.w.GetTokensByLimit(did, rem)
	if err != nil || len(pt) == 0 {
		if rem >= 1 {
			c.w.ReleaseTokens(wt)
			c.log.Error("failed to get part tokens", "err", err)
			return nil, fmt.Errorf("insufficient balance")
		}
		tt, err := c.w.GetCloserToken(did, rem)
		if err != nil {
			c.w.ReleaseTokens(wt)
			c.log.Error("failed to fetch whole token", "err", err)
			return nil, err
		}
		tkn := tt.TokenID
		c.w.ReleaseToken(tkn)
		parts := []float64{rem, floatPrecision(tt.TokenValue-rem, MaxDecimalPlaces)}
		nt, err := c.createPartToken(dc, did, tkn, parts, 1)
		if err != nil {
			c.w.ReleaseTokens(wt)
			c.log.Error("failed to create part tokens", "err", err)
			return nil, err
		}
		nt[0].TokenStatus = wallet.TokenIsLocked
		c.w.UpdateToken(&nt[0])
		wt = append(wt, nt[0])
		return wt, nil
	}
	if rem < 1 {

		for i := range pt {
			if pt[i].TokenValue == rem {
				wt = append(wt, pt[i])
				pt = append(pt[:i], pt[i+1:]...)
				c.w.ReleaseTokens(pt)
				return wt, nil
			}
		}
	}
	idx := make([]int, 0)
	rpt := make([]wallet.Token, 0)
	for i := range pt {
		if pt[i].TokenValue <= rem {
			wt = append(wt, pt[i])
			rem = floatPrecision(rem-pt[i].TokenValue, MaxDecimalPlaces)
			idx = append(idx, i)
		} else {
			rpt = append(rpt, pt[i])
		}
	}

	if rem == 0 {
		c.w.ReleaseTokens(rpt)
		return wt, nil
	}
	if len(rpt) > 0 {
		parts := []float64{rem, floatPrecision(rpt[0].TokenValue-rem, MaxDecimalPlaces)}
		c.w.ReleaseToken(rpt[0].TokenID)
		npt, err := c.createPartToken(dc, did, rpt[0].TokenID, parts, 2)
		if err != nil {
			c.w.ReleaseTokens(wt)
			c.w.ReleaseTokens(rpt)
			return nil, err
		}
		c.w.ReleaseTokens(rpt)
		npt[0].TokenStatus = wallet.TokenIsLocked
		c.w.UpdateToken(&npt[0])
		wt = append(wt, npt[0])
		return wt, nil
	}
	nwt, err := c.w.GetCloserToken(did, rem)
	if err != nil && err.Error() != "no records found" {
		c.w.ReleaseTokens(wt)
		c.log.Error("failed to get whole token", "err", err)
		return nil, fmt.Errorf("failed to get whole token")
	}
	if nwt == nil {
		c.w.ReleaseTokens(rpt)
		c.log.Debug("No More tokens left to pledge")
		return wt, nil
	}
	c.w.ReleaseToken(nwt.TokenID)
	parts := []float64{rem, floatPrecision(nwt.TokenValue-rem, MaxDecimalPlaces)}
	npt, err := c.createPartToken(dc, did, nwt.TokenID, parts, 3)
	if err != nil {
		c.w.ReleaseTokens(wt)
		c.w.ReleaseToken(nwt.TokenID)
		c.log.Error("failed to create part token", "err", err)
		return nil, fmt.Errorf("failed to create part token")
	}
	npt[0].TokenStatus = wallet.TokenIsLocked
	c.w.UpdateToken(&npt[0])
	wt = append(wt, npt[0])
	return wt, nil
}

func (c *Core) createMultiplePartTokens(dc did.DIDCrypto, did string, wholeTokenn wallet.Token, partTokensCount int, partTokenValue float64) ([]*wallet.Token, error) {
	defer c.w.ReleaseToken(wholeTokenn.TokenID)
	
	var partTokens []*wallet.Token = make([]*wallet.Token, 0)
	var partTokenIDs []string = make([]string, 0)

	if dc == nil {
		return nil, fmt.Errorf("did crypto is not initialised")
	}

	if wholeTokenn.TokenValue != 1.0 {
		return nil, fmt.Errorf("token %v is not a whole token, as its fetched value is %v", wholeTokenn, wholeTokenn.TokenValue)
	}

	wholeTokenTypeVal := c.TokenType(RBTString)

	wholeTokenGenesisBlock := c.w.GetGenesisTokenBlock(wholeTokenn.TokenID, wholeTokenTypeVal)
	if wholeTokenGenesisBlock == nil {
		return nil, fmt.Errorf("unable to fetch genesis block for whole token %v", wholeTokenn)
	}
	
	parentBlockID, grandParentBlockID, err := wholeTokenGenesisBlock.GetParentDetials(wholeTokenn.TokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent token details of %v, err: %v", wholeTokenn, err)
	}
	if grandParentBlockID == nil {
		grandParentBlockID = make([]string, 0)
	}
	if parentBlockID != "" {
		grandParentBlockID = append(grandParentBlockID, parentBlockID)
	}


	// Generate Part Tokens
	for i := 0; i < partTokensCount; i++ {
		racToken := &rac.RacType{
			Type:        c.RACPartTokenType(),
			DID:         did,
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
			PartInfo: &rac.RacPartInfo{
				Parent:  wholeTokenn.TokenID,
				PartNum: i,
				Value:   floatPrecision(partTokenValue, MaxDecimalPlaces),
			},
		}

		racTokenBlocks, err := rac.CreateRac(racToken)
		if err != nil {
			c.log.Error("failed to create rac block", "err", err)
			return nil, fmt.Errorf("failed to create rac block, err: %v", err)
		}

		if len(racTokenBlocks) != 1 {
			return nil, fmt.Errorf("failed to create rac block, RAC Block array to be 1, error occured for whole token %v", wholeTokenn)
		}
		racTokenBlock := racTokenBlocks[0]

		err = racTokenBlock.UpdateSignature(dc)
		if err != nil {
			return nil, fmt.Errorf("failed to update signature for whole token %v rac block, err: %v", wholeTokenn, err)
		}

		racTokenBlockBytes := racTokenBlock.GetBlock()
		racTokenBlockStr := util.HexToStr(racTokenBlockBytes)
		//racTokenBlockBuffer := bytes.NewBuffer([]byte(racTokenBlockStr))

		//partTokenID, err := c.w.Add(racTokenBlockBuffer, did, wallet.AddFunc)
		partTokenID, err := c.w.AddV2(racTokenBlockStr)
		if err != nil {
			return nil, fmt.Errorf("failed to create part token while adding RAC block to IPFS, err: %v", err)
		}
		partTokenIDs = append(partTokenIDs, partTokenID)

		partTokenTransactionInfo := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token: partTokenID,
					TokenType: c.TokenType(PartString),
				},
			},
		}

		partTokenChainBlock := &block.TokenChainBlock{
			TransactionType: block.TokenGeneratedType,
			TokenOwner: did,
			TransInfo: partTokenTransactionInfo,
			GenesisBlock: &block.GenesisBlock{
				Info: []block.GenesisTokenInfo{
					{
						Token: partTokenID,
						ParentID: wholeTokenn.TokenID,
						GrandParentID: grandParentBlockID,
					},
				},
			},
		}

		ctcb := make(map[string]*block.Block)
		ctcb[partTokenID] = nil 
		
		partTokenBlock := block.CreateNewBlock(ctcb, partTokenChainBlock)
		if partTokenBlock == nil {
			return nil, fmt.Errorf("failed to create new block, whole token: %v", wholeTokenn)
		}
		
		errSig := partTokenBlock.UpdateSignature(dc)
		if errSig != nil {
			return nil, fmt.Errorf("error while updating signature for part token, err: %v", errSig)
		}

		// TODO: remove the following WARN logger
		if (len(partTokenBlock.GetBlock()) > 1024) {
			c.log.Warn(fmt.Sprintf("Size of part token %v exceeds 1 Kb, its size is %v", partTokenID, len(partTokenBlock.GetBlock())))
		}

		errAddBlock := c.w.AddTokenBlock(partTokenID, partTokenBlock)
		if errAddBlock != nil {
			return nil, fmt.Errorf("Failed to add part token %v block, err: %v", partTokenID, errAddBlock)
		}
	}

	// Burn the whole token
	wholeTokenTransactionInfo := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token: wholeTokenn.TokenID,
				TokenType: wholeTokenTypeVal,
			},
		},
	}

	wholeTokenChainBlock := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner: did,
		TransInfo: wholeTokenTransactionInfo,
		TokenValue: floatPrecision(1.0, MaxDecimalPlaces),
		ChildTokens: partTokenIDs,
	}

	ctcb := make(map[string]*block.Block)
	ctcb[wholeTokenn.TokenID] = c.w.GetLatestTokenBlock(wholeTokenn.TokenID, wholeTokenTypeVal)
	updatedParentTokenBlock := block.CreateNewBlock(ctcb, wholeTokenChainBlock)
	if updatedParentTokenBlock == nil {
		return nil, fmt.Errorf("failed to update the whole token %v with new token block, err: %v", wholeTokenn, err)
	}

	errSig := updatedParentTokenBlock.UpdateSignature(dc)
	if errSig != nil {
		return nil, fmt.Errorf("error while singing updated token chain for whole token %v, err: %v", wholeTokenn, errSig)
	}

	errAddBlock := c.w.AddTokenBlock(wholeTokenn.TokenID, updatedParentTokenBlock)
	if errAddBlock != nil {
		return nil, fmt.Errorf("error while adding updated token block for whole token %v, err: %v", wholeTokenn, errAddBlock)
	}

	// Update SQL DB for part tokens
	for i := 0; i < partTokensCount; i++ {
		partToken := &wallet.Token{
			TokenID:       partTokenIDs[i],
			ParentTokenID: wholeTokenn.TokenID,
			TokenValue:    floatPrecision(partTokenValue, MaxDecimalPlaces),
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
		}

		err := c.w.CreateToken(partToken)
		if err != nil {
			return nil, fmt.Errorf("failed to add part token record, err: %v", err)
		}

		partTokens = append(partTokens, partToken)
	}

	// Update whole token in SQL DB to Burnt
	wholeTokenn.TokenStatus = wallet.TokenIsBurnt
	err = c.w.UpdateToken(&wholeTokenn)
	if err != nil {
		return nil, fmt.Errorf("failed to update whole token status to Burnt (%v), err: %v", wallet.TokenIsBurnt, err)
	}

	return partTokens, nil
}

func (c *Core) createPartToken(dc did.DIDCrypto, did string, tkn string, parts []float64, num int) ([]wallet.Token, error) {
	if dc == nil {
		return nil, fmt.Errorf("did crypto is not initialised")
	}
	t, err := c.w.GetToken(tkn, wallet.TokenIsFree)
	if err != nil || t == nil {
		return nil, fmt.Errorf("failed to get token or token does not exist")
	}
	release := true
	defer c.relaseToken(&release, tkn)
	ptts := RBTString
	if t.ParentTokenID != "" && t.TokenValue < 1 {
		ptts = PartString
	}
	ptt := c.TokenType(ptts)

	// check part split not crossing RBT
	amount := float64(0)
	for i := range parts {
		amount = amount + parts[i]
		amount = floatPrecision(amount, MaxDecimalPlaces)
		if amount > t.TokenValue {
			return nil, fmt.Errorf("invalid part split, split sum is more than the parent token -1")
		}
	}

	if amount != t.TokenValue {
		return nil, fmt.Errorf("invalid part split, sum of parts value not matching with parent token -2")
	}
	pts := make([]string, len(parts))
	b := c.w.GetGenesisTokenBlock(tkn, ptt)
	p, gp, err := b.GetParentDetials(tkn)
	if gp == nil {
		gp = make([]string, 0)
	}
	if p != "" {
		gp = append(gp, p)
	}
	if err != nil {
		c.log.Error("failed to get parent detials", "err", err)
		return nil, err
	}
	var ChildTokenList []ChildToken
	for i := range parts {
		rt := &rac.RacType{
			Type:        c.RACPartTokenType(),
			DID:         did,
			TotalSupply: 1,
			TimeStamp:   time.Now().String(),
			PartInfo: &rac.RacPartInfo{
				Parent:  tkn,
				PartNum: i,
				Value:   parts[i],
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
			c.log.Error("Failed to create part token, failed to add rac token to ipfs", "err", err)
			return nil, err
		}
		pts[i] = pt
		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     pt,
					TokenType: c.TokenType(PartString),
				},
			},
			Comment: "Part token generated at : " + time.Now().String(),
		}
		tcb := &block.TokenChainBlock{
			TransactionType: block.TokenGeneratedType,
			TokenOwner:      did,
			TransInfo:       bti,
			GenesisBlock: &block.GenesisBlock{
				Info: []block.GenesisTokenInfo{
					{
						Token:         pt,
						ParentID:      tkn,
						GrandParentID: gp,
					},
				},
			},
			TokenValue: floatPrecision(parts[i], MaxDecimalPlaces),
		}
		ctcb := make(map[string]*block.Block)
		ctcb[pt] = nil
		b := block.CreateNewBlock(ctcb, tcb)
		if b == nil {
			return nil, fmt.Errorf("failed to create new block")
		}
		err = b.UpdateSignature(dc)
		if err != nil {
			c.log.Error("part token creation failed, failed to update signature", "err", err)
			return nil, err
		}
		err = c.w.AddTokenBlock(pt, b)
		if err != nil {
			c.log.Error("Failed to create part token, failed to add token chan block", "err", err)
			return nil, err
		}
		ChildTokenList = append(ChildTokenList, ChildToken{ChildTokenID: pt, TokenValue: parts[i]})
	}
	newPartToken := &ExplorerCreateTokenParts{
		ChildTokenList: ChildTokenList,
		UserDID:        did,
		ParentToken:    tkn,
	}
	c.ec.ExplorerTokenCreateParts(newPartToken)
	bti := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     tkn,
				TokenType: ptt,
			},
		},
		Comment: "Token burnt at : " + time.Now().String(),
	}
	tcb := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner:      did,
		TransInfo:       bti,
		TokenValue:      floatPrecision(amount, MaxDecimalPlaces),
		ChildTokens:     pts,
	}
	ctcb := make(map[string]*block.Block)
	ctcb[tkn] = c.w.GetLatestTokenBlock(tkn, ptt)
	b = block.CreateNewBlock(ctcb, tcb)
	if b == nil {
		return nil, fmt.Errorf("failed to create new block")
	}
	err = b.UpdateSignature(dc)
	if err != nil {
		c.log.Error("part token creation failed, failed to update signature", "err", err)
		return nil, err
	}
	err = c.w.AddTokenBlock(tkn, b)
	if err != nil {
		c.log.Error("part token creation failed, failed to add token block", "err", err)
		return nil, err
	}
	npt := make([]wallet.Token, 0)
	for i := range parts {
		ptkn := &wallet.Token{
			TokenID:       pts[i],
			ParentTokenID: tkn,
			TokenValue:    parts[i],
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
		}
		err = c.w.CreateToken(ptkn)
		if err != nil {
			c.log.Error("part token creation failed, failed to create token", "err", err)
			return nil, err
		}
		npt = append(npt, *ptkn)
	}
	t.TokenStatus = wallet.TokenIsBurnt
	err = c.w.UpdateToken(t)
	if err != nil {
		c.log.Error("part token creation failed, failed to update token status", "err", err)
		return nil, err
	}
	release = false
	return npt, nil
}
