package parts

import (
	"bytes"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func planTokenSplit(heirarchicalID TokenID, needed float64, log logger.Logger) ([]SplitOp, error) {
	var splits []SplitOp

	currentTokenID := heirarchicalID
	currentNeeded := needed
	zeroFloat := rubixmath.FloatPrecision(0)

	var currentLevel int

	for currentNeeded > zeroFloat {
		currentLevel = currentTokenID.Level()

		if currentLevel >= GetMaxDenomTreeLevel()-1 {
			return nil, fmt.Errorf("planSplit: invalid level for token: %v, got level: %v", string(heirarchicalID), currentLevel)
		}

		childLevel := currentLevel + 1
		childValue, err := wallet.LevelToDenom(childLevel)
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
			HierarchicalTokenID: currentTokenID,
			ChildrenToTransfer:  childrenToTransfer,
			ChildrenToKeep:      childrenToKeep,
		})

		if needToSplitChild {
			currentTokenID = currentTokenID.Child(childToSplit)
			currentNeeded = rubixmath.FloatPrecision(float64(remainder) * getLowestPossibleDenom())
		} else {
			break
		}
	}

	return splits, nil
}

func performTokenSplit(w *wallet.Wallet, dc did.DIDCrypto,
	splitOp SplitOp, tokenCache map[string]*wallet.Token, isTestnet bool,
	tokenDenomArr []int, publishFn func(*model.PubSubTxnInfo) error,
) ([]wallet.Token, error) {
	var parentToken *wallet.Token
	var networkID string
	parentTokenHeirarchicalID := splitOp.HierarchicalTokenID
	parentTokenIndexedID, err := HeirarchicalToIndexed(parentTokenHeirarchicalID)
	if err != nil {
		return nil, fmt.Errorf("performTokenSplit: failed to convert parent token hierarchical ID to indexed ID: %v", err)
	}

	if cachedToken, exists := tokenCache[parentTokenIndexedID]; exists {
		parentToken = cachedToken
	} else {
		var err error
		parentToken, err = w.GetToken(parentTokenIndexedID, wallet.TokenIsFree)
		if err != nil {
			return nil, fmt.Errorf("performTokenSplit: unable to get parent token: %v with id: %v; indexid: %v, err: %v", splitOp.HierarchicalTokenID.String(), parentTokenIndexedID, parentTokenIndexedID, err)
		}

		parentTokenType := getTokenType(isTestnet, parentToken.TokenValue)
		
		networkID, err = w.GetTokenNetworkID(parentToken.TokenID, parentTokenType)
		if err != nil {
			return nil, fmt.Errorf("performTokenSplit: failed to get network ID for parent token: %v, err: %v", parentToken.TokenID, err)
		}
	}

	childLevel := parentTokenHeirarchicalID.Level() + 1

	childTokensCreatedMap, err := createChildTokensAtLevel(dc, w, parentTokenHeirarchicalID, parentTokenIndexedID, childLevel, isTestnet, publishFn, tokenDenomArr, networkID)
	if err != nil {
		return nil, err
	}

	childTokenIDs := func() []string {
		var tokenList []string = make([]string, 0)

		for _, token := range childTokensCreatedMap {
			tokenList = append(tokenList, token.TokenID)
		}

		return tokenList
	}()

	err = burnParentToken(dc, w, parentToken.TokenID, parentToken.TokenValue, childTokenIDs, dc.GetDID(), isTestnet, tokenDenomArr, publishFn)
	if err != nil {
		return nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenIndexedID, dc.GetDID(), err)
	}

	childTokensToTransfer := make([]wallet.Token, 0)

	for _, transferIdx := range splitOp.ChildrenToTransfer {
		childTokenToTransfer, exists := childTokensCreatedMap[transferIdx]
		if !exists {
			return nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v", transferIdx, parentTokenIndexedID)
		}

		childTokensToTransfer = append(childTokensToTransfer, childTokenToTransfer)
	}

	return childTokensToTransfer, nil
}

func createHierarchicalChildTokenContent(hierarchicalParentTokenContent string, index int) string {
	return hierarchicalParentTokenContent + "_" + fmt.Sprint(index)
}

func createChildTokenAtIndex(parentTokenHierarchicalID string, index int) (string, error) {
	hierarchicalChildTokenContent := createHierarchicalChildTokenContent(parentTokenHierarchicalID, index)

	indexedChildTokenContent, err := HeirarchicalToIndexed(TokenID(hierarchicalChildTokenContent))
	if err != nil {
		return "", fmt.Errorf("createChildToken: failed to convert hierarchical child token content to indexed, err: %v", err)
	}

	return indexedChildTokenContent, nil
}

