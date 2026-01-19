package parts

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/coin"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func CollectRBTTokens(dc did.DIDCrypto, w *wallet.Wallet, did string, targetAmount float64, ipfsOps IPFSOperation, isTestnet bool, log logger.Logger) ([]wallet.Token, error) {
	var splitOps []SplitOp = make([]SplitOp, 0)
	var tokensTransfer []wallet.Token = make([]wallet.Token, 0)

	tokens, err := w.GetFreeTokens(did)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: failed to get free tokens for did: %v, err: %v", did, err)
	}

	// Build the tree
	tokenDenomTree, err := BuildDenomTree(tokens, did, ipfsOps)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: failed to get the denom tree for did: %v, err: %v", did, err)
	}

	remaining := targetAmount
	zeroFloat := floatPrecision(0.0, coin.MaxSupportedDecimalPlaces)

	for _, token := range tokenDenomTree.Leaves {
		if remaining <= zeroFloat {
			break
		}

		heirarchicalID := token.HierarchicalID
		tokenValue := token.Token.TokenValue

		if tokenValue <= remaining {
			tokensTransfer = append(tokensTransfer, token.Token)
			remaining = floatPrecision(remaining-tokenValue, coin.MaxSupportedDecimalPlaces)
		} else if tokenValue > remaining {
			splitOp, err := planSplit(heirarchicalID, remaining, log)
			if err != nil {
				return nil, fmt.Errorf("CollectRBTTokens: failed to plan the token split, err: %v", err)
			}

			splitOps = append(splitOps, splitOp...)
			remaining = zeroFloat
		}
	}

	if remaining > zeroFloat {
		return nil, fmt.Errorf("CollectRBTTokens: could not satisfy transfer amount, remaining: %v", remaining)
	}

	tokenCache := make(map[string]*wallet.Token)

	for _, splitOp := range splitOps {
		partTokensToTransfer, err := performTokenSplitAtLevel(w, dc, ipfsOps, splitOp, tokenCache, isTestnet)
		if err != nil {
			return nil, nil
		}

		tokensTransfer = append(tokensTransfer, partTokensToTransfer...)
	}

	err = w.LockTokens(tokensTransfer)
	if err != nil {
		return nil, fmt.Errorf("CollectRBTTokens: could not lock all transferrable tokens, err: %v", err)
	}

	return tokensTransfer, nil
}
