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
			TokenID:            currentTokenID,
			ChildrenToTransfer: childrenToTransfer,
			ChildrenToKeep:     childrenToKeep,
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

func performTokenSplit(w *wallet.Wallet, dc did.DIDCrypto, ipfsOps IPFSOperation,
	splitOp SplitOp, tokenCache map[string]*wallet.Token, isTestnet bool,
	tokenDenomArr []string, publishFn func(*model.PubSubTxnInfo) error,
) ([]wallet.Token, error) {
	var parentToken *wallet.Token
	parentTokenHeirarchicalID := splitOp.TokenID
	parentTokenIndexedID, err := HeirarchicalToIndexed(parentTokenHeirarchicalID)
	if err != nil {
		return nil, fmt.Errorf("performTokenSplit: failed to convert parent token hierarchical ID to indexed ID: %v", err)
	}

	parentTokenIFPSId, err := IpfsAddString(parentTokenIndexedID, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("performTokenSplit: failed to get IPFS ID for Parent token: %v, err: %v", splitOp.TokenID.String(), err)
	}

	if cachedToken, exists := tokenCache[parentTokenIFPSId]; exists {
		parentToken = cachedToken
	} else {
		var err error
		parentToken, err = w.GetToken(parentTokenIFPSId, wallet.TokenIsFree)
		if err != nil {
			return nil, fmt.Errorf("performTokenSplit: unable to get parent token: %v, err: %v", splitOp.TokenID.String(), err)
		}
	}

	childLevel := parentTokenHeirarchicalID.Level() + 1

	childTokensCreatedMap, err := createChildTokensAtLevel(dc, w, parentToken.TokenID, childLevel, ipfsOps, isTestnet, publishFn)
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
		return nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenIFPSId, dc.GetDID(), err)
	}

	childTokensToTransfer := make([]wallet.Token, 0)

	for _, transferIdx := range splitOp.ChildrenToTransfer {
		childTokenToTransfer, exists := childTokensCreatedMap[transferIdx]
		if !exists {
			return nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v", transferIdx, parentTokenIFPSId)
		}

		childTokensToTransfer = append(childTokensToTransfer, childTokenToTransfer)
	}

	return childTokensToTransfer, nil
}

func createHierarchicalChildTokenContent(hierarchicalParentTokenContent string, index int) string {
	return hierarchicalParentTokenContent + "_" + fmt.Sprint(index)
}

func createChildTokenAtIndex(
	parentTokenContent string,
	index int, ipfsOps IPFSOperation,
) (string, error) {
	hierarchicalChildTokenContent := createHierarchicalChildTokenContent(parentTokenContent, index)

	indexedChildTokenContent, err := HeirarchicalToIndexed(TokenID(hierarchicalChildTokenContent))
	if err != nil {
		return "", fmt.Errorf("createChildToken: failed to convert hierarchical child token content to indexed, err: %v", err)
	}

	childTokenContentBuffer := bytes.NewBufferString(indexedChildTokenContent)
	childTokenHash, err := ipfsOps.Add(childTokenContentBuffer)
	if err != nil {
		return "", fmt.Errorf("createChildToken: failed to perform IPFS Add operation while fetching token hash, err: %v", err)
	}

	return childTokenHash, nil
}

func burnParentToken(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string,
	parentTokenValue float64, partTokenIDs []string, did string, isTestnet bool,
	tokenDenomArr []string, publishFn func(*model.PubSubTxnInfo) error,
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

	if _, err := w.Pin(parentTokenID, wallet.OwnerRole, did, "NA", did, "NA", parentTokenValue); err != nil {
		return fmt.Errorf("burnParentToken: failed to pin parent token: %v, err: %v", parentTokenID, err)
	}

	// Immediate update of token denom array for the burnt token
	err = wallet.DecrementTokenDenomCountAtIndex(tokenDenomArr, parentTokenLevel, 1)
	if err != nil {
		return fmt.Errorf(
			"burnParentToken: unable to decrement token denom array index for level %v, token %v, err: %v",
			parentTokenLevel,
			parentTokenID,
			err,
		)
	}

	err = w.UpdateTokenDenomRaw(tokenDenomArr, did)
	if err != nil {
		return fmt.Errorf("createChildTokensAtLevel: unable to update Token denom arr, err: %v", err)
	}

	return nil
}

func createChildTokensAtLevel(dc did.DIDCrypto, w *wallet.Wallet, parentTokenID string, level int,
	ipfsOps IPFSOperation, isTestnet bool, publishFn func(*model.PubSubTxnInfo) error,
) (map[int]wallet.Token, error) {
	var childTokenIndexMap map[int]wallet.Token = make(map[int]wallet.Token)

	maxTokenCount := MaxTokensAtLevel(level)
	childTokenValue, err := wallet.LevelToDenom(level)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch the denom for level: %v, err: %v", level, err)
	}

	did := dc.GetDID()

	// Get Parent Token Details
	parentTokenIndex, err := IpfsCatString(parentTokenID, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to get the content for parent token: %v, err: %v", parentTokenID, err)
	}

	parentTokenHeirarchicalID, err := IndexedToHierarchical(parentTokenIndex)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to convert parent token index to hierarchical ID: %v", err)
	}

	for index := 1; index <= maxTokenCount; index++ {
		childTokenID, err := createChildTokenAtIndex(parentTokenHeirarchicalID.String(), index, ipfsOps)
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

		if _, err := w.Pin(childTokenID, wallet.OwnerRole, did, "NA", did, "NA", childTokenValue); err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to pin child token: %v, err: %v", childTokenID, err)
		}

		childToken := wallet.Token{
			TokenID:       childTokenID,
			ParentTokenID: parentTokenID,
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

	// Immediate update of token denom array for the newly created tokens
	tokenDenomArr, err := w.GetTokenDenomArrForDID(did)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch token denom arr for did: %v, err: %v", did, err)
	}

	err = wallet.IncrementTokenDenomCountAtIndex(tokenDenomArr, level, maxTokenCount)
	if err != nil {
		return nil,
			fmt.Errorf(
				"createChildTokensAtLevel: unable to increment token denom array index for level %v, parent token %v, err: %v",
				level,
				parentTokenID,
				err,
			)
	}

	err = w.UpdateTokenDenomRaw(tokenDenomArr, did)
	if err != nil {
		return nil,
			fmt.Errorf("createChildTokensAtLevel: unable to update Token denom arr, err: %v", err)
	}
	return childTokenIndexMap, nil
}
