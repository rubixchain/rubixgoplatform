package wallet

import (
	"fmt"
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

const (
	RACTestTokenVersion int = 1
)

const (
	RACTypeKey         string = "type"
	RACDidKey          string = "creatorDid"
	RACTotalSupplyKey  string = "totalSupply"
	RACTokenCountKey   string = "tokenCount"
	RACCreatorInputKey string = "creatorInput"
	RACHashKey         string = "contentHash"
	RACUrlKey          string = "url"
	RACVersionKey      string = "version"
	RACNonceKey        string = "nonce"
	RACSignKey         string = "pvtKeySign"
)

type Token struct {
	TokenID      string `gorm:"column:token_id;primary_key"`
	TokenDetials string `gorm:"column:token_detials"`
	DID          string `gorm:"column:did"`
	TokenChainID string `gorm:"column:token_chain_id"`
	ProofChainID string `gorm:"column:proof_chain_id"`
	TokenStatus  int    `gorm:"column:token_status;"`
}

type RACType struct {
	Type         int    `json:"type"`
	CreatorDID   string `json:"creatorDid"`
	TotalSupply  int    `json:"totalSupply"`
	TokenCount   int    `json:"tokenCount"`
	ContentHash  string `json:"contentHash"`
	URL          string `json:"url"`
	CreatorInput string `json:"creatorInput"`
	Version      int    `json:"version"`
	Nonce        string `json:"nonce"`
	Signature    string `json:"pvtKeySign"`
}

type TestTokenDetials struct {
	DID      string `json:"column:did"`
	RandomID []byte `json:"column:random_id"`
}

type PartToken struct {
	TokenID      string `gorm:"column:token_id;primary_key"`
	WholeTokenID string `gorm:"column:whole_token_id"`
	TokenValue   string `gorm:"column:token_value"`
	DID          string `gorm:"column:did"`
	TokenChainID string `gorm:"column:token_chain_id"`
	TokenStatus  int    `gorm:"column:token_status;"`
}

func (w *Wallet) CreateToken(t *Token) error {
	return w.s.Write(TokenStorage, t)
}

func (w *Wallet) PledgeWholeToken(did string, token string, tcb map[string]interface{}) error {
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
	h, ok := tcb[TCBlockHashKey]
	if !ok {
		w.log.Error("Invalid token chain block")
		return fmt.Errorf("Invalid token chain block")
	}
	t.TokenChainID = h.(string)
	t.TokenStatus = TokenIsPledged
	err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, token)
	if err != nil {
		w.log.Error("Failed to update token", "token", token, "err", err)
		return err
	}
	err = w.AddLatestTokenBlock(token, tcb)
	if err != nil {
		w.log.Error("Failed to add token chain block", "token", token, "err", err)
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

func (w *Wallet) GetAllPartTokens(did string) ([]PartToken, error) {
	var t []PartToken
	err := w.s.Read(PartTokenStorage, &t, "did=?", did)
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

func (w *Wallet) GetTokens(did string, amt float64) ([]Token, []PartToken, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=?", did, TokenIsFree)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
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
		wt[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &wt[i], "did=? AND token_id=?", did, wt[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, nil, err
		}
	}
	//::TODO:: Part Tokens
	return wt, nil, nil
}

func (w *Wallet) LockToken(wt *Token) error {
	w.l.Lock()
	defer w.l.Unlock()
	wt.TokenStatus = TokenIsLocked
	return w.s.Update(TokenStorage, wt, "did=? AND token_id=?", wt.DID, wt.TokenID)
}

func (w *Wallet) ReleaseTokens(wt []Token, pt []PartToken) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		wt[i].TokenStatus = TokenIsFree
		err := w.s.Update(TokenStorage, &wt[i], "did=? AND token_id=?", wt[i].DID, wt[i].TokenID)
		if err != nil {
			return err
		}
	}
	for i := range pt {
		pt[i].TokenStatus = TokenIsFree
		err := w.s.Update(PartTokenStorage, &pt[i], "did=? AND token_id=?", pt[i].DID, pt[i].TokenID)
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
		err := w.s.Delete(TokenStorage, &Token{}, "did=? AND token_id=?", wt[i].DID, wt[i].TokenID)
		if err != nil {
			return err
		}
	}
	for i := range pt {
		err := w.s.Delete(PartTokenStorage, &PartToken{}, "did=? AND token_id=?", pt[i].DID, pt[i].TokenID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) TokensTransferred(did string, wt []string, pt []string, tcb map[string]interface{}) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		var t Token
		err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, wt[i])
		if err != nil {
			return err
		}
		ha, ok := tcb[TCBlockHashKey]
		if !ok {
			return fmt.Errorf("invalid token chain block")
		}
		t.TokenChainID = ha.(string)
		t.TokenStatus = TokenIsTransferred
		err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, wt[i])
		if err != nil {
			return err
		}
		w.AddLatestTokenBlock(wt[i], tcb)
	}
	for i := range pt {
		var t Token
		err := w.s.Read(PartTokenStorage, &t, "did=? AND token_id=?", did, pt[i])
		if err != nil {
			return err
		}
		ha, ok := tcb[TCBlockHashKey]
		if !ok {
			return fmt.Errorf("invalid token chain block")
		}
		t.TokenChainID = ha.(string)
		t.TokenStatus = TokenIsTransferred
		err = w.s.Update(PartTokenStorage, &t, "did=? AND token_id=?", did, pt[i])
		if err != nil {
			return err
		}
		w.AddLatestTokenBlock(pt[i], tcb)
	}
	return nil
}

func (w *Wallet) TokensReceived(did string, wt []string, pt []string, tcb map[string]interface{}) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range wt {
		var t Token
		err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, wt[i])
		if err != nil {
			t = Token{
				TokenID: wt[i],
				DID:     did,
			}
			err = w.s.Write(TokenStorage, &t)
			if err != nil {
				return err
			}
		}
		ha, ok := tcb[TCBlockHashKey]
		if !ok {
			return fmt.Errorf("invalid token chain block")
		}
		t.TokenChainID = ha.(string)
		t.TokenStatus = TokenIsFree
		err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, wt[i])
		if err != nil {
			return err
		}
		w.AddLatestTokenBlock(wt[i], tcb)
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
	// 	w.AddLatestTokenBlock(pt[i], tcb)
	// }
	return nil
}
