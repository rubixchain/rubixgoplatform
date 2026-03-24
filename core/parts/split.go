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

// performTokenSplit executes a single split operation, performing IPFS uploads for
// child tokens and the burned parent. Returns the resulting tokens and GenesisMintRecord
// stubs for the child tokens (TransactionID is NOT set yet — the caller sets it after
// computing the genesis txID from createGenesisTransaction).
func performTokenSplit(w *wallet.Wallet, dc types.DIDCrypto,
	splitOp SplitOp, tokenCache map[string]models.Token,
	tokenDenomArr map[types.DenomValue]types.DenomCount,
	network string,
) (freeTokens []models.Token, keepTokens []models.Token, burnTokens []models.Token,
	childMintRecords []wallet.GenesisMintRecord, err error) {
	freeTokens = make([]models.Token, 0)
	keepTokens = make([]models.Token, 0)
	burnTokens = make([]models.Token, 0)
	childMintRecords = make([]wallet.GenesisMintRecord, 0)

	var parentToken models.Token
	parentTokenID := string(splitOp.TokenID)

	if cachedToken, exists := tokenCache[parentTokenID]; exists {
		parentToken = cachedToken
	} else {
		var fetchErr error
		parentToken, fetchErr = w.GetRBTToken(parentTokenID)
		if fetchErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("performTokenSplit: unable to get parent token: %v, err: %v", parentTokenID, fetchErr)
		}

		if lockErr := w.LockToken(parentToken); lockErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("performTokenSplit: unable to lock parent token: %v, err: %v", parentToken.TokenID, lockErr)
		}
	}

	// get parent token elements
	parentElems, err := util.GetRbtIDElements(parentTokenID)
	if err != nil {
		return nil,nil, nil, nil, fmt.Errorf("performTokenSplit: failed to split elements of parent token %s; err: %w", parentTokenID, err)
	} 
	parentDenomLevel, err := util.GetTreeLevelFromPartIndex(parentElems.PartIndex)
	childLevel := parentDenomLevel + 1

	childTokensCreatedMap, err := createChildTokensAtLevel(dc, w, splitOp.TokenID, childLevel, tokenDenomArr)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	err = burnParentToken(dc, w, parentToken.TokenID, parentToken.TokenValue, dc.GetDID(), tokenDenomArr)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to burn part token: %v of did: %v, err: %v", parentTokenID, dc.GetDID(), err)
	}

	mintRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Mint))

	for _, transferIdx := range splitOp.ChildrenToTransfer {
		childTokenToTransfer, exists := childTokensCreatedMap[transferIdx]
		if !exists {
			return nil, nil, nil, nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v", transferIdx, parentTokenID)
		}
		freeTokens = append(freeTokens, childTokenToTransfer)
		// Build a partial GenesisMintRecord; TxRecord and TokenChain.TransactionID will be
		// filled in by the caller once the genesis txID is computed.
		childMintRecords = append(childMintRecords, wallet.GenesisMintRecord{
			Token: &models.Token{
				TokenID:        childTokenToTransfer.TokenID,
				ParentTokenID:  childTokenToTransfer.ParentTokenID,
				TokenValue:     childTokenToTransfer.TokenValue,
				TokenStatus:    childTokenToTransfer.TokenStatus,
				DID:            childTokenToTransfer.DID,
				TokenStateHash: childTokenToTransfer.TokenStateHash,
				TokenType:      childTokenToTransfer.TokenType,
				LatestPosition: childTokenToTransfer.LatestPosition,
				LatestRole:     childTokenToTransfer.LatestRole,
			},
			TokenChain: &models.TokenChain{
				TokenID:               childTokenToTransfer.TokenID,
				Position:              0,
				Role:                  mintRoleID,
				PreviousTransactionID: nil,
			},
		})
	}

	for _, keepIdx := range splitOp.ChildrenToKeep {
		childTokenToKeep, exists := childTokensCreatedMap[keepIdx]
		if !exists {
			return nil, nil, nil, nil, fmt.Errorf("performTokenSplit: unexpected error: unable to fetch token at index: %v for parent token: %v to keep", keepIdx, parentTokenID)
		}
		keepTokens = append(keepTokens, childTokenToKeep)
		childMintRecords = append(childMintRecords, wallet.GenesisMintRecord{
			Token: &models.Token{
				TokenID:        childTokenToKeep.TokenID,
				ParentTokenID:  childTokenToKeep.ParentTokenID,
				TokenValue:     childTokenToKeep.TokenValue,
				TokenStatus:    childTokenToKeep.TokenStatus,
				DID:            childTokenToKeep.DID,
				TokenStateHash: childTokenToKeep.TokenStateHash,
				TokenType:      childTokenToKeep.TokenType,
				LatestPosition: childTokenToKeep.LatestPosition,
				LatestRole:     childTokenToKeep.LatestRole,
			},
			TokenChain: &models.TokenChain{
				TokenID:               childTokenToKeep.TokenID,
				Position:              0,
				Role:                  mintRoleID,
				PreviousTransactionID: nil,
			},
		})
	}

	burnTokens = append(burnTokens, parentToken)

	return freeTokens, burnTokens, burnTokens, childMintRecords, nil
}

func burnParentToken(dc types.DIDCrypto, w *wallet.Wallet, parentTokenID string,
	parentTokenValue float64, did string, tokenDenomArr map[types.DenomValue]types.DenomCount,
) error {
	// NOTE: w.BurnToken (DB write) removed — DB burn is now the caller's responsibility
	// via PersistGenesisBatch. Only IPFS upload is performed here.

	parentTokenIDBuffer := bytes.NewBufferString(parentTokenID)
	parentTokenHash, err := w.Add(parentTokenIDBuffer, did, constants.TokenProviderFunc_Add, true)
	if err != nil {
		return fmt.Errorf("burnParentToken: failed to add parent token to ipfs: %v, err: %v", parentTokenID, err)
	}

	if _, err := w.Pin(parentTokenHash, constants.TokenProviderRole_Owner, did, "NA", did, "NA", parentTokenValue); err != nil {
		return fmt.Errorf("burnParentToken: failed to pin parent token: %v, err: %v", parentTokenID, err)
	}

	// In-memory denom decrement for subsequent split planning (no DB write).
	tokenDenomArr[parentTokenValue] -= 1

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
			TokenStateHash: childTokenHash, // CRITICAL: explicit IPFS CID assignment after w.Pin
			DID:            did,
			TokenStatus:    constants.TokenStatus_Free,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			LatestPosition: 0,
			LatestRole:     int16(models.GetTokenRoleID(constants.TokenRole_Mint)),
			TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
		}

 		// NOTE: w.CreateRBTToken (DB write) removed — DB insert is now the caller's
		// responsibility via PersistGenesisBatch.
		// the key in the map represents child position among all children
		childTokenIndexMap[index - childrenIndexRange.First + 1] = childToken
	}

	// In-memory denom increment for subsequent split planning (no DB write).
	tokenDenomArr[childTokenValue] += int64(maxTokenCount)

	// NOTE: w.UpdateTokenDenomArray (DB write) removed — denom update is performed
	// atomically inside PersistGenesisBatch.

	return childTokenIndexMap, nil
}