func burnParentToken(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string,
	parentTokenValue float64, partTokenIDs []string, did string, isTestnet bool,
	tokenDenomArr []int, publishFn func(*model.PubSubTxnInfo) error,
) error {
	parentTokenType := getTokenType(isTestnet, parentTokenValue)

	parentTokenLevel, err := wallet.DenomToLevel(parentTokenValue)
	if err != nil {
		return err
	}

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
		Epoch:           int(time.Now().Unix()),
	}

	ctcb := make(map[string]*block.Block)
	ctcb[parentTokenID] = w.GetLatestTokenBlock(parentTokenID, parentTokenType)
	burntParentTokenBlock := block.CreateNewBlock(ctcb, parentTokenChainBlock)
	if burntParentTokenBlock == nil {
		return fmt.Errorf("burnParentToken: failed to create new burnt block for parent token: %v", parentTokenID)
	}

	err = burntParentTokenBlock.UpdateSignature(dc)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to sign the burnt block for parent token, err: %v", err)
	}

	err = w.AddTokenBlock(parentTokenID, burntParentTokenBlock)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to token block, err: %v", err)
	}

	burntParentTokenBlockHash, err := burntParentTokenBlock.GetHash()
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to get the hash of burnt block for parent token, err: %v", err)
	}

	burntBlockPublishInfo := &model.PubSubTxnInfo{
		BlockHash:    burntParentTokenBlockHash,
		TxnType:      parentTokenChainBlock.TransactionType,
		AssetType:    RBTTokenType,
		PublisherDID: dc.GetDID(),
		TxnBlock:     burntParentTokenBlock.GetBlock(),
	}

	err = publishFn(burntBlockPublishInfo)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to publish burnt block info for token: %v, err: %v", parentTokenID, err)
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

	parentTokenIDBuffer := bytes.NewBufferString(parentTokenID)
	parentTokenHash, err := w.Add(parentTokenIDBuffer, did, wallet.AddFunc, true)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to add parent token to ipfs: %v, err: %v", parentTokenID, err)
	}

	if _, err := w.Pin(parentTokenHash, wallet.OwnerRole, did, "NA", did, "NA", parentTokenValue); err != nil {
		return fmt.Errorf("burnParentToken: failed to pin parent token: %v, err: %v", parentTokenID, err)
	}

	// Immediate update of token denom array for the burnt token
	tokenDenomArr[parentTokenLevel] -= 1
	if err != nil {
		return fmt.Errorf(
			"burnParentToken: unable to decrement token denom array index for level %v, token %v, err: %v",
			parentTokenLevel,
			parentTokenID,
			err,
		)
	}

	return nil
}

func createChildTokensAtLevel(dc did.DIDCrypto, w *wallet.Wallet, parentTokenHierarchicalID TokenID, parenTokenIndexedID string,
	level int, isTestnet bool, publishFn func(*model.PubSubTxnInfo) error,
	tokenDenomArr []int, networkID string,
) (map[int]wallet.Token, error) {
	var childTokenIndexMap map[int]wallet.Token = make(map[int]wallet.Token)

	maxTokenCount := MaxTokensAtLevel(level)

	childTokenValue, err := wallet.LevelToDenom(level)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch the denom for level: %v, err: %v", level, err)
	}

	did := dc.GetDID()

	// Get Parent Token Details
	for index := 1; index <= maxTokenCount; index++ {
		childTokenID, err := createChildTokenAtIndex(parentTokenHierarchicalID.String(), index)
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
						ParentID: parenTokenIndexedID,
						NetworkID: networkID,
					},
				},
			},
			TokenValue: childTokenValue,
			Epoch:      int(time.Now().Unix()),
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

		partTokenBlockHash, err := childTokenBlock.GetHash()
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to get the hash of child block for child token: %v, err: %v", childTokenID, err)
		}

		partTokenWalletInfo := &model.PubSubTxnInfo{
			BlockHash:    partTokenBlockHash,
			TxnType:      tcb.TransactionType,
			AssetType:    RBTTokenType,
			PublisherDID: dc.GetDID(),
			TxnBlock:     childTokenBlock.GetBlock(),
		}

		err = publishFn(partTokenWalletInfo)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to publish part token info for token: %v, err: %v", childTokenID, err)
		}

		childTokenIDBuffer := bytes.NewBufferString(childTokenID)
		childTokenHash, err := w.Add(childTokenIDBuffer, did, wallet.AddFunc, true)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add child token to ipfs: %v, err: %v", childTokenID, err)
		} 

		if _, err := w.Pin(childTokenHash, wallet.OwnerRole, did, "NA", did, "NA", childTokenValue); err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to pin child token: %v, err: %v", childTokenID, err)
		}

		childToken := wallet.Token{
			TokenID:       childTokenID,
			ParentTokenID: parenTokenIndexedID,
			TokenValue:    childTokenValue,
			DID:           did,
			TokenStatus:   wallet.TokenIsFree,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err = w.CreateToken(&childToken)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add child token %v to DB, err: %v", childToken, err)
		}

		childTokenIndexMap[index] = childToken
	}

	tokenDenomArr[level] += maxTokenCount

	return childTokenIndexMap, nil
}
