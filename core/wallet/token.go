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
	TokenIsCommitted
	TokenIsGenerated
	TokenIsDeployed
	TokenIsFetched
	TokenIsBurnt
	TokenIsExecuted
	TokenIsOrphaned
	TokenChainSyncIssue
	TokenPledgeIssue
	TokenIsBeingDoubleSpent
)
const (
	Zero int = iota
	One
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

func (w *Wallet) UnpledgeWholeToken(did string, token string, tt int) error {
	w.l.Lock()
	defer w.l.Unlock()
	var t Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, token)
	if err != nil {
		w.log.Error("Failed to get token", "token", token, "err", err)
		return err
	}

	if t.TokenStatus != TokenIsPledged {
		w.log.Error("Token is not pledged")
		return fmt.Errorf("token is not pledged")
	}

	b := w.GetLatestTokenBlock(token, tt)
	if b.GetTransType() != block.TokenUnpledgedType {
		w.log.Error("Token block not in un pledged state")
		return fmt.Errorf("Token block not in un pledged state")
	}
	t.TokenStatus = TokenIsFree
	err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, token)
	if err != nil {
		w.log.Error("Failed to update token", "token", token, "err", err)
		return err
	}
	return nil
}

func (w *Wallet) GetAllTokens(did string) ([]Token, error) {
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

func (w *Wallet) GetAllPledgedTokens() ([]Token, error) {
	var t []Token
	err := w.s.Read(TokenStorage, &t, "token_status=?", TokenIsPledged)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (w *Wallet) GetCloserToken(did string, rem float64) (*Token, error) {
	if rem > 1.0 {
		return nil, fmt.Errorf("token value not less than whole token")
	}
	var tks []Token
	err := w.s.Read(TokenStorage, &tks, "did=? AND token_status=? AND token_value>=? AND token_value <?", did, TokenIsFree, rem, 1.0)
	if err != nil || len(tks) == 0 {
		err := w.s.Read(TokenStorage, &tks, "did=? AND token_status=? AND token_value=?", did, TokenIsFree, 1.0)
		if err != nil {
			return nil, err
		}
		if len(tks) == 0 {
			return nil, fmt.Errorf("failed to find free token")
		}
		return &tks[0], err
	}
	TokenSort(tks, false)
	return &tks[0], nil
}

func (w *Wallet) GetWholeTokens(did string, num int) ([]Token, int, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=? AND token_value=?", did, TokenIsFree, 1.0)
	if err != nil {
		return nil, num, err
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
			return nil, num, err
		}
	}
	return wt, (num - tl), nil
}

func (w *Wallet) GetTokensByLimit(did string, limit float64) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=? AND token_value<=?", did, TokenIsFree, limit)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	for i := range t {
		t[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &t[i], "did=? AND token_id=?", did, t[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	TokenSort(t, true)
	return t, nil
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

func (w *Wallet) GetToken(token string, token_Status int) (*Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t Token
	err := w.s.Read(TokenStorage, &t, "token_id=? AND token_status=?", token, token_Status)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	t.TokenStatus = TokenIsLocked
	err = w.s.Update(TokenStorage, &t, "token_id=?", t.TokenID)
	if err != nil {
		w.log.Error("Failed to update token status", "err", err)
		return nil, err
	}
	return &t, nil
}

func (w *Wallet) ReadToken(token string) (*Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t Token
	err := w.s.Read(TokenStorage, &t, "token_id=?", token)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return &t, nil
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

func (w *Wallet) ReleaseToken(token string) error {
	w.l.Lock()
	defer w.l.Unlock()
	var t Token
	err := w.s.Read(TokenStorage, &t, "token_id=?", token)
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

func (w *Wallet) UpdateToken(t *Token) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.s.Update(TokenStorage, t, "token_id=?", t.TokenID)
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) TokensTransferred(did string, ti []contract.TokenInfo, b *block.Block, local bool) error {
	w.l.Lock()
	defer w.l.Unlock()
	// ::TODO:: need to address part & other tokens
	// Skip update if it is local DID
	if !local {
		err := w.CreateTokenBlock(b)
		if err != nil {
			return err
		}
		for i := range ti {
			var t Token
			err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, ti[i].Token)
			if err != nil {
				return err
			}
			t.TokenStatus = TokenIsTransferred
			err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, ti[i].Token)
			if err != nil {
				return err
			}
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

func (w *Wallet) TokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string) error {
	w.l.Lock()
	defer w.l.Unlock()
	// TODO :: Needs to be address
	err := w.CreateTokenBlock(b)
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
			gb := w.GetGenesisTokenBlock(ti[i].Token, ti[i].TokenType)
			pt := ""
			if gb != nil {
				pt, _, _ = gb.GetParentDetials(ti[i].Token)
			}
			t = Token{
				TokenID:       ti[i].Token,
				TokenValue:    ti[i].TokenValue,
				ParentTokenID: pt,
				DID:           did,
			}
			err = w.s.Write(TokenStorage, &t)
			if err != nil {
				return err
			}
		}

		t.DID = did
		t.TokenStatus = TokenIsFree
		err = w.s.Update(TokenStorage, &t, "token_id=?", ti[i].Token)
		if err != nil {
			return err
		}
		senderAddress := senderPeerId + "." + b.GetSenderDID()
		receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
		//Pinnig the whole tokens and pat tokens
		ok, err := w.Pin(ti[i].Token, OwnerRole, did, b.GetTid(), senderAddress, receiverAddress, ti[i].TokenValue)
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

func (w *Wallet) CommitTokens(did string, rbtTokens []string) error {
	w.l.Lock()
	defer w.l.Unlock()
	for i := range rbtTokens {
		var t Token
		err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, rbtTokens[i])
		if err != nil {
			return err
		}
		t.TokenStatus = TokenIsCommitted
		err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, rbtTokens[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) GetAllPartTokens(did string) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=? AND token_value>? AND token_value<? ORDER BY token_value DESC", did, TokenIsFree, Zero, One)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	for i := range t {
		t[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &t[i], "did=? AND token_id=?", did, t[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	return t, nil
}

func (w *Wallet) GetAllWholeTokens(did string) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=? AND token_value=?", did, TokenIsFree, 1.0)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	for i := range t {
		t[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &t[i], "did=? AND token_id=?", did, t[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	return t, nil
}

/* func (w *Wallet) UpdateChildTokenStatusToOrphan(tokenHash string) (error){
	w.l.Lock()
	defer w.l.Unlock()
	err := w.s.Update(TokenStorage, nil, "token_id=?", tokenHash)
	if err != nil {
		return err
	}
	return nil
} */

func (w *Wallet) GetChildToken(did string, parentTokenID string) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND parent_token_id=? ", did, parentTokenID)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	for i := range t {
		t[i].TokenStatus = TokenIsLocked
		err = w.s.Update(TokenStorage, &t[i], "did=? AND token_id=?", did, t[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	return t, nil
}

func (w *Wallet) GetAllLockedTokens() ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "token_status=?", TokenIsLocked)
	if err != nil && err.Error() != "no records found" {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

func (w *Wallet) ReleaseAllLockedTokens() error {
	var lockedTokens []Token
	lockedTokens, err := w.GetAllLockedTokens()
	if err != nil && err.Error() != "no records found" {
		w.log.Error("Failed to get tokens", "err", err)
		return err
	}

	if len(lockedTokens) == 0 {
		w.log.Info("No locked tokens to release")
		return nil
	}
	for _, t := range lockedTokens {
		t.TokenStatus = TokenIsFree
		err = w.s.Update(TokenStorage, &t, "token_id=?", t.TokenID)
		if err != nil {
			w.log.Error("Failed to update token", "err", err)
			return err
		}
	}
	return nil
}
