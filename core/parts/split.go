package parts

import (
	"bytes"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func planTokenSplit(heirarchicalID TokenID, needed float64, log logger.Logger) ([]SplitOp, error) {
	var splits []SplitOp

	currentTokenID := heirarchicalID
	currentNeeded := needed
	zeroFloat := FloatPrecision(0)

	var currentLevel int

	for currentNeeded > zeroFloat {
		currentLevel = currentTokenID.Level()

		if currentLevel >= GetMaxDenomTreeLevel()-1 {
			return nil, fmt.Errorf("planSplit: invalid level for token: %v, got level: %v", string(heirarchicalID), currentLevel)
		}

		childLevel := currentLevel + 1
		childValue, err := LevelToDenom(childLevel)
		if err != nil {
			return nil, fmt.Errorf("planSplit: err occured while getting current child value: %v", err)
		}
		splitFactor := MaxTokensAtLevel(childLevel)

		childrenNeeded, err := scaledFloatDiv(currentNeeded, childValue)
		if err != nil {
			return nil, fmt.Errorf("planSplit: div operation failed: %v", err)
		}

		remainder, err := scaledMod(currentNeeded, childValue)
		if err != nil {
			return nil, fmt.Errorf("planSplit: module operation failed: %v", err)
		}

		if childrenNeeded > splitFactor {
			childrenNeeded = splitFactor
		}

		childrenToTransfer := make([]int, 0)
		childrenToKeep := make([]int, 0)

		needToSplitChild := (remainder > 0)
		childToSplit := 0
		if needToSplitChild {
			// We'll split the next child (after the whole children we transfer)
			childToSplit = childrenNeeded + 1
		}

		for i := 1; i <= splitFactor; i++ {
			if i <= childrenNeeded {
				childrenToTransfer = append(childrenToTransfer, i)
			} else if i == childToSplit {
				childrenToKeep = append(childrenToKeep, i)
			} else {
				childrenToKeep = append(childrenToKeep, i)
			}
		}

		splits = append(splits, SplitOp{
			TokenID:            currentTokenID,
			ChildrenToTransfer: childrenToTransfer,
			ChildrenToKeep:     childrenToKeep,
		})

		if needToSplitChild {
			currentTokenID = currentTokenID.Child(childToSplit)
			currentNeeded = FloatPrecision(float64(remainder) * getLowestPossibleDenom())
		} else {
			break
		}
	}

	return splits, nil
}

func performTokenSplit(w *wallet.Wallet, dc did.DIDCrypto, ipfsOps IPFSOperation, splitOp SplitOp, tokenCache map[string]*wallet.Token, isTestnet bool) ([]wallet.Token, error) {
	var parentToken *wallet.Token
	parentTokenHeirarchicalID := splitOp.TokenID
	parentTokenIFPSId, err := IpfsAddString(parentTokenHeirarchicalID, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("splitAtLevel: failed to get IPFS ID for Parent token: %v, err: %v", splitOp.TokenID.String(), err)
	}

	if cachedToken, exists := tokenCache[parentTokenIFPSId]; exists {
		parentToken = cachedToken
	} else {
		var err error
		parentToken, err = w.GetToken(parentTokenIFPSId, wallet.TokenIsFree)
		if err != nil {
			return nil, fmt.Errorf("splitAtLevel: unable to get parent token: %v, err: %v", splitOp.TokenID.String(), err)
		}
	}

	childLevel := parentTokenHeirarchicalID.Level() + 1

	childTokensCreatedMap, err := createChildTokensAtLevel(dc, w, parentToken.TokenID, childLevel, ipfsOps, isTestnet)
	if err != nil {
		return nil, err
	}

	childTokensToTransfer := make([]wallet.Token, 0)

	for _, transferIdx := range splitOp.ChildrenToTransfer {
		childTokenToTransfer, exists := childTokensCreatedMap[transferIdx]
		if !exists {
			return nil, fmt.Errorf("splitAtLevel: unexpected error: unable to fetch token at index: %v for parent token: %v", transferIdx, parentTokenIFPSId)
		}

		childTokensToTransfer = append(childTokensToTransfer, childTokenToTransfer)
	}

	childTokenIDs := func() []string {
		var tokenList []string = make([]string, 0)

		for _, token := range childTokensCreatedMap {
			tokenList = append(tokenList, token.TokenID)
		}

		return tokenList
	}()

	err = burnParentToken(dc, w, parentToken.TokenID, parentToken.TokenValue, childTokenIDs, dc.GetDID(), isTestnet)
	if err != nil {
		return nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenIFPSId, dc.GetDID(), err)
	}

	return childTokensToTransfer, nil
}

func createChildTokenContent(parentTokenContent string, index int) string {
	return parentTokenContent + "-" + fmt.Sprint(index)
}

