package parts

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/core/coin"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func floatPrecision(num float64, precision int) float64 {
	precision = coin.MaxSupportedDecimalPlaces
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func CollectTokens(dc did.DIDCrypto, w *wallet.Wallet, did string, targetAmount float64, ipfsOps IPFSOperation, isTestnet bool, log logger.Logger) ([]wallet.Token, error) {
	tokens, err := w.GetFreeTokens(did)
	if err != nil {
		return nil, fmt.Errorf("CollectTokens: failed to get free tokens for did: %v, err: %v", did, err)
	}

	// Build the tree
	tokenDenomTree, err := BuildTokenTree(tokens, did, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("CollectTokens: failed to get the denom tree for did: %v, err: %v", did, err)
	}

	plan := &TransferPlan{
		TokensToTransfer: []wallet.Token{},
		TokensToSplit:    []SplitOp{},
	}

	remaining := targetAmount
	zeroFloat := floatPrecision(0.0, coin.MaxSupportedDecimalPlaces)

	for idx, token := range tokenDenomTree.Leaves {
		log.Debug(fmt.Sprintf("CollectTokens: iteraction : %v", idx))
		if remaining <= zeroFloat {
			break
		}

		heirarchicalID := token.HierarchicalID
		tokenValue := token.Token.TokenValue

		log.Debug(fmt.Sprintf("token value: %v", tokenValue))
	
		if tokenValue <= remaining {
			log.Debug("token <= remaining")

			plan.TokensToTransfer = append(plan.TokensToTransfer, token.Token)
			plan.TotalValue += tokenValue
			log.Debug(fmt.Sprintf("remaining value (pre): %v", remaining))
			remaining = floatPrecision(remaining-tokenValue, coin.MaxSupportedDecimalPlaces)
			log.Debug(fmt.Sprintf("remaining value (post): %v", remaining))
		} else if tokenValue > remaining {
			log.Debug(fmt.Sprintf("token value > remaining"))

			splitOp, err := planSplit(ipfsOps, heirarchicalID, remaining)
			if err != nil {
				return nil, fmt.Errorf("CollectTokens: failed to plan the token split, err: %v", err)
			}

			plan.TokensToSplit = append(plan.TokensToSplit, splitOp...)
			plan.TotalValue = plan.TotalValue + remaining
			remaining = zeroFloat
		}

		log.Debug(fmt.Sprintf("remaining value: %v", remaining))
	}

	if remaining > zeroFloat {
		return nil, fmt.Errorf("CollectTokens: could not satisfy transfer amount, remaining: %v", remaining)
	}

	tokenCache := make(map[string]*wallet.Token)

	for _, splitOp := range plan.TokensToSplit {
		someSplittedTokensToTransfer, err := splitAtLevel(w, dc, ipfsOps, splitOp, tokenCache, isTestnet)
		if err != nil {
			return nil, nil
		}

		plan.TokensToTransfer = append(plan.TokensToTransfer, someSplittedTokensToTransfer...)
	}

	return plan.TokensToTransfer, nil
}
