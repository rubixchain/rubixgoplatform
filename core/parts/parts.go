package parts

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func checkSufficientBalance(w *wallet.Wallet, did string, transferAmount float64) error {
	rbtBalance, err := w.GetRBTBalanceFromDenomArr(did)
	if err != nil {
		return fmt.Errorf("checkSufficientBalance: failed to fetch RBT balance for did: %v, err: %v", did, err)
	}

	if rbtBalance < transferAmount {
		return fmt.Errorf(
			"checkSufficientBalance: insufficient balance as the current balance is: %v, transfer amount: %v",
			rbtBalance,
			transferAmount,
		)
	}

	return nil
}

func CollectRBTTokens(dc types.DIDCrypto, w *wallet.Wallet, transferAmount float64,
	network string, log logger.Logger, pubsub *types.PubSub,
) ([]*models.TokenInfo, map[types.DenomValue]types.DenomCount, error) {
	var splitOps []SplitOp = make([]SplitOp, 0)
	var tokensTransfer []*models.TokenInfo = make([]*models.TokenInfo, 0)

	var did string = dc.GetDID()

	// Check if transfer amount doesn't exceed maximum supported decimal places
	decimalPlaces := strconv.FormatFloat(transferAmount, 'f', -1, 64)
	decimalPlacesStr := strings.Split(decimalPlaces, ".")
	if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > constants.MaxSupportedDecimalPlaces {
		return nil, nil, fmt.Errorf("transaction amount exceeds %v decimal places", constants.MaxSupportedDecimalPlaces)
	}

	// Check Balance
	err := checkSufficientBalance(w, did, transferAmount)
	if err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: failed while checking balance, err: %v", err)
	}

	ownedRBTTokens, _, err := w.GetFreeRBTTokens(did)
	if err != nil {
		return nil, nil, err
	}

	// Attempt to collect tokens which wouldn't require any splitting
	// by looking at user's balance denom array
	var nonSplitTokenTransfer []models.Token = make([]models.Token, 0)

	denomMap, err := w.GetTokenDenomArray(did)
	if err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: failed while fetching token denom array, err: %v", err)
	}

	nonSplitDenomArr, remainingBalanceDenomArr, remainingAmount, err := GetSplitAndNonsplitTokenDenom(denomMap, transferAmount)
	if err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: error occured while looking to fetch non-split token denom array for transfer, err: %v", err)
	}

	if len(nonSplitDenomArr) != 0 {
		nonSplitTokenTransfer, err = w.GetTokensFromDenomMap(nonSplitDenomArr, did)
		if err != nil {
			return nil, nil, fmt.Errorf("CollectRBTTokens: error occured while fetching non-split tokens for transfer, err: %v", err)
		}

		for _, nonSplit := range nonSplitTokenTransfer {
			tokensTransfer = append(tokensTransfer, &models.TokenInfo{
				TokenID:               nonSplit.TokenID,
				PreviousTransactionID: nonSplit.TransactionID,
			})
		}
	}

	var tokensToKeepList []models.Token = make([]models.Token, 0)
	var tokensBeingBurntList []models.Token = make([]models.Token, 0)

	if remainingAmount > rubixmath.ZeroFloat() {
		// For the remaining amount, proceed to build the denom tree
		// and split accordingly
		var remainingAvailableTokens []models.Token = removeTokensFromList(ownedRBTTokens, nonSplitTokenTransfer)

		// Build the tree
		tokenDenomTree, err := BuildDenomTree(remainingAvailableTokens, did)
		if err != nil {
			return nil, nil, fmt.Errorf("CollectRBTTokens: failed to get the denom tree for did: %v, err: %v", did, err)
		}

		for _, token := range tokenDenomTree.Leaves {
			if remainingAmount <= rubixmath.ZeroFloat() {
				break
			}

			heirarchicalID := token.HierarchicalID
			tokenValue := token.Token.TokenValue

			if tokenValue > remainingAmount {
				splitOp, err := planTokenSplit(heirarchicalID, remainingAmount, log)
				if err != nil {
					return nil, nil, fmt.Errorf("CollectRBTTokens: failed to plan the token split, err: %v", err)
				}

				splitOps = append(splitOps, splitOp...)
				remainingAmount = rubixmath.ZeroFloat()
			}
		}

		if remainingAmount > rubixmath.ZeroFloat() {
			return nil, nil, fmt.Errorf("CollectRBTTokens: could not satisfy transfer amount, remaining: %v", remainingAmount)
		}

		tokenCache := make(map[string]models.Token)

		// We move through the tree and burn and mint tokens and collect them in respective lists
		for _, splitOp := range splitOps {
			partTokensToTransfer, tokensToKeep, tokensBeingBurnt, err := performTokenSplit(w, dc, splitOp, tokenCache, remainingBalanceDenomArr)
			if err != nil {
				return nil, nil, fmt.Errorf("CollectRBTTokens: could not perform split at Level: %v, err: %v", splitOp.HierarchicalTokenID.Level(), err)
			}

			tokensToKeepList = append(tokensToKeepList, tokensToKeep...)
			tokensBeingBurntList = append(tokensBeingBurntList, tokensBeingBurnt...)

			for _, partToken := range partTokensToTransfer {
				tokensTransfer = append(tokensTransfer, &models.TokenInfo{
					TokenID:               partToken.TokenID,
					PreviousTransactionID: partToken.TransactionID,
				})
			}
		}
	}

	// KeepList and BurntList are compared to find any common elements.
	// If found, the element is removed from KeepList since its anyway
	// going to be burned
	commonTokens := util.FindCommonElementsInList(tokensToKeepList, tokensBeingBurntList)
	tokensToKeepList = util.RemoveElementsFromList(tokensToKeepList, commonTokens)

	transactionInfo, signature, err := createGenesisTransaction(dc, tokensToKeepList, tokensBeingBurntList, did, network)
	if err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: failed to get genesis transaction info, err: %v", err)
	}

	transaction, err := util.PublishTransaction(pubsub, transactionInfo, signature)
	if err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: failed to publish transaction, err: %v", err)
	}

	if err := storeGenesisTx(w, *transaction); err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: failed to store genesis transaction, err: %v", err)
	}

	var tokenIDsToUnlock []string = make([]string, 0)
	for _, tokenInfo := range tokensTransfer {
		tokenIDsToUnlock = append(tokenIDsToUnlock, tokenInfo.TokenID)
	}

	if err := w.LockTokensByID(tokenIDsToUnlock); err != nil {
		return nil, nil, fmt.Errorf("CollectRBTTokens: failed to lock tokens involved in transfer, err: %v", err)
	}

	return tokensTransfer, remainingBalanceDenomArr, nil
}

// This function returns the max possible parts index by the max decimal places, For 3 decimal places, it will be 1332
func MaxPossiblePartsIndexByMaxDecimalPlaces() int {
	totalPossibleLevels := 2 * constants.MaxSupportedDecimalPlaces //Actually 2*maxDecimalPlaces+1 will be the total possible levels but 1st level is about whole token so we are ignoring
	sum := 0
	nodesInLevel := make([]int, totalPossibleLevels+1)
	nodesInLevel[0] = 1 // level 0 has 1 node
	for i := 1; i <= totalPossibleLevels; i++ {
		if i%2 == 1 {
			nodesInLevel[i] = 2 * nodesInLevel[i-1]
		} else {
			nodesInLevel[i] = 5 * nodesInLevel[i-1]
		}
		sum += nodesInLevel[i]

	}
	return sum

}
