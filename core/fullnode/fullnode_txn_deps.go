package fullnode

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Each TokenInfo.PreviousTransactionID names the previous transaction for that
// token; these IDs link transactions into bundles. A split's ID is backfilled
// as the PreviousTransactionID of the minted tokens the transfer then spends
// (see BuildTransactionInfoFromRequest).

// forEachTokenInfo calls fn for every non-nil TokenInfo in info, in a fixed
// order (RBT, FT, NFT, SmartContract, CommittedTokens, quorum pledge tokens).
// Unlike wallet.collectFullNodeTokenInputs it walks CommittedTokens even when
// Tokens is nil, since missing a dependency is the costly mistake here.
// fn may see the same token more than once; callers dedupe.
func forEachTokenInfo(info *models.TransactionInfo, fn func(*models.TokenInfo)) {
	if info == nil {
		return
	}

	if info.Tokens != nil {
		for _, list := range [][]*models.TokenInfo{
			info.Tokens.RBT,
			info.Tokens.FT,
			info.Tokens.NFT,
			info.Tokens.SmartContract,
		} {
			for _, token := range list {
				if token != nil {
					fn(token)
				}
			}
		}
	}

	for _, token := range info.CommittedTokens {
		if token != nil {
			fn(token)
		}
	}

	for _, quorum := range info.Quorums {
		if quorum == nil {
			continue
		}
		for _, token := range quorum.Tokens {
			if token != nil {
				fn(token)
			}
		}
	}
}

// transactionDependencies returns the distinct non-empty PreviousTransactionIDs
// the transaction declares, in traversal order. An empty ID marks a genesis
// entry and is not a dependency; a pure genesis transaction yields nil.
func transactionDependencies(info *models.TransactionInfo) []string {
	var deps []string
	seen := make(map[string]struct{})

	forEachTokenInfo(info, func(token *models.TokenInfo) {
		prev := token.PreviousTransactionID
		if prev == "" {
			return
		}
		if _, dup := seen[prev]; dup {
			return
		}
		seen[prev] = struct{}{}
		deps = append(deps, prev)
	})

	return deps
}

// transactionTokenIDs returns the distinct non-empty TokenIDs the transaction
// touches, in traversal order, including tokens it mints.
func transactionTokenIDs(info *models.TransactionInfo) []string {
	var tokenIDs []string
	seen := make(map[string]struct{})

	forEachTokenInfo(info, func(token *models.TokenInfo) {
		id := token.TokenID
		if id == "" {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		tokenIDs = append(tokenIDs, id)
	})

	return tokenIDs
}
