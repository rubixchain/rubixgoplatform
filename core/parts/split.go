package parts

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
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
		childValue, err := util.LevelToDenom(childLevel)
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
		tokensBeingBurnt := make([]int, 0)

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
				tokensBeingBurnt = append(tokensBeingBurnt, i)
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

// performTokenSplit - transferFree, transferBurn, keepFree, keepBurnt
func performTokenSplit(w *wallet.Wallet, dc types.DIDCrypto,
	splitOp SplitOp, tokenCache map[string]models.Token, tokenDenomArr map[types.DenomValue]types.DenomCount,
) (freeTokens []models.Token, keepTokens []models.Token, burnTokens []models.Token, err error) {
	freeTokens = make([]models.Token, 0)
	keepTokens = make([]models.Token, 0)
	burnTokens = make([]models.Token, 0)

	var parentToken models.Token
	parentTokenHeirarchicalID := splitOp.HierarchicalTokenID
	parentTokenIndexedID, err := HeirarchicalToIndexed(parentTokenHeirarchicalID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("performTokenSplit: failed to convert parent token hierarchical ID to indexed ID: %v", err)
	}

	if cachedToken, exists := tokenCache[parentTokenIndexedID]; exists {
		parentToken = cachedToken
	} else {
		var err error
		parentToken, err = w.GetRBTToken(parentTokenIndexedID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unable to get parent token: %v with id: %v; indexid: %v, err: %v", splitOp.HierarchicalTokenID.String(), parentTokenIndexedID, parentTokenIndexedID, err)
		}

		if err := w.LockToken(parentToken); err != nil {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unable to local parent token: %v, err: %v", parentToken.TokenID, err)
		}
	}

	childLevel := parentTokenHeirarchicalID.Level() + 1

	childTokensCreatedMap, err := createChildTokensAtLevel(dc, w, parentTokenHeirarchicalID, parentTokenIndexedID, childLevel, tokenDenomArr)
	if err != nil {
		return nil, nil, nil, err
	}

	err = burnParentToken(dc, w, parentToken.TokenID, parentToken.TokenValue, dc.GetDID(), tokenDenomArr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenIndexedID, dc.GetDID(), err)
	}

	for _, transferIdx := range splitOp.ChildrenToTransfer {
		childTokenToTransfer, exists := childTokensCreatedMap[transferIdx]
		if !exists {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v", transferIdx, parentTokenIndexedID)
		}

		freeTokens = append(freeTokens, childTokenToTransfer)
	}

	for _, keepIdx := range splitOp.ChildrenToKeep {
		childTokenToKeep, exists := childTokensCreatedMap[keepIdx]
		if !exists {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v to keep", keepIdx, parentTokenIndexedID)
		}

		keepTokens = append(keepTokens, childTokenToKeep)
	}

	burnTokens = append(burnTokens, parentToken)

	return freeTokens, burnTokens, burnTokens, nil
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

func burnParentToken(dc types.DIDCrypto, w *wallet.Wallet, parentTokenID string,
	parentTokenValue float64, did string, tokenDenomArr map[types.DenomValue]types.DenomCount,
) error {
	parentTokenLevel, err := util.DenomToLevel(parentTokenValue)
	if err != nil {
		return err
	}

	err = w.BurnToken(parentTokenID)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed while updating Parent token %v status to burnt, err: %v", parentTokenID, err)
	}

	parentTokenIDBuffer := bytes.NewBufferString(parentTokenID)
	parentTokenHash, err := w.Add(parentTokenIDBuffer, did, constants.TokenProviderFunc_Add, true)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to add parent token to ipfs: %v, err: %v", parentTokenID, err)
	}

	if _, err := w.Pin(parentTokenHash, constants.TokenProviderRole_Owner, did, "NA", did, "NA", parentTokenValue); err != nil {
		return fmt.Errorf("burnParentToken: failed to pin parent token: %v, err: %v", parentTokenID, err)
	}

	// Immediate update of token denom array for the burnt token
	tokenDenomArr[parentTokenValue] -= 1
	if err != nil {
		return fmt.Errorf(
			"burnParentToken: unable to decrement token denom array index for level %v, token %v, err: %v",
			parentTokenLevel,
			parentTokenID,
			err,
		)
	}

	if err := w.UpdateTokenDenomArray(did, tokenDenomArr); err != nil {
		return fmt.Errorf("createChildTokensAtLevel: failed to update token denom")
	}

	return nil
}

func createChildTokensAtLevel(dc types.DIDCrypto, w *wallet.Wallet, parentTokenHierarchicalID TokenID, parenTokenIndexedID string,
	level int, tokenDenomArr map[types.DenomValue]types.DenomCount,
) (map[int]models.Token, error) {
	var childTokenIndexMap map[int]models.Token = make(map[int]models.Token)

	maxTokenCount := MaxTokensAtLevel(level)

	childTokenValue, err := util.LevelToDenom(level)
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

		childTokenIDBuffer := bytes.NewBufferString(childTokenID)
		childTokenHash, err := w.Add(childTokenIDBuffer, did, constants.TokenProviderFunc_Add, true)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add child token to ipfs: %v, err: %v", childTokenID, err)
		}

		if _, err := w.Pin(childTokenHash, constants.TokenProviderRole_Owner, did, "NA", did, "NA", childTokenValue); err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to pin child token: %v, err: %v", childTokenID, err)
		}

		childToken := models.Token{
			TokenID: childTokenID,
			ParentTokenID: pgtype.Text{
				String: parenTokenIndexedID,
				Valid:  true,
			},
			TokenValue:     childTokenValue,
			DID:            did,
			TokenStatus:    constants.TokenStatus_Free,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			LatestPosition: 0,
			LatestRole:     int16(models.GetTokenRoleID(constants.TokenRole_Mint)),
			TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
		}

		err = w.CreateRBTToken(childToken)
		if err != nil {
			return nil, fmt.Errorf("createChildTokensAtLevel: failed to add child token %v to DB, err: %v", childToken, err)
		}

		childTokenIndexMap[index] = childToken
	}

	tokenDenomArr[childTokenValue] += int64(maxTokenCount)

	if err := w.UpdateTokenDenomArray(did, tokenDenomArr); err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to update token denom")
	}

	return childTokenIndexMap, nil
}
