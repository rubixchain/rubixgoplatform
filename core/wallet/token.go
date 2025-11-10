package wallet

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
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
	TokenIsPinnedAsService
	TokenIsBurntForFT
	TokenIsPending                // Tokens received but not yet confirmed by consensus finality
	QuorumPledgedForThisToken int = 20
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

const (
	SyncUnrequired int = iota
	SyncIncomplete
	SyncCompleted
)

type Token struct {
	TokenID        string    `gorm:"column:token_id;primaryKey"`
	ParentTokenID  string    `gorm:"column:parent_token_id"`
	TokenValue     float64   `gorm:"column:token_value"`
	DID            string    `gorm:"column:did"`
	TokenStatus    int       `gorm:"column:token_status;"`
	TokenStateHash string    `gorm:"column:token_state_hash"`
	TransactionID  string    `gorm:"column:transaction_id"`
	Added          bool      `gorm:"column:added"`
	SyncStatus     int       `gorm:"column:sync_status"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

type SyncedRBT struct {
	TokenID string `gorm:"column:token_id;primaryKey"`
	// ParentTokenID string  `gorm:"column:parent_token_id"`
	TokenValue    float64 `gorm:"column:token_value"`
	OwnerDID      string  `gorm:"column:owner_did"`
	PublisherDID  string  `gorm:"column:publisher_did"`
	TransactionID string  `gorm:"column:transaction_id"`
	BlockHash     string  `gorm:"column:block_hash"`
	BlockHeight   uint64  `gorm:"column:block_height"`
	SyncStaus     int     `gorm:"column:sync_status"`
	TokenStatus   int     `gorm:"column:token_status"`
	// SenderAddress string  `gorm:"column:owner_address"`
}

type RBTContent struct {
	TokenID    string `gorm:"column:token_id;primaryKey"`
	RBTContent string `gorm:"column:rbt_content"`
}

func (w *Wallet) CreateToken(t *Token) error {
	return w.s.Write(TokenStorage, t)
}
func (w *Wallet) CreateFT(ft *FTToken) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.s.Write(FTTokenStorage, ft)
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

func (w *Wallet) GetFreeTokens(did string) ([]Token, error) {
	var t []Token
	err := w.s.Read(TokenStorage, &t, "token_status=? AND did=?", TokenIsFree, did)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []Token{}, nil
		} else {
			return nil, err
		}
	}
	return t, nil
}

// This function fetches all rbt  tokens, which a node can have maximum token chain length(i.e Tokens with status as Free, Pledged, Burnt or TokenISBurntForFT)
func (w *Wallet) GetRBTTokensWithMaxChainLength(did string) ([]Token, error) {
	var tokens []Token

	err := w.s.Read(TokenStorage, &tokens, "(token_status=? OR token_status=? OR token_status=? OR token_status=?) AND did=?", TokenIsFree, TokenIsPledged, TokenIsBurnt, TokenIsBurntForFT, did)
	if err != nil {
		w.log.Error("Failed to get rbt tokens with maximum chain length ", "err", err)
		return nil, err
	}

	return tokens, nil
}

// This function fetches FT Tokens, which a node can have maximum token chain length(i.e tokens with status Free)
func (w *Wallet) GetFTTokensWithMaxChainLength(did string) ([]FTToken, error) {
	var FTTokens []FTToken
	err := w.s.Read(FTTokenStorage, &FTTokens, "token_status=? AND owner_did=?", TokenIsFree, did)
	if err != nil {
		w.log.Error("Failed to get FT tokens with maximum chain length ", "err", err)
		return nil, err
	}
	return FTTokens, nil
}

// This function fetches NFT Tokens, which a node can have maximum token chain length(i.e tokens with status Free and TokenIsDeployed)
func (w *Wallet) GetNFTTokensWithMaxChainLength(did string) ([]NFT, error) {
	var NFTTokens []NFT
	err := w.s.Read(NFTTokenStorage, &NFTTokens, "(token_status=? OR token_status=?) AND did=?", TokenIsDeployed, TokenIsFree, did)
	if err != nil {
		w.log.Error("Failed to get NFT tokens with maximum chain length ", "err", err)
		return nil, err
	}
	return NFTTokens, nil
}

// This function fetches smart contract Tokens, which a node can have maximum token chain length(i.e tokens with status Free and TokenIsDeployed)
func (w *Wallet) GetSmartContractTokensWithMaxChainLength(did string) ([]SmartContract, error) {
	var SmartContractTokens []SmartContract
	err := w.s.Read(SmartContractStorage, &SmartContractTokens, "(contract_status=? OR contract_status=?) AND deployer=?", TokenIsDeployed, TokenIsExecuted, did)
	if err != nil {
		w.log.Error("Failed to get smart contract tokens with maximum chain length ", "err", err)
		return nil, err
	}
	return SmartContractTokens, nil
}

// This function will return all the FTs, their count and the creator DID in the node
func (w *Wallet) GetAllFTsAndCount() ([]FT, error) {
	fts, err := w.GetAllFreeFTs()
	if err != nil {
		errStr := fmt.Sprint(err)
		if strings.Contains(errStr, "no records found") {
			w.log.Info("No free FTs found")
			return nil, err
		}
		w.log.Error("Failed to free FTs", "err", err)
		return nil, err
	}
	ftNameCreatorCounts := make(map[string]map[string]int)

	for _, t := range fts {
		if ftNameCreatorCounts[t.FTName] == nil {
			ftNameCreatorCounts[t.FTName] = make(map[string]int)
		}
		ftNameCreatorCounts[t.FTName][t.CreatorDID]++
	}

	info := make([]FT, 0)
	idCounter := 1 // Initialize ID counter starting from 1
	for ftName, creatorCounts := range ftNameCreatorCounts {
		for creatorDID, count := range creatorCounts {
			info = append(info, FT{
				ID:         fmt.Sprintf("%d", idCounter),
				FTName:     ftName,
				FTCount:    count,
				CreatorDID: creatorDID,
			})
			idCounter++
		}
	}
	return info, nil
}

// This function will return all the FTs, their count and the creator DID for a DID
func (w *Wallet) GetFTsAndCount(did string) ([]FT, error) {
	fts, err := w.GetFreeFTsByDID(did)
	if err != nil {
		errStr := fmt.Sprint(err)
		if strings.Contains(errStr, "no records found") {
			w.log.Info("No free FTs found")
			return nil, err
		}
		w.log.Error("Failed to free FTs", "err", err)
		return nil, err
	}

	ftNameCreatorCounts := make(map[string]map[string]int)

	for _, t := range fts {
		if ftNameCreatorCounts[t.FTName] == nil {
			ftNameCreatorCounts[t.FTName] = make(map[string]int)
		}
		ftNameCreatorCounts[t.FTName][t.CreatorDID]++
	}

	info := make([]FT, 0)
	idCounter := 1 // Initialize ID counter starting from 1
	for ftName, creatorCounts := range ftNameCreatorCounts {
		for creatorDID, count := range creatorCounts {
			info = append(info, FT{
				ID:         fmt.Sprintf("%d", idCounter),
				FTName:     ftName,
				FTCount:    count,
				CreatorDID: creatorDID,
			})
			idCounter++
		}
	}

	return info, nil
}

func (w *Wallet) GetFreeFTsByDID(did string) ([]FTToken, error) {
	var FT []FTToken
	err := w.s.Read(FTTokenStorage, &FT, "owner_did=? AND token_status=? OR token_status=?", did, TokenIsFree, TokenIsGenerated)

	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No free FTs")
			return nil, err
		}
		w.log.Error("Failed to get FTs", "err", err)
		return nil, err
	}
	return FT, nil
}

func (w *Wallet) GetAllFreeFTs() ([]FTToken, error) {
	var FT []FTToken
	err := w.s.Read(FTTokenStorage, &FT, "token_status=?", TokenIsFree)
	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No free FTs")
			return nil, err
		}
		w.log.Error("Failed to get FTs", "err", err)
		return nil, err
	}
	return FT, nil
}

func (w *Wallet) GetFreeFTsByNameAndDID(ftName string, did string) ([]FTToken, error) {
	var FT []FTToken
	err := w.s.Read(FTTokenStorage, &FT, "ft_name=? AND token_status =? AND  owner_did=?", ftName, TokenIsFree, did)

	if err != nil {
		w.log.Error("Failed to get Free FTs by name", "err", err)
		return nil, err
	}
	return FT, nil
}

func (w *Wallet) GetFreeFTsByNameAndCreatorDID(ftName string, did string, creatorDID string) ([]FTToken, error) {
	var FT []FTToken
	err := w.s.Read(FTTokenStorage, &FT, "ft_name=? AND token_status =? AND owner_did=? AND creator_did=?", ftName, TokenIsFree, did, creatorDID)
	if err != nil {
		w.log.Error("Failed to get Free FTs by name and creator DID", "err", err)
		return nil, err
	}
	return FT, nil
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

func (w *Wallet) GetWholeTokens(did string, num int, trnxMode int) ([]Token, int, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	if trnxMode == 0 {
		err := w.s.Read(TokenStorage, &t, "did=? AND (token_status=? OR token_status=?) AND token_value=?", did, TokenIsFree, TokenIsPinnedAsService, 1.0)
		if err != nil {
			return nil, num, err
		}
	} else {
		err := w.s.Read(TokenStorage, &t, "did=? AND token_status=? AND token_value=?", did, TokenIsFree, 1.0)
		if err != nil {
			return nil, num, err
		}

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
		err1 := w.s.Update(TokenStorage, &wt[i], "did=? AND token_id=?", did, wt[i].TokenID)
		if err1 != nil {
			w.log.Error("Failed to update token status", "err", err1)
			return nil, num, err1
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

func (w *Wallet) GetAllFreeToken(did string) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=?", did, TokenIsFree)
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
	//::TODO:: Part Tokens
	return t, nil
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

func (w *Wallet) ReadFTToken(token string) (*FTToken, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t FTToken
	err := w.s.Read(FTTokenStorage, &t, "token_id=?", token)
	if err != nil {
		w.log.Error(fmt.Sprintf("failed to get FT Token %v from FTTokenStorage, err: %v", token, err))
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

func (w *Wallet) UpdateFTToken(t *FTToken) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.s.Update(FTTokenStorage, t, "token_id=?", t.TokenID)
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) UpdateTokenStatus(did string, tokenHash string, tokenType int, tokenStatus int) error {
	w.l.Lock()
	defer w.l.Unlock()

	var (
		storage string
		token   interface{}
	)

	// Determine the storage and token type
	var didVar string
	switch tokenType {
	case 0, 1, 3, 5:
		storage = TokenStorage
		token = &Token{}
		didVar = "did"
	case 9:
		storage = FTTokenStorage
		token = &FTToken{}
		didVar = "owner_did"
	default:
		w.log.Warn("Unsupported token type: %d", tokenType)
		return fmt.Errorf("unsupported token type: %d", tokenType)
	}
	condition := fmt.Sprintf("%s=?", didVar)
	// Read the token
	err := w.s.Read(storage, token, condition+" AND token_id=?", did, tokenHash)
	if err != nil {
		w.log.Error("Error reading from %s: %v", storage, err)
		return err
	}

	// Update the token status
	switch t := token.(type) {
	case *Token:
		t.TokenStatus = tokenStatus
	case *FTToken:
		t.TokenStatus = tokenStatus
	}

	// Save the updated token
	err = w.s.Update(storage, token, condition+" AND token_id=?", did, tokenHash)
	if err != nil {
		w.log.Error("Error updating %s: %v", storage, err)
		return err
	}
	return nil
}

func (w *Wallet) GetTokenStatus(did string, tokenHash string, tokenType int) (model.TokenStatusResponse, error) {
	var (
		storage string
		token   interface{}
	)
	var resp model.TokenStatusResponse
	var didVar string
	// Determine the storage and token type
	switch tokenType {
	case 0, 1, 3, 5:
		storage = TokenStorage
		token = &Token{}
		didVar = "did"
	case 9:
		storage = FTTokenStorage
		token = &FTToken{}
		didVar = "owner_did"
	default:
		err := fmt.Errorf("unsupported token type: %d", tokenType)
		w.log.Warn(err.Error())
		return resp, err
	}

	// Lock only around the critical section
	w.l.Lock()
	condition := fmt.Sprintf("%s=?", didVar)
	err := w.s.Read(storage, token, condition+" AND token_id=?", did, tokenHash)
	w.l.Unlock()

	if err != nil {
		w.log.Error("Error reading from %s for DID: %s, TokenHash: %s: %v", storage, did, tokenHash, err)
		return resp, err
	}

	// Populate the response based on the token type
	switch t := token.(type) {
	case *Token:
		resp = populateTokenResponse(t.DID, t.TokenID, tokenType, t.TokenStatus, t.TransactionID, "")
	case *FTToken:
		resp = populateTokenResponse(t.DID, t.TokenID, tokenType, t.TokenStatus, t.TransactionID, t.FTName)
	}
	return resp, nil
}

// Helper function to populate the response
func populateTokenResponse(did, tokenID string, tokenType, tokenStatus int, transactionID string, ftName string) model.TokenStatusResponse {
	return model.TokenStatusResponse{
		DID:           did,
		Token:         tokenID,
		Type:          tokenType,
		Status:        tokenStatus,
		TransactionID: transactionID,
		FTName:        ftName,
	}
}

func (w *Wallet) TokensTransferred(did string, ti []contract.TokenInfo, b *block.Block, local bool, pinningServiceMode bool) error {
	w.l.Lock()
	defer w.l.Unlock()
	// ::TODO:: need to address part & other tokens
	// Skip update if it is local DID
	if !local {
		err := w.CreateTokenBlock(b)
		if err != nil {
			return err
		}
		var tokenStatus int
		if pinningServiceMode {
			tokenStatus = TokenIsPinnedAsService
		} else {
			tokenStatus = TokenIsTransferred
		}
		for i := range ti {
			var t Token
			err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, ti[i].Token)
			if err != nil {
				return err
			}
			t.TokenStatus = tokenStatus
			t.TransactionID = b.GetTid()

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
func (w *Wallet) FTTokensTransffered(did string, ti []contract.TokenInfo, b *block.Block, areReceiverAndSenderPeerSame bool) error {
	w.l.Lock()
	defer w.l.Unlock()

	// Check if the Reciever DID is local or not
	// If so, then skip the following as its has been
	// done by the previous Receive process
	if !areReceiverAndSenderPeerSame {
		err := w.CreateTokenBlock(b)
		if err != nil {
			return err
		}
		tokenStatus := TokenIsTransferred
		for i := range ti {
			var t FTToken
			err := w.s.Read(FTTokenStorage, &t, "token_id=?", ti[i].Token)
			if err != nil {
				return err
			}
			t.TokenStatus = tokenStatus
			//TODO: Check the need of transaction ID in FT Tokens table
			//t.TransactionID = b.GetTid()
			err = w.s.Update(FTTokenStorage, &t, "token_id=?", ti[i].Token)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
func (w *Wallet) TokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, pinningServiceMode bool, ipfsShell *ipfsnode.Shell) ([]string, error) {
	// For large transfers, use optimized processing with batch downloads
	if len(ti) > 50 {
		w.log.Info("Using optimized token receiver with batch downloads", "token_count", len(ti))
		return w.OptimizedTokensReceived(did, ti, b, senderPeerId, receiverPeerId, pinningServiceMode, ipfsShell)
	}

	w.l.Lock()
	defer w.l.Unlock()
	// TODO :: Needs to be address
	err := w.CreateTokenBlock(b)
	if err != nil {
		blockId, _ := b.GetBlockID(ti[0].Token)
		fmt.Println("failed to create token block, block Id", blockId)
		return nil, err
	}

	//add to ipfs to get latest Token State Hash after receiving the token by receiver. The hashes will be returned to sender, and from there to
	//quorums using pledgefinality function, to be added to TokenStateHash Table
	var updatedtokenhashes []string = make([]string, 0)
	var tokenHashMap map[string]string = make(map[string]string)

	// Prepare to collect provider details for batch write
	providerMaps := make([]model.TokenProviderMap, 0, len(ti))

	for _, info := range ti {
		t := info.Token
		b := w.GetLatestTokenBlock(info.Token, info.TokenType)
		blockId, _ := b.GetBlockID(t)
		tokenIDTokenStateData := t + blockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
		tokenIDTokenStateHash, tpm, _ := w.AddWithProviderMap(tokenIDTokenStateBuffer, did, OwnerRole)
		updatedtokenhashes = append(updatedtokenhashes, tokenIDTokenStateHash)
		tokenHashMap[t] = tokenIDTokenStateHash
		// Fill in extra fields for pinning
		tpm.FuncID = PinFunc
		tpm.TransactionID = b.GetTid()
		tpm.Sender = senderPeerId + "." + b.GetSenderDID()
		tpm.Receiver = receiverPeerId + "." + b.GetReceiverDID()
		tpm.TokenValue = info.TokenValue
		providerMaps = append(providerMaps, tpm)
	}

	// Handle each token
	for _, tokenInfo := range ti {
		// Check if token already exists
		var t Token
		err := w.s.Read(TokenStorage, &t, "token_id=?", tokenInfo.Token)
		if err != nil || t.TokenID == "" {
			// Token doesn't exist, proceed to handle it
			dir := util.GetRandString()
			if err := util.CreateDir(dir); err != nil {
				w.log.Error("Failed to create directory", "err", err)
				return nil, err
			}
			defer os.RemoveAll(dir)

			// Get the token
			if err := w.Get(tokenInfo.Token, did, OwnerRole, dir); err != nil {
				w.log.Error("Failed to get token", "err", err)
				return nil, err
			}

			// Get parent token details
			var parentTokenID string
			gb := w.GetGenesisTokenBlock(tokenInfo.Token, tokenInfo.TokenType)
			if gb != nil {
				parentTokenID, _, _ = gb.GetParentDetials(tokenInfo.Token)
			}

			// Create new token entry
			t = Token{
				TokenID:       tokenInfo.Token,
				TokenValue:    tokenInfo.TokenValue,
				ParentTokenID: parentTokenID,
				DID:           tokenInfo.OwnerDID,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}

			err = w.s.Write(TokenStorage, &t)
			if err != nil {
				fmt.Println("failed to write to db, token ", tokenInfo.Token)
				return nil, err
			}
		}
		// Update token status and pin tokens
		tokenStatus := TokenIsPending // Changed from TokenIsFree to prevent premature spending
		role := OwnerRole
		ownerdid := did
		if pinningServiceMode {
			tokenStatus = TokenIsPinnedAsService
			role = PinningRole
			ownerdid = b.GetOwner()
		}

		// Update token status
		t.DID = ownerdid
		t.TokenStatus = tokenStatus
		t.TransactionID = b.GetTid()
		t.TokenStateHash = tokenHashMap[tokenInfo.Token]
		t.SyncStatus = SyncIncomplete

		err = w.s.Update(TokenStorage, &t, "token_id=?", tokenInfo.Token)
		if err != nil {
			fmt.Println("failed to update to db, token ", tokenInfo.Token)
			return nil, err
		}
		senderAddress := senderPeerId + "." + b.GetSenderDID()
		receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
		//Pinnig the whole tokens and pat tokens (skip AddProviderDetails)
		_, err = w.Pin(tokenInfo.Token, role, did, b.GetTid(), senderAddress, receiverAddress, tokenInfo.TokenValue, true)
		if err != nil {
			fmt.Println("failed to pin token ", tokenInfo.Token)
			return nil, err
		}
	}

	// For large transfers, use async provider details processing
	if len(providerMaps) > 100 && w.asyncProviderMgr != nil {
		// Submit to async queue
		err := w.asyncProviderMgr.SubmitProviderDetails(providerMaps, b.GetTid())
		if err != nil {
			w.log.Error("Failed to submit provider details to async queue, falling back to sync", "err", err)
			// Fall back to synchronous processing
			goto syncProcessing
		}
		w.log.Info("Provider details submitted for async processing",
			"transaction_id", b.GetTid(),
			"token_count", len(providerMaps))
		return updatedtokenhashes, nil
	}

syncProcessing:
	// Batch write provider details with retry/backoff (synchronous)
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := w.AddProviderDetailsBatch(providerMaps)
		if err == nil {
			return updatedtokenhashes, nil
		}
		w.log.Error("Batch AddProviderDetails failed, retrying", "attempt", attempt+1, "err", err)
		time.Sleep(backoff(attempt))
	}
	return nil, fmt.Errorf("failed to batch add provider details after retries")
}

// need to update in such a way that only for FTs
func (w *Wallet) FTTokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, ipfsShell *ipfsnode.Shell, ftInfo FTToken) ([]string, error) {
	// Always use parallel FT receiver for consistent performance and no global locks
	w.log.Info("Using parallel FT receiver",
		"ft_count", len(ti),
		"ft_name", ftInfo.FTName)
	parallelReceiver := NewParallelFTReceiver(w)
	return parallelReceiver.ParallelFTTokensReceived(did, ti, b, senderPeerId, receiverPeerId, ipfsShell, ftInfo)
}

// FTTokensReceivedLegacy is the old implementation with global wallet lock
// Kept for reference but should not be used in production
func (w *Wallet) FTTokensReceivedLegacy(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, ipfsShell *ipfsnode.Shell, ftInfo FTToken) ([]string, error) {
	w.l.Lock()
	defer w.l.Unlock()
	// TODO :: Needs to be address
	err := w.CreateTokenBlock(b)
	if err != nil {
		return nil, err
	}

	//add to ipfs to get latest Token State Hash after receiving the token by receiver. The hashes will be returned to sender, and from there to
	//quorums using pledgefinality function, to be added to TokenStateHash Table
	var updatedtokenhashes []string = make([]string, 0)
	var tokenHashMap map[string]string = make(map[string]string)

	// Prepare to collect provider details for batch write
	providerMaps := make([]model.TokenProviderMap, 0, len(ti))

	for _, info := range ti {
		t := info.Token
		b := w.GetLatestTokenBlock(info.Token, info.TokenType)
		blockId, _ := b.GetBlockID(t)
		tokenIDTokenStateData := t + blockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
		tokenIDTokenStateHash, tpm, _ := w.AddWithProviderMap(tokenIDTokenStateBuffer, did, OwnerRole)
		updatedtokenhashes = append(updatedtokenhashes, tokenIDTokenStateHash)
		tokenHashMap[t] = tokenIDTokenStateHash
		// Fill in extra fields for pinning
		tpm.FuncID = PinFunc
		tpm.TransactionID = b.GetTid()
		tpm.Sender = senderPeerId + "." + b.GetSenderDID()
		tpm.Receiver = receiverPeerId + "." + b.GetReceiverDID()
		tpm.TokenValue = info.TokenValue
		providerMaps = append(providerMaps, tpm)
	}

	// Handle each token
	for _, tokenInfo := range ti {
		var FTInfo FTToken
		err := w.s.Read(FTTokenStorage, &FTInfo, "token_id=?", tokenInfo.Token)
		if err != nil || FTInfo.TokenID == "" {
			// Token doesn't exist, proceed to handle it
			dir := util.GetRandString()
			if err := util.CreateDir(dir); err != nil {
				w.log.Error("Failed to create directory", "err", err)
				return nil, err
			}
			defer os.RemoveAll(dir)

			// Get the token
			if err := w.Get(tokenInfo.Token, did, OwnerRole, dir); err != nil {
				w.log.Error("Failed to get token", "err", err)
				return nil, err
			}
			tt := tokenInfo.TokenType
			blk := w.GetGenesisTokenBlock(tokenInfo.Token, tt)
			if blk == nil {
				w.log.Error("failed to get gensis block for Parent DID updation, invalid token chain")
				return nil, err
			}
			FTOwner := blk.GetOwner()
			// Create new token entry
			FTInfo = FTToken{
				TokenID:    tokenInfo.Token,
				TokenValue: tokenInfo.TokenValue,
				CreatorDID: FTOwner,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			err = w.s.Write(FTTokenStorage, &FTInfo)
			if err != nil {
				return nil, err
			}
		}
		// Update token status and pin tokens
		var tokenStatus int
		if senderPeerId != receiverPeerId {
			tokenStatus = TokenIsPending // Changed from TokenIsFree to prevent premature spending
		} else {
			tokenStatus = TokenIsFree
		}
		role := OwnerRole
		ownerdid := did

		// Update token status
		FTInfo.FTName = ftInfo.FTName
		FTInfo.DID = ownerdid
		FTInfo.TokenStatus = tokenStatus
		FTInfo.TransactionID = b.GetTid()
		FTInfo.TokenStateHash = tokenHashMap[tokenInfo.Token]

		err = w.s.Update(FTTokenStorage, &FTInfo, "token_id=?", tokenInfo.Token)
		if err != nil {
			return nil, err
		}
		senderAddress := senderPeerId + "." + b.GetSenderDID()
		receiverAddress := receiverPeerId + "." + b.GetReceiverDID()
		//Pinnig the whole tokens and pat tokens (skip AddProviderDetails)
		if senderPeerId != receiverPeerId {
			_, err = w.Pin(tokenInfo.Token, role, did, b.GetTid(), senderAddress, receiverAddress, tokenInfo.TokenValue, true)
			if err != nil {
				return nil, err
			}
		}
	}

	// For large FT transfers, use async provider details processing
	if len(providerMaps) > 100 && w.asyncProviderMgr != nil {
		// Submit to async queue
		err := w.asyncProviderMgr.SubmitProviderDetails(providerMaps, b.GetTid())
		if err != nil {
			w.log.Error("Failed to submit FT provider details to async queue, falling back to sync", "err", err)
			// Fall back to synchronous processing
			goto ftSyncProcessing
		}
		w.log.Info("FT provider details submitted for async processing",
			"transaction_id", b.GetTid(),
			"token_count", len(providerMaps))
		return updatedtokenhashes, nil
	}

ftSyncProcessing:
	// Batch write provider details with retry/backoff (synchronous)
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := w.AddProviderDetailsBatch(providerMaps)
		if err == nil {
			return updatedtokenhashes, nil
		}
		w.log.Error("Batch AddProviderDetails failed, retrying", "attempt", attempt+1, "err", err)
		time.Sleep(backoff(attempt))
	}
	return nil, fmt.Errorf("failed to batch add provider details after retries")
}

// Local backoff function for retry logic in FTTokensReceived
func backoff(attempt int) time.Duration {
	jitter := time.Duration(rand.Intn(100)) * time.Millisecond
	return time.Duration(math.Pow(2, float64(attempt-1)))*100*time.Millisecond + jitter
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

func (w *Wallet) UnlockLockedTokens(did string, tokenList []string) error {
	for _, tid := range tokenList {
		var t Token
		err := w.s.Read(TokenStorage, &t, "did=? AND token_id=?", did, tid)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return err
		}
		t.TokenStatus = TokenIsFree
		err = w.s.Update(TokenStorage, &t, "did=? AND token_id=?", did, tid)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return err
		}
	}
	return nil
}

func (w *Wallet) AddTokenStateHash(did string, tokenStateHashes []string, pledgedtokens []string, TransactionID string) error {
	w.l.Lock()
	defer w.l.Unlock()
	var td TokenStateDetails
	if tokenStateHashes == nil {
		return nil
	}
	concatenatedpledgedtokens := strings.Join(pledgedtokens, ",")

	for _, tokenStateHash := range tokenStateHashes {
		td.DID = did
		td.PledgedTokens = concatenatedpledgedtokens
		td.TokenStateHash = tokenStateHash
		td.TransactionID = TransactionID

		err := w.s.Write(TokenStateHash, &td)
		if err != nil {
			w.log.Error("Token State Hash could not be added", "token state hash", tokenStateHash, "err", err)
			return err
		}
	}

	return nil
}

func (w *Wallet) GetTokenStateHashByTransactionID(transactionID string) ([]TokenStateDetails, error) {
	var td []TokenStateDetails
	err := w.s.Read(TokenStateHash, &td, "transaction_id = ?", transactionID)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []TokenStateDetails{}, nil
		} else {
			w.log.Error("Failed to get token states", "err", err)
			return nil, err
		}
	}
	return td, nil
}

func (w *Wallet) GetAllTokenStateHash() ([]TokenStateDetails, error) {
	var td []TokenStateDetails
	err := w.s.Read(TokenStateHash, &td, "did!=?", "")
	if err != nil {
		w.log.Error("Failed to get token states", "err", err)
		return nil, err
	}
	return td, nil
}

func (w *Wallet) RemoveTokenStateHash(tokenstatehash string) error {
	var td TokenStateDetails

	//Getting all the details about a particular token state hash
	err := w.s.Read(TokenStateHash, &td, "token_state_hash=?", tokenstatehash)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return nil
		} else {
			w.log.Error("Failed to fetch token state from DB", "err", err)
			return err
		}
	}

	err = w.s.Delete(TokenStateHash, &td, "token_state_hash=?", tokenstatehash)
	if err != nil {
		w.log.Error("Failed to delete token state hash details from DB", "err", err)
		return err
	}

	return nil
}

func (w *Wallet) RemoveTokenStateHashByTransactionID(transactionID string) error {
	var td []TokenStateDetails

	//Getting all the details about a particular token state hash
	err := w.s.Read(TokenStateHash, &td, "transaction_id=?", transactionID)
	if err != nil {
		if !strings.Contains(err.Error(), "no records found") {
			w.log.Error("Failed to fetch token state from DB", "err", err)
			return err
		} else {
			return nil
		}
	}

	if len(td) > 0 {
		err = w.s.Delete(TokenStateHash, &td, "transaction_id=?", transactionID)
		if err != nil {
			w.log.Error("Failed to delete token state hash details from DB", "err", err)
			return err
		}
	}

	return nil
}

func (w *Wallet) GetAllPinnedTokens(did string) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []Token
	err := w.s.Read(TokenStorage, &t, "did=? AND token_status=? ", did, TokenIsPinnedAsService)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	for i := range t {
		t[i].TokenStatus = TokenIsLocked // Here should we change it to TokenIsRecovered
		err = w.s.Update(TokenStorage, &t[i], "did=? AND token_id=?", did, t[i].TokenID)
		if err != nil {
			w.log.Error("Failed to update token status", "err", err)
			return nil, err
		}
	}
	return t, nil

}

func (w *Wallet) UpdateUnpledgedTokenStatus(did string, token string, tt int) error {
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

func (w *Wallet) GetTokensToBeSynced() ([]Token, error) {
	var tokensList []Token
	err := w.s.Read(TokenStorage, &tokensList, "sync_status = ? and (token_status = ? or token_status = ? or token_status = ? or token_status = ?)", SyncIncomplete, TokenIsFree, TokenIsLocked, QuorumPledgedForThisToken, TokenIsBurnt)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []Token{}, nil
		} else {
			w.log.Error("Failed to get tokens to be synced", "err", err)
			return nil, err
		}
	}
	return tokensList, nil
}

func (w *Wallet) UpdateTokenSyncStatus(tokenID string, syncStatus int) error {
	if syncStatus < SyncUnrequired || syncStatus > SyncCompleted {
		return fmt.Errorf("invalid sync status, cannot update")
	}
	var tokenInfo Token
	err := w.s.Read(TokenStorage, &tokenInfo, " token_id = ?", tokenID)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return err
		} else {
			w.log.Error("Failed to get token states", "err", err)
			return err
		}
	}
	if tokenInfo.SyncStatus == syncStatus {
		return nil
	}
	tokenInfo.SyncStatus = syncStatus
	err = w.s.Update(TokenStorage, &tokenInfo, "token_id=?", tokenID)
	if err != nil {
		w.log.Error("Failed to update token sync status", "err", err)
		return err
	}
	return nil
}

// func (w *Wallet) AddTokenDetailsToTokensTable(tokenDetails Token) error {
// 	// w.l.Lock()
// 	// defer w.l.Unlock()
// 	err := w.s.Write(TokenStorage, tokenDetails)
// 	if err != nil {
// 		w.log.Error("failed to write to db, token ", tokenDetails.TokenID)
// 		return err
// 	}
// 	return nil
// }

func (w *Wallet) GetLockedFTs() ([]FTToken, error) {
	var ftTokens []FTToken
	err := w.s.Read(FTTokenStorage, &ftTokens, "token_status = ?", TokenIsLocked)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []FTToken{}, nil
		} else {
			w.log.Error("Failed to get locked FTs", "err", err)
			return nil, err
		}
	}
	return ftTokens, nil
}

// This function is used by fullnode to write all synced RBTs to sqlite table
func (w *Wallet) AddSyncedRBTToTable(t *SyncedRBT) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.fullNodeSQLDB.Write(FullNodeRBTTable, t)
	if err == nil {
		go w.notifyTokenUpdate(FullNodeRBTTable, t)
	}
	return err
}

// This function is used by fullnode to write all synced FTs to sqlite table
func (w *Wallet) AddSyncedFTToTable(t *SyncedFT) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.fullNodeSQLDB.Write(FullNodeFTTable, t)
	if err == nil {
		go w.notifyTokenUpdate(FullNodeFTTable, t)
	}
	return err
}

// This function is used by fullnode to write all synced NFTs to sqlite table
func (w *Wallet) AddSyncedNFTToTable(t *SyncedNFT) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.fullNodeSQLDB.Write(FullNodeNFTTable, t)
	if err == nil {
		go w.notifyTokenUpdate(FullNodeNFTTable, t)
	}
	return err
}

func (w *Wallet) AddSyncedSmartContractToTable(t *SyncedSmartContract) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.fullNodeSQLDB.Write(FullNodeSmartContractTable, t)
	if err == nil {
		go w.notifyTokenUpdate(FullNodeSmartContractTable, t)
	}
	return err
}

func (w *Wallet) AddFailedTokensToTable(t *model.FailedToSyncTokenDetailsInfo) error {
	w.l.Lock()
	defer w.l.Unlock()
	err := w.fullNodeSQLDB.Write(FullNodeFailedToSyncTokens, t)
	if err == nil {
		go w.notifyTokenUpdate(FullNodeFailedToSyncTokens, t)
	}
	return err
}

// This function is used by fullnode to read from the list of all synced RBTs
func (w *Wallet) ReadSyncedRBTFromTable(tokenId string) (*SyncedRBT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var rbt SyncedRBT
	err := w.fullNodeSQLDB.Read(FullNodeRBTTable, &rbt, "token_id=?", tokenId)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get rbt, err : %v", err)
		w.log.Warn(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	return &rbt, nil
}

// This function is used by fullnode to read from the list of all synced FTs
func (w *Wallet) ReadSyncedFTFromTable(tokenId string) (*SyncedFT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var ft SyncedFT
	err := w.fullNodeSQLDB.Read(FullNodeFTTable, &ft, "token_id=?", tokenId)
	if err != nil {
		w.log.Warn("Failed to get ft", "err", err)
		return nil, err
	}
	return &ft, nil
}

// This function is used by fullnode to read from the list of all synced NFTs
func (w *Wallet) ReadSyncedNFTFromTable(tokenId string) (*SyncedNFT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var nft SyncedNFT
	err := w.fullNodeSQLDB.Read(FullNodeNFTTable, &nft, "token_id=?", tokenId)
	if err != nil {
		w.log.Warn("Failed to get nft", "err", err)
		return nil, err
	}
	return &nft, nil
}

// This function is used by fullnode to read from the list of all synced smart contracts
func (w *Wallet) ReadSyncedSmartContractFromTable(contractHash string) (*SyncedSmartContract, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var sc SyncedSmartContract
	err := w.fullNodeSQLDB.Read(FullNodeSmartContractTable, &sc, "smart_contract_hash=?", contractHash)
	if err != nil {
		w.log.Warn("Failed to get sc", "err", err)
		return nil, err
	}
	return &sc, nil
}

func (w *Wallet) ReadFailedToSyncTokensFromTable(tokenID string) (*model.FailedToSyncTokenDetailsInfo, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var token model.FailedToSyncTokenDetailsInfo
	err := w.fullNodeSQLDB.Read(FullNodeFailedToSyncTokens, &token, "token_id=?", tokenID)
	if err != nil {
		w.log.Warn("Failed to get sc", "err", err)
		return nil, err
	}
	return &token, nil
}

func (w *Wallet) DeleteFailedToSyncTokenFromTable(tokenID string) error {
	w.log.Debug("****Calling DeleteFailedToSyncTokenFromTable for the token: ******", tokenID)

	token, err := w.ReadFailedToSyncTokensFromTable(tokenID)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return nil
		}
		w.log.Error("faile to read FullNodeFailedToSyncTokens table", "token_id", tokenID, "error", err)
		return err
	}

	w.l.Lock()
	defer w.l.Unlock()

	deleteErr := w.fullNodeSQLDB.Delete(FullNodeFailedToSyncTokens, &token, "token_id=?", tokenID)
	if deleteErr != nil {
		w.log.Error("Failed to delete token from FullNodeFailedToSyncTokens table", "token_id", tokenID, "error", deleteErr)
		return deleteErr
	}

	w.log.Info("Successfully deleted token from FullNodeFailedToSyncTokens table", "token_id", tokenID)
	return nil
}

// This function is used by fullnode to update synced RBTs
func (w *Wallet) UpdateSyncedRBTToTable(rbt *SyncedRBT) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Update(FullNodeRBTTable, &rbt, "token_id=?", rbt.TokenID)
}

// This function is used by fullnode to update synced FTs
func (w *Wallet) UpdateSyncedFTToTable(ft *SyncedFT) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Update(FullNodeFTTable, &ft, "token_id=?", ft.TokenID)
}

// This function is used by fullnode to update synced NFTs
func (w *Wallet) UpdateSyncedNFTToTable(nft *SyncedNFT) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Update(FullNodeNFTTable, &nft, "token_id=?", nft.TokenID)
}

// This function is used by fullnode to update synced smart contracts
func (w *Wallet) UpdateSyncedSmartContractToTable(sc *SyncedSmartContract) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Update(FullNodeSmartContractTable, &sc, "smart_contract_hash=?", sc.SmartContractHash)
}

// This function is used by fullnode to delete synced RBTs
func (w *Wallet) RemoveSyncedRBTFromTable(tokenID string) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Delete(FullNodeRBTTable, &SyncedRBT{}, "token_id=?", tokenID)
}

// This function is used by fullnode to delete synced FTs
func (w *Wallet) RemoveSyncedFTFromTable(tokenID string) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Delete(FullNodeFTTable, &SyncedFT{}, "token_id=?", tokenID)
}

// This function is used by fullnode to delete synced NFTs
func (w *Wallet) RemoveSyncedNFTFromTable(tokenID string) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Delete(FullNodeNFTTable, &SyncedNFT{}, "token_id=?", tokenID)
}

// This function is used by fullnode to delete synced smart contracts
func (w *Wallet) RemoveSyncedSmartContractFromTable(smartContractHash string) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Delete(FullNodeSmartContractTable, &SyncedSmartContract{}, "smart_contract_hash=?", smartContractHash)
}

// This function will return array of RBTs which are free (Used in de-exp)
func (w *Wallet) GetAllRBTs() ([]SyncedRBT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var RBTs []SyncedRBT
	err := w.fullNodeSQLDB.Read(FullNodeRBTTable, &RBTs, "token_id!=?", "")
	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No free RBTs")
			return nil, fmt.Errorf("Failed to get tokens, No free RBTs")
		}
		w.log.Error("Failed to get RBTs", "err", err)
		return nil, err
	}
	return RBTs, nil
}

func (w *Wallet) GetAllFTs() ([]SyncedFT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var FT []SyncedFT
	err := w.fullNodeSQLDB.Read(FullNodeFTTable, &FT, "token_id!=?", "")
	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No free FTs")
			return nil, err
		}
		w.log.Error("Failed to get FTs", "err", err)
		return nil, err
	}
	return FT, nil
}

// function to return all the NFTs from the fullnode database
func (w *Wallet) GetAllNFTs() ([]SyncedNFT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var NFT []SyncedNFT
	err := w.fullNodeSQLDB.Read(FullNodeNFTTable, &NFT, "token_id!=?", "")
	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No free NFTs")
			return nil, err
		}
		w.log.Error("Failed to get NFTs", "err", err)
		return nil, err
	}
	return NFT, nil
}

// function to return all the smart contracts from the fullnode database
func (w *Wallet) GetAllSmartContracts() ([]SyncedSmartContract, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var SC []SyncedSmartContract
	err := w.fullNodeSQLDB.Read(FullNodeSmartContractTable, &SC, "smart_contract_hash!=?", "")
	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No free Smart Contracts")
			return nil, err
		}
		w.log.Error("Failed to get Smart Contracts", "err", err)
		return nil, err
	}
	return SC, nil
}

func (w *Wallet) GetAllRBTbyDID(did string) ([]SyncedRBT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []SyncedRBT
	err := w.fullNodeSQLDB.Read(FullNodeRBTTable, &t, "owner_did=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

func (w *Wallet) GetAllFTsbyDID(did string) ([]FTToken, error) {
	var t []FTToken
	err := w.s.Read(FTTokenStorage, &t, "owner_did=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

func (w *Wallet) GetAllNFTsbyDID(did string) ([]SyncedNFT, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []SyncedNFT
	err := w.fullNodeSQLDB.Read(FullNodeNFTTable, &t, "owner_did=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

func (w *Wallet) GetAllSmartContractsbyDID(did string) ([]SyncedSmartContract, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var t []SyncedSmartContract
	err := w.fullNodeSQLDB.Read(FullNodeSmartContractTable, &t, "deployer=?", did)
	if err != nil {
		w.log.Error("Failed to get tokens", "err", err)
		return nil, err
	}
	return t, nil
}

// func (w *Wallet) UpdateFailedToSyncTokensFromTable(token *model.FailedToSyncTokenDetailsInfo) error {
// 	w.l.Lock()
// 	defer w.l.Unlock()
// 	return w.fullNodeSQLDB.Update(FullNodeFailedToSyncTokens, &token, "token=?", token.Token)
// }

// Store failed transactions in fullnode DB for later analysis and retry
func (w *Wallet) StoreFailedTransaction(failedTxn *model.FailedTransaction) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Write(FailedTxnsTable, failedTxn)
}



// Store double spent tokens in fullnode DB for later analysis
func (w *Wallet) AddDoubleSpentTokenInfo(doubleSpentTokenInfo *model.DoubleSpentTokenInfo) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Write(FullnodeDoubleSpentTokensTable, doubleSpentTokenInfo)
}

// Store double spent tokens in fullnode DB for later analysis
func (w *Wallet) UpdateDoubleSpentTokenInfo(doubleSpentTokenInfo *model.DoubleSpentTokenInfo) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodeSQLDB.Update(FullnodeDoubleSpentTokensTable, &doubleSpentTokenInfo, "token_id=?", doubleSpentTokenInfo.TokenID)
}

// Store double spent tokens in fullnode DB for later analysis
func (w *Wallet) ReadDoubleSpentTokenInfo(doubleSpentTokenID string) (*model.DoubleSpentTokenInfo, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var doubleSpentTokenInfo model.DoubleSpentTokenInfo
	err := w.fullNodeSQLDB.Read(FullnodeDoubleSpentTokensTable, &doubleSpentTokenInfo, "token_id=?", doubleSpentTokenID)
	if err != nil {
		w.log.Warn("Failed to read double spent token from table", "err", err)
		return nil, err
	}
	return &doubleSpentTokenInfo, nil
}

// This function is used by fullnode to write all synced RBTs' IPFS content to sqlite table
func (w *Wallet) AddRBTContentToPSQl(rbt *RBTContent) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodePSQLTokensDB.Write(FullNodeRBTContentTable, rbt)
}

// This function is used by fullnode to write all synced FTs' IPFS content to sqlite table
func (w *Wallet) AddFTContentToPSQl(ft *FTContent) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodePSQLTokensDB.Write(FullNodeFTContentTable, ft)
}

// This function is used by fullnode to write all synced NFTs' IPFS content to sqlite table
func (w *Wallet) AddNFTContentToPSQl(nft *NFTContent) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodePSQLTokensDB.Write(FullNodeNFTContentTable, nft)
}

// This function is used by fullnode to write all synced SmartContracts' IPFS content to sqlite table
func (w *Wallet) AddSmartContractContentToPSQl(smartContract *SmartContractContent) error {
	w.l.Lock()
	defer w.l.Unlock()
	return w.fullNodePSQLTokensDB.Write(FullNodeSCContentTable, smartContract)
}

// This function is used by fullnode to read from the list of all RBTs' IPFS content
func (w *Wallet) ReadRBTContentFromTable(tokenId string) (*RBTContent, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var rbt RBTContent
	err := w.fullNodePSQLTokensDB.Read(FullNodeRBTContentTable, &rbt, "token_id=?", tokenId)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get rbt, err : %v", err)
		// w.log.Error(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	return &rbt, nil
}

// This function is used by fullnode to read from the list of all FTs' IPFS content
func (w *Wallet) ReadFTContentFromTable(tokenId string) (*FTContent, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var ft FTContent
	err := w.fullNodePSQLTokensDB.Read(FullNodeFTContentTable, &ft, "token_id=?", tokenId)
	if err != nil {
		// w.log.Error("Failed to get ft", "err", err)
		return nil, err
	}
	return &ft, nil
}

// This function is used by fullnode to read from the list of all NFTs' IPFS content
func (w *Wallet) ReadNFTContentFromTable(tokenId string) (*NFTContent, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var nft NFTContent
	err := w.fullNodePSQLTokensDB.Read(FullNodeNFTContentTable, &nft, "nft_id=?", tokenId)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get rbt, err : %v", err)
		// w.log.Error(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	return &nft, nil
}

// This function is used by fullnode to read from the list of all SmartContracts' IPFS content
func (w *Wallet) ReadSmartContractContentFromTable(tokenId string) (*SmartContractContent, error) {
	w.l.Lock()
	defer w.l.Unlock()
	var scContent SmartContractContent
	err := w.fullNodePSQLTokensDB.Read(FullNodeSCContentTable, &scContent, "smart_contract_hash=?", tokenId)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get rbt, err : %v", err)
		// w.log.Error(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	return &scContent, nil
}

func (w *Wallet) GetAllFailedToSyncTokens() ([]*model.FailedToSyncTokenDetailsInfo, error) {
	w.l.Lock()
	defer w.l.Unlock()

	var tokens []*model.FailedToSyncTokenDetailsInfo
	err := w.fullNodeSQLDB.Read(FullNodeFailedToSyncTokens, &tokens, "1=1")
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (w *Wallet) GetTxnAmountFromFullNode(txnID string) (*model.FullNodeTxnHistoryInfo, error) {
	w.l.Lock()
	defer w.l.Unlock()

	var txnAmountInfo *model.FullNodeTxnHistoryInfo
	err := w.fullNodeSQLDB.Read(FullNodeTxnHistoryTable, &txnAmountInfo, "transaction_id=?", txnID)
	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No record found")
			return nil, err
		}
		w.log.Error("Failed to get txn amount", "err", err)
		return nil, err
	}

	return txnAmountInfo, nil
}
