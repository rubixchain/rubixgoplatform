package parts

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
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

func CollectRBTTokens(dc did.DIDCrypto, w *wallet.Wallet, transferAmount float64,
	isTestnet bool, log logger.Logger, publishFn func(*model.PubSubTxnInfo) error,
) (*TokenSplitInfo, *TokenSplitInfo, map[types.DenomValue]types.DenomCount, error) {
	var splitOps []SplitOp = make([]SplitOp, 0)
	var tokensTransfer []models.Token = make([]models.Token, 0)
	var burntTokensTransfer []models.Token = make([]models.Token, 0)
	var leftoverTokens []models.Token = make([]models.Token, 0)
	var burntTokensKeep []models.Token = make([]models.Token, 0)

	var tokenTransferInfo *TokenSplitInfo = &TokenSplitInfo{}
	var tokenKeepInfo *TokenSplitInfo = &TokenSplitInfo{}

	var did string = dc.GetDID()

	// Check if transfer amount doesn't exceed maximum supported decimal places
	decimalPlaces := strconv.FormatFloat(transferAmount, 'f', -1, 64)
	decimalPlacesStr := strings.Split(decimalPlaces, ".")
	if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > constants.MaxSupportedDecimalPlaces {
		return nil, nil, nil, fmt.Errorf("transaction amount exceeds %v decimal places", constants.MaxSupportedDecimalPlaces)
	}

	// Check Balance
	err := checkSufficientBalance(w, did, transferAmount)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed while checking balance, err: %v", err)
	}

	ownedRBTTokens, _, err := w.GetFreeRBTTokens(did)
	if err != nil {
		return nil, nil, nil, err
	}

	// Attempt to collect tokens which wouldn't require any splitting
	// by looking at user's balance denom array
	var nonSplitTokenTransfer []models.Token = make([]models.Token, 0)

	denomMap, err := w.GetTokenDenomArray(did)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed while fetching token denom array, err: %v", err)
	}

	nonSplitDenomArr, remainingBalanceDenomArr, remainingAmount, err := GetSplitAndNonsplitTokenDenom(denomMap, transferAmount)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CollectRBTTokens: error occured while looking to fetch non-split token denom array for transfer, err: %v", err)
	}

	if len(nonSplitDenomArr) != 0 {
		nonSplitTokenTransfer, err = w.GetTokensFromDenomMap(nonSplitDenomArr, did)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("CollectRBTTokens: error occured while fetching non-split tokens for transfer, err: %v", err)
		}

		tokensTransfer = append(tokensTransfer, nonSplitTokenTransfer...)
	}

	if remainingAmount > rubixmath.ZeroFloat() {
		// For the remaining amount, proceed to build the denom tree
		// and split accordingly
		var remainingAvailableTokens []models.Token = removeTokensFromList(ownedRBTTokens, nonSplitTokenTransfer)

		// Build the tree
		tokenDenomTree, err := BuildDenomTree(remainingAvailableTokens, did)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to get the denom tree for did: %v, err: %v", did, err)
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
					return nil, nil, nil, fmt.Errorf("CollectRBTTokens: failed to plan the token split, err: %v", err)
				}

				splitOps = append(splitOps, splitOp...)
				remainingAmount = rubixmath.ZeroFloat()
			}
		}

		if remainingAmount > rubixmath.ZeroFloat() {
			return nil, nil, nil, fmt.Errorf("CollectRBTTokens: could not satisfy transfer amount, remaining: %v", remainingAmount)
		}

		tokenCache := make(map[string]models.Token)

		for _, splitOp := range splitOps {
			partTokensToTransfer, tokensBurntForTransferTokens, partTokensToKeep, tokensBurntForKeptTokens, err := performTokenSplit(w, dc, splitOp, tokenCache, remainingBalanceDenomArr)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("CollectRBTTokens: could not perform split at Level: %v, err: %v", splitOp.HierarchicalTokenID.Level(), err)
			}

			tokensTransfer = append(tokensTransfer, partTokensToTransfer...)
			burntTokensTransfer = append(burntTokensTransfer, tokensBurntForTransferTokens...)

			leftoverTokens = append(leftoverTokens, partTokensToKeep...)
			burntTokensKeep = append(burntTokensKeep, tokensBurntForKeptTokens...)
		}
	}

	tokenTransferInfo = &TokenSplitInfo{
		TransferTokens: tokensTransfer,
		BurntTokens:    burntTokensTransfer,
	}

	tokenKeepInfo = &TokenSplitInfo{
		TransferTokens: leftoverTokens,
		BurntTokens:    burntTokensKeep,
	}

	err = w.LockTokens(tokensTransfer)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CollectRBTTokens: could not lock all transferrable tokens, err: %v", err)
	}

	return tokenTransferInfo, tokenKeepInfo, remainingBalanceDenomArr, nil
}
