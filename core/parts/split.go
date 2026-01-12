package parts

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/coin"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
)

func planSplit(ipfsOps IPFSOperation, heirarchicalID TokenID, needed float64) ([]SplitOp, error) {
	var splits []SplitOp

	currentTokenID := heirarchicalID
	currentNeeded := needed
	zeroFloat := floatPrecision(0, coin.MaxSupportedDecimalPlaces)

	for currentNeeded > 0 {
		currentLevel := currentTokenID.Level()
		if currentLevel >= wallet.GetMaxLevel(coin.MaxSupportedDecimalPlaces)-1 {
			return nil, fmt.Errorf("planSplit: invalid level for token: %v, got level: %v", string(heirarchicalID), currentLevel)
		}

		childLevel := currentLevel + 1
		childValue, err := wallet.IdxToDenom(childLevel)
		if err != nil {
			return nil, fmt.Errorf("planSplit: err occured while getting current child value: %v", err)
		}
		splitFactor := SplitFactor(childLevel)

		childrenNeeded := int(currentNeeded / childValue)
		remainder, err := floatModulo(currentNeeded, childValue)
		if err != nil {
			return nil, fmt.Errorf("planSplit: module operation failed: %v", err)
		}
		if remainder != 0 {
			childrenNeeded++
		}

		if childrenNeeded > splitFactor {
			childrenNeeded = splitFactor
		}

		childrenToTransfer := make([]int, 0, childrenNeeded)
		childrenToKeep := make([]int, 0, splitFactor-childrenNeeded)

		for i := 1; i <= splitFactor; i++ {
			if i <= childrenNeeded {
				childrenToTransfer = append(childrenToTransfer, i)
			} else {
				childrenToKeep = append(childrenToKeep, i)
			}
		}

		splits = append(splits, SplitOp{
			TokenID:            currentTokenID,
			ChildrenToTransfer: childrenToTransfer,
			ChildrenToKeep:     childrenToKeep,
		})

		wholeChildrenValue := floatMultiply(childValue, (childrenNeeded - 1))
		remainingAfterWholeChildren := currentNeeded - wholeChildrenValue

		if remainingAfterWholeChildren < childValue && remainingAfterWholeChildren > zeroFloat {
			currentTokenID = heirarchicalID.Child(childrenNeeded)
			currentNeeded = remainingAfterWholeChildren
		} else {
			break
		}
	}

	return splits, nil
}

func splitAtLevel(w *wallet.Wallet, dc did.DIDCrypto, ipfsOps IPFSOperation, splitOp SplitOp, tokenCache map[string]*wallet.Token, isTestnet bool) ([]wallet.Token, error) {
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

	childTokensCreatedMap, err := createChildTokenForLevel(dc, w, parentToken.TokenID, childLevel, ipfsOps, isTestnet)
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

func SplitFactor(level int) int {
	if level%2 == 1 {
		return 2
	}

	return 5
}
