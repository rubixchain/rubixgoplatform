package wallet

import (
	"bytes"
	"fmt"
	"os"
	"strings"

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
	TokenID        string  `gorm:"column:token_id;primaryKey"`
	ParentTokenID  string  `gorm:"column:parent_token_id"`
	TokenValue     float64 `gorm:"column:token_value"`
	DID            string  `gorm:"column:did"`
	TokenStatus    int     `gorm:"column:token_status;"`
	TokenStateHash string  `gorm:"column:token_state_hash"`
	TransactionID  string  `gorm:"column:transaction_id"`
	Added          bool    `gorm:"column:added"`
	SyncStatus     int     `gorm:"column:sync_status"`
}

func (w *Wallet) CreateToken(t *Token) error {
	return w.s.Write(TokenStorage, t)
}
func (w *Wallet) CreateFT(ft *FTToken) error {
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

	for _, info := range ti {
		t := info.Token
		b := w.GetLatestTokenBlock(info.Token, info.TokenType)
		blockId, _ := b.GetBlockID(t)
		tokenIDTokenStateData := t + blockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
		tokenIDTokenStateHash, _ := ipfsShell.Add(tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		updatedtokenhashes = append(updatedtokenhashes, tokenIDTokenStateHash)
		tokenHashMap[t] = tokenIDTokenStateHash
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
			}

			err = w.s.Write(TokenStorage, &t)
			if err != nil {
				fmt.Println("failed to write to db, token ", tokenInfo.Token)
				return nil, err
			}
		}
		// Update token status and pin tokens
		tokenStatus := TokenIsFree
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
		//Pinnig the whole tokens and pat tokens
		ok, err := w.Pin(tokenInfo.Token, role, did, b.GetTid(), senderAddress, receiverAddress, tokenInfo.TokenValue)
		if err != nil {
			fmt.Println("failed to pin token ", tokenInfo.Token)
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("failed to pin token")
		}
	}
	return updatedtokenhashes, nil
}

// need to update in such a way that only for FTs
func (w *Wallet) FTTokensReceived(did string, ti []contract.TokenInfo, b *block.Block, senderPeerId string, receiverPeerId string, ipfsShell *ipfsnode.Shell, ftInfo FTToken) ([]string, error) {
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

	for _, info := range ti {
		t := info.Token
		b := w.GetLatestTokenBlock(info.Token, info.TokenType)
		blockId, _ := b.GetBlockID(t)
		tokenIDTokenStateData := t + blockId
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))
		tokenIDTokenStateHash, _ := ipfsShell.Add(tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		updatedtokenhashes = append(updatedtokenhashes, tokenIDTokenStateHash)
		tokenHashMap[t] = tokenIDTokenStateHash
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
			}

			err = w.s.Write(FTTokenStorage, &FTInfo)
			if err != nil {
				return nil, err
			}
		}
		// Update token status and pin tokens
		tokenStatus := TokenIsFree
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
		//Pinnig the whole tokens and pat tokens
		ok, err := w.Pin(tokenInfo.Token, role, did, b.GetTid(), senderAddress, receiverAddress, tokenInfo.TokenValue)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("failed to pin token")
		}
	}
	return updatedtokenhashes, nil
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
