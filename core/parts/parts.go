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
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func checkSufficientBalance(w *wallet.Wallet, did string, transferAmount float64) error {
	userOwnedTokens, err := w.GetFreeTokens(did)
	if err != nil {
		return fmt.Errorf("checkSufficientBalance: unable to fetch balance for %v, err: %v", did, err)
	}

	var accountBalance float64 = rubixmath.ZeroFloat()
	for _, token := range userOwnedTokens {
		accountBalance += token.TokenValue
	}

	if accountBalance < transferAmount {
		return fmt.Errorf(
			"checkSufficientBalance: insufficient balance as the current balance is: %v, transfer amount: %v",
			accountBalance,
			transferAmount,
		)
	}

	return nil
}

func CollectRBTTokens(dc did.DIDCrypto, w *wallet.Wallet, transferAmount float64,
	 isTestnet bool, log logger.Logger, publishFn func(*model.PubSubTxnInfo) error,
) ([]wallet.Token, error) {
	var splitOps []SplitOp = make([]SplitOp, 0)
	var tokensTransfer []wallet.Token = make([]wallet.Token, 0)
	var did string = dc.GetDID()

	// Check if transfer amount doesn't exceed maximum supported decimal places
	decimalPlaces := strconv.FormatFloat(transferAmount, 'f', -1, 64)
	decimalPlacesStr := strings.Split(decimalPlaces, ".")
	if len(decimalPlacesStr) == 2 && len(decimalPlacesStr[1]) > constants.MaxSupportedDecimalPlaces {
		return nil, fmt.Errorf("transaction amount exceeds %v decimal places", constants.MaxSupportedDecimalPlaces)
	}

	// Check Balance
	err := checkSufficientBalance(w, did, transferAmount)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: failed while checking balance, err: %v", err)
	}

	// Attempt to collect tokens which wouldn't require any splitting
	// by looking at user's balance denom array
	var nonSplitTokenTransfer []wallet.Token = make([]wallet.Token, 0)

	balanceDenomArr, leadTokenList, err := w.GetTokenDenomArrForDID(did)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: failed to fetch token denom arr for did: %v, err: %v", did, err)
	}

	nonSplitDenomArr, remainingBalanceDenomArr, remainingAmount, err := wallet.GetTokenDenomArrayWithoutSplit(balanceDenomArr, transferAmount)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: error occured while looking to fetch non-split token denom array for transfer, err: %v", err)
	}

	if !wallet.CheckEmptyTokenDenomArr(nonSplitDenomArr) {
		nonSplitTokenTransfer, err = w.GetTokensFromDenomArr(nonSplitDenomArr, did)
		if err != nil {
			return nil, fmt.Errorf("CollectRBTTokens: error occured while fetching non-split tokens for transfer, err: %v", err)
		}

		tokensTransfer = append(tokensTransfer, nonSplitTokenTransfer...)
	}

	if remainingAmount > rubixmath.ZeroFloat() {
		// For the remaining amount, proceed to build the denom tree
		// and split accordingly
		var remainingAvailableTokens []wallet.Token = removeTokensFromList(leadTokenList, nonSplitTokenTransfer)

		// Build the tree
		tokenDenomTree, err := BuildDenomTree(remainingAvailableTokens, did)
		if err != nil {
			return nil, fmt.Errorf("CollectRBTTokens: failed to get the denom tree for did: %v, err: %v", did, err)
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
					return nil, fmt.Errorf("CollectRBTTokens: failed to plan the token split, err: %v", err)
				}

				splitOps = append(splitOps, splitOp...)
				remainingAmount = rubixmath.ZeroFloat()
			}
		}

		if remainingAmount > rubixmath.ZeroFloat() {
			return nil, fmt.Errorf("CollectRBTTokens: could not satisfy transfer amount, remaining: %v", remainingAmount)
		}

		tokenCache := make(map[string]*wallet.Token)

		for _, splitOp := range splitOps {
			partTokensToTransfer, err := performTokenSplit(w, dc, splitOp, tokenCache, isTestnet, remainingBalanceDenomArr, publishFn)
			if err != nil {
				return nil, fmt.Errorf("CollectRBTTokens: could not perform split at Level: %v, err: %v", splitOp.HierarchicalTokenID.Level(), err)
			}

			tokensTransfer = append(tokensTransfer, partTokensToTransfer...)
		}
	}

	err = w.LockTokens(tokensTransfer)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: could not lock all transferrable tokens, err: %v", err)
	}

	return tokensTransfer, nil
}
