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

func planTokenSplit(tokenID TokenID, needed float64, log logger.Logger) ([]SplitOp, error) {
	var splits []SplitOp

	currentTokenID := tokenID
	currentNeeded := needed
	zeroFloat := rubixmath.FloatPrecision(0)
	currentTokenElems, err := util.GetRbtIDElements(string(currentTokenID))
	if err != nil {
		return nil, fmt.Errorf("planSplit: invalid token id: %v, err: %v", string(tokenID), err)
	}

	var currentLevel int

	for currentNeeded > zeroFloat {
		currentLevel = currentTokenElems.Level

		if currentLevel >= GetMaxDenomTreeLevel()-1 {
			return nil, fmt.Errorf("planSplit: invalid level for token: %v, got level: %v", string(tokenID), currentLevel)
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

// performTokenSplit - transferFree, transferBurn, keepFree, keepBurnt
func performTokenSplit(w *wallet.Wallet, dc types.DIDCrypto,
	splitOp SplitOp, tokenCache map[string]models.Token, tokenDenomArr map[types.DenomValue]types.DenomCount,
) (freeTokens []models.Token, keepTokens []models.Token, burnTokens []models.Token, err error) {
	freeTokens = make([]models.Token, 0)
	keepTokens = make([]models.Token, 0)
	burnTokens = make([]models.Token, 0)

	var parentToken models.Token
	// parentTokenHeirarchicalID := splitOp.TokenID
	parentTokenID := string(splitOp.TokenID)

	if cachedToken, exists := tokenCache[parentTokenID]; exists {
		parentToken = cachedToken
	} else {
		var err error
		parentToken, err = w.GetRBTToken(parentTokenID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unable to get info of parent token: %v, err: %v", parentTokenID, err)
		}
	}

	// get parent token elements
	parentElems, err := util.GetRbtIDElements(parentTokenID)
	if err != nil {
		return nil,nil, nil, fmt.Errorf("performTokenSplit: failed to split elements of parent token %s; err: %w", parentTokenID, err)
	} 
	parentDenomLevel, err := util.GetTreeLevelFromPartIndex(parentElems.PartIndex)
	childLevel := parentDenomLevel + 1

	childTokensCreatedMap, err := createChildTokensAtLevel(dc, w, splitOp.TokenID, childLevel, tokenDenomArr)
	if err != nil {
		return nil, nil, nil, err
	}

	err = burnParentToken(dc, w, parentToken.TokenID, parentToken.TokenValue, dc.GetDID(), tokenDenomArr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenID, dc.GetDID(), err)
	}

	for _, transferIdx := range splitOp.ChildrenToTransfer {
		childTokenToTransfer, exists := childTokensCreatedMap[transferIdx]
		if !exists {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v", transferIdx, parentTokenID)
		}

		freeTokens = append(freeTokens, childTokenToTransfer)
	}

	for _, keepIdx := range splitOp.ChildrenToKeep {
		childTokenToKeep, exists := childTokensCreatedMap[keepIdx]
		if !exists {
			return nil, nil, nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v to keep", keepIdx, parentTokenID)
		}

		keepTokens = append(keepTokens, childTokenToKeep)
	}

	burnTokens = append(burnTokens, parentToken)

	return freeTokens, burnTokens, burnTokens, nil
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

func createChildTokensAtLevel(dc types.DIDCrypto, w *wallet.Wallet, parentTokenID TokenID,
	level int, tokenDenomArr map[types.DenomValue]types.DenomCount,
) (map[int]models.Token, error) {
	var childTokenIndexMap map[int]models.Token = make(map[int]models.Token)

	maxTokenCount := MaxTokensAtLevel(level)

	childTokenValue, err := util.LevelToDenom(level)
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch the denom for level: %v, err: %v", level, err)
	}

	did := dc.GetDID()

	// get children index range of parent token
	childrenIndexRange, err := parentTokenID.GetChildrenIndexRange()
	if err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to fetch the children index rangfe for parent: %v, err: %v", parentTokenID, err)
	}

	// Get Parent Token Details
	for index := childrenIndexRange.First; index <= childrenIndexRange.Last; index++ {
		childTokenID := fmt.Sprintf("%s_%d", parentTokenID,  index)

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
				String: parentTokenID.String(),
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

		// the key in the map represents child position among all children
		childTokenIndexMap[index - childrenIndexRange.First + 1] = childToken
	}

	tokenDenomArr[childTokenValue] += int64(maxTokenCount)

	if err := w.UpdateTokenDenomArray(did, tokenDenomArr); err != nil {
		return nil, fmt.Errorf("createChildTokensAtLevel: failed to update token denom")
	}

	return childTokenIndexMap, nil
}