// TODO: Current implementation references (c *Core).createPartToken
// which uses RAC for construction of child token. In future, the implementation
// will undergo change once its decided to move away from RAC.
func createChildTokenAtIndex(
	parentTokenID string, parentTokenContent string,
	index int, ipfsOps IPFSOperation, userDID string,
	denomValue float64,
) (string, error) {
	racType := &rac.RacType{
		Type:        7, // TODO: change this
		DID:         userDID,
		TotalSupply: 1,
		TimeStamp:   time.Now().String(),
		PartInfo: &rac.RacPartInfo{
			Parent:  parentTokenID,
			PartNum: index,
			Value:   denomValue,
		},
	}

	racBlock, err := rac.CreateRac(racType)
	if err != nil {
		return "", fmt.Errorf("createChildToken: error occured while creating RAC block, err: %v", err)
	}

	if len(racBlock) != 1 {
		return "", fmt.Errorf("createChildToken: failed to create RAC genesis block for parentToken: %v and index: %v", parentTokenID, index)
	}

	childTokenContent := createChildTokenContent(parentTokenContent, index)
	childTokenContentBuffer := bytes.NewBufferString(childTokenContent)
	childTokenHash, err := ipfsOps.Add(childTokenContentBuffer)
	if err != nil {
		return "", fmt.Errorf("createChildToken: failed to perform IPFS Add operation while fetching token hash, err: %v", err)
	}

	return childTokenHash, nil
}

func burnParentToken(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string, parentTokenValue float64, partTokenIDs []string, did string, isTestnet bool) error {
	parentTokenType := getTokenType(isTestnet, parentTokenValue)

	// Burn the parent token
	bti := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     parentTokenID,
				TokenType: parentTokenType,
			},
		},
		Comment: "Token burnt at : " + time.Now().String(),
	}

	parentTokenChainBlock := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner:      did,
		TransInfo:       bti,
		TokenValue:      parentTokenValue,
		ChildTokens:     partTokenIDs,
	}

	ctcb := make(map[string]*block.Block)
	ctcb[parentTokenID] = w.GetLatestTokenBlock(parentTokenID, parentTokenType)
	burntParentTokenBlock := block.CreateNewBlock(ctcb, parentTokenChainBlock)
	if burntParentTokenBlock == nil {
		return fmt.Errorf("burnParentToken: failed to create new burnt block for parent token: %v", parentTokenID)
	}

	err := burntParentTokenBlock.UpdateSignature(dc)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to sign the burnt block for parent token, err: %v", err)
	}

	err = w.AddTokenBlock(parentTokenID, burntParentTokenBlock)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to token block, err: %v", err)
	}

	parentTokenInfo, err := w.GetToken(parentTokenID, wallet.TokenIsLocked)
	if err != nil {
		return fmt.Errorf(`burnParentToken: unexpected error: failed to fetch parent token info from SQL, err: %v. If,
		error is 'no records found', it is possibly due to the token having invalid token_status as it,
		is expected to be Locked state`, err)
	}

	parentTokenInfo.TokenStatus = wallet.TokenIsBurnt
	err = w.UpdateToken(parentTokenInfo)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed while updating Parent token %v status to burnt, err: %v", parentTokenID, err)
	}

	return nil
}

func createChildTokensAtLevel(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string, level int, ipfsOps IPFSOperation, isTestnet bool) (map[int]wallet.Token, error) {
	var childTokenIndexMap map[int]wallet.Token = make(map[int]wallet.Token)

	maxTokenCount := MaxTokensAtLevel(level)
	childTokenValue, err := LevelToDenom(level)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch the denom for level: %v, err: %v", level, err)
	}

	did := dc.GetDID()

	// Get Parent Token Details
	parentTokenHeirarchicalID, err := IpfsCatString(parentTokenID, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to get the content for parent token: %v, err: %v", parentTokenID, err)
	}

	for i := 1; i <= maxTokenCount; i++ {
		childTokenID, err := createChildTokenAtIndex(parentTokenID, parentTokenHeirarchicalID.String(), i,
			ipfsOps, did, childTokenValue)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to create child token with ID: %v, err: %v", childTokenID, err)
		}

		childTokenType := getTokenType(isTestnet, childTokenValue)

		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     childTokenID,
					TokenType: childTokenType,
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
						Token:    childTokenID,
						ParentID: parentTokenID,
					},
				},
			},
			TokenValue: childTokenValue,
		}

		ctcb := make(map[string]*block.Block)
		ctcb[childTokenID] = nil
		childTokenBlock := block.CreateNewBlock(ctcb, tcb)
		if childTokenBlock == nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to create new block for child token: %v", childTokenID)
		}

		err = childTokenBlock.UpdateSignature(dc)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to update the signature of child Token: %v, err: %v", childTokenBlock, err)
		}

		err = w.AddTokenBlock(childTokenID, childTokenBlock)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add token block for child token: %v, err: %v", childTokenID, err)
		}

		childToken := wallet.Token{
			TokenID:       childTokenID,
			ParentTokenID: parentTokenID,
			TokenValue:    childTokenValue,
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
		}

		err = w.CreateToken(&childToken)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add child token %v to DB, err: %v", childToken, err)
		}

		childTokenIndexMap[i] = childToken
	}

	return childTokenIndexMap, nil
}
