package wallet

import (
	"fmt"
	"os"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	TokenIsFree int = iota
	TokenIsLocked
	TokenIsPledged
	TokenIsUnPledged
	TokenIsTransferred
)

const (
	RACTestTokenType int = iota
	RACOldNFTType
	RACNFTType
)

type Token struct {
	TokenID       string  `gorm:"column:token_id;primaryKey"`
	ParentTokenID string  `gorm:"column:parent_token_id"`
	TokenValue    float64 `gorm:"column:token_value"`
	DID           string  `gorm:"column:did"`
	TokenStatus   int     `gorm:"column:token_status;"`
}

func (w *Wallet) CreateToken(t *Token) error {
	return w.s.Write(TokenStorage, t)
}

func (w *Wallet) PledgeWholeToken(did string, token string, b *block.Block) error {
	w.l.Lock()
	defer w.l.Unlock()
	var t Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, token)
	if err != nil {
		w.log.Error("Failed to get token", "token", token, "err", err)
		return err
	}

	if t.TokenStatus != TokenIsLocked {
		w.log.Error("Token is not locked")
		return fmt.Errorf("token is not locked")
	}
	t.TokenStatus = TokenIsPledged
	err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, token)
	if err != nil {
		w.log.Error("Failed to update token", "token", token, "err", err)
		return err
	}

	return nil
}

func (w *Wallet) GetAllWholeTokens(did string) ([]Token, error) {
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

func (w *Wallet) GetWholeTokens(did string, num int) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=?", did, TokenIsFree)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	tl := len(t)
	if tl > num {
		tl = num
	}
	wt := make([]Token, 0)
	for i := 0; i < tl; i++ {
		wt = append(wt, t[i])
	}
	for i := range wt {
		wt[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &wt[i], "did=? AND token_id=?", did, wt[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	return wt, nil
}

func (w *Wallet) GetTokens(did string, amt float64) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=?", did, TokenIsFree)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	if int(amt) > len(t) {
		return nil, fmt.Errorf("insufficient tokens")
	}
	wt := make([]Token, 0)
	for i := 0; i < int(amt); i++ {
		wt = append(wt, t[i])
	}
	for i := range wt {
		wt[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &wt[i], "did=? AND token_id=?", did, wt[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	//::TODO:: Part Tokens
	return wt, nil
}

func (w *Wallet) LockToken(wt *Token) error {
	w.l.Lock()
	defer w.l.Unlock()
	wt.TokenStatus = TokenIsLocked
	return w.s.Update(TokenStorage, wt, "did=? AND token_id=?", wt.DID, wt.TokenID)
}

func (w *Wallet) ReleaseTokens(wt []Token) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", wt[i].TokenID)
		if err != nil {
			w.log.Error("Failed to read token", "err", err)
			return err
		}
		if t.TokenStatus == TokenIsLocked {
			t.TokenStatus = TokenIsFree
			err = w.s.Update(TokenStorage, &t, "token_id=?", t.TokenID)
			if err != nil {
				w.log.Error("Failed to update token", "err", err)
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) RemoveTokens(wt []Token) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		err := w.s.Delete(TokenStorage, &Token{}, "did=? AND token_id=?", wt[i].DID, wt[i].TokenID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) ClearTokens(did string) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.s.Delete(TokenStorage, &Token{}, "did=?", did)
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) TokensTransferred(did string, ti []contract.TokenInfo, b *block.Block) error {
	w.l.Lock()
	defer w.l.Unlock()
	// ::TODO:: need to address part & other tokens
	err := w.CreateTokenBlock(b, ti[0].TokenType)
	if err != nil {
		return err
	}
	for i := range ti {
		var t Token
		err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, ti[i].Token)
		if err != nil {
			return err
		}
		t.TokenValue = 1
		t.TokenStatus = TokenIsTransferred
		err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, ti[i].Token)
		if err != nil {
			return err
		}
	}
	// for i := range pt {
	// 	var t Token
	// 	err := w.s.Read(PartTokenStorage, &t, "did=? AND token_id=?", did, pt[i])
	// 	if err != nil {
	// 		return err
	// 	}
	// 	bid, err := b.GetBlockID(pt[i])
	// 	if err != nil {
	// 		return err
	// 	}
	// 	err = w.AddTokenBlock(pt[i], b)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	t.TokenChainID = bid
	// 	t.TokenStatus = TokenIsTransferred
	// 	err = w.s.Update(PartTokenStorage, &t, "did=? AND token_id=?", did, pt[i])
	// 	if err != nil {
	// 		return err
	// 	}
	// }
	return nil
}

func (w *Wallet) TokensReceived(did string, ti []contract.TokenInfo, b *block.Block) error {
	w.l.Lock()
	defer w.l.Unlock()
	// TODO :: Needs to be address
	err := w.CreateTokenBlock(b, ti[0].TokenType)
	if err != nil {
		return err
	}
	for i := range ti {
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", ti[i].Token)
		if err != nil || t.TokenID == "" {
			dir := util.GetRandString()
			err := util.CreateDir(dir)
			if err != nil {
				w.log.Error("Faled to create directory", "err", err)
				return err
			}
			defer os.RemoveAll(dir)
			err = w.Get(ti[i].Token, did, OwnerRole, dir)
			if err != nil {
				w.log.Error("Faled to get token", "err", err)
				return err
			}
			t = Token{
				TokenID:    ti[i].Token,
				TokenValue: 1,
				DID:        did,
			}
			err = w.s.Write(TokenStorage, &t)
			if err != nil {
				return err
			}
		} else {
			if t.TokenStatus != TokenIsTransferred {
				return fmt.Errorf("Token already exist")
			}
		}

		t.DID = did
		t.TokenStatus = TokenIsFree
		err = w.s.Update(TokenStorage, &t, "token_id=?", ti[i].Token)
		if err != nil {
			return err
		}
		//Pinnig the whole tokens and pat tokens
		ok, err := w.Pin(ti[i].Token, OwnerRole, did)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("failed to pin token")
		}
	}
	// for i := range pt {
	// 	var t Token
	// 	err := w.s.Read(PartTokenStorage, &t, "did=? AND token_id=?", did, pt[i])
	// 	if err != nil {
	// 		t = Token{
	// 			TokenID: wt[i],
	// 			DID:     did,
	// 		}
	// 	}
	// 	ha, ok := tcb[TCBlockHashKey]
	// 	if !ok {
	// 		return fmt.Errorf("invalid token chain block")
	// 	}
	// 	t.TokenChainID = ha.(string)
	// 	t.TokenStatus = TokenIsTransferred
	// 	w.AddTokenBlock(pt[i], tcb)
	// }
	return nil
}
