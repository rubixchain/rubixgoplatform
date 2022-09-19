package wallet

import "fmt"

type Token struct {
	TokenID      string `gorm:"column:token_id;primary_key"`
	TokenDetials string `gorm:"column:token_detials"`
	DID          string `gorm:"column:did"`
	TokenChainID string `gorm:"column:token_chain_id"`
	Lock         bool   `gorm:"column:lock;type:bit"`
}

type PartToken struct {
	TokenID      string `gorm:"column:token_id;primary_key"`
	WholeTokenID string `gorm:"column:whole_token_id"`
	TokenValue   string `gorm:"column:token_value"`
	DID          string `gorm:"column:did"`
	TokenChainID string `gorm:"column:token_chain_id"`
	Lock         bool   `gorm:"column:lock;type:bit"`
}

func (w *Wallet) CreateToken(t *Token) error {
	return w.s.Write(TokenStorage, t)
}

func (w *Wallet) GetTokens(did string, amt float64) ([]Token, []PartToken, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND lock=?", did, false)
	if err != nil {
		return nil, nil, err
	}
	if int(amt) > len(t) {
		return nil, nil, fmt.Errorf("insufficient tokens")
	}
	wt := make([]Token, 0)
	for i := 0; i < int(amt); i++ {
		wt = append(wt, t[i])
	}
	for i := range wt {
		wt[i].Lock = true
		err = w.s.Update(TokenStorage, &wt[i], "did=? AND token_id", did, wt[i].TokenID)
		if err != nil {
			return nil, nil, err
		}
	}
	//::TODO:: Part Tokens
	return wt, nil, nil
}

func (w *Wallet) ReleaseTokens(wt []Token, pt []PartToken) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		wt[i].Lock = false
		err := w.s.Update(TokenStorage, &wt[i], "did=? AND token_id", wt[i].DID, wt[i].TokenID)
		if err != nil {
			return err
		}
	}
	for i := range pt {
		pt[i].Lock = false
		err := w.s.Update(PartTokenStorage, &pt[i], "did=? AND token_id", pt[i].DID, pt[i].TokenID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) RemoveTokens(wt []Token, pt []PartToken) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		err := w.s.Delete(TokenStorage, &Token{}, "did=? AND token_id", wt[i].DID, wt[i].TokenID)
		if err != nil {
			return err
		}
	}
	for i := range pt {
		err := w.s.Delete(PartTokenStorage, &PartToken{}, "did=? AND token_id", pt[i].DID, pt[i].TokenID)
		if err != nil {
			return err
		}
	}
	return nil
}
