package core

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Dependency extraction for the fullnode transaction pipeline.
//
// A transaction declares, per token it touches, the transaction that last moved
// that token — TokenInfo.PreviousTransactionID. Those IDs are the edges the
// fullnode uses to relate transactions to one another: a split's ID appears
// verbatim as the PreviousTransactionID of the transfer that spends its output
// (core/transaction_builder.go:159 and core/consensus/consensus.go:70-75 both
// perform that backfill).
//
// consensus.TokenChainIntegrityCheck and wallet.collectFullNodeTokenInputs each
// walk the same token set for their own purposes. This file is the single
// definition of that walk for the fullnode pipeline, so the dependency check,
// the sync gate and the persistence layer cannot drift apart.

// forEachTokenInfo invokes fn for every non-nil TokenInfo referenced by info.
//
// Traversal order mirrors wallet.collectFullNodeTokenInputs
// (core/wallet/fullnode_persistence.go:200-225): RBT, FT, NFT, SmartContract,
// CommittedTokens, then each quorum's pledge tokens in quorum order. Order is
// fixed rather than map-driven — consensus.TokenChainIntegrityCheck ranges over
// a map[string][]*models.TokenInfo (core/consensus/checks.go:709-716), so its
// order varies between runs and cannot be relied on.
//
// Two deliberate differences from collectFullNodeTokenInputs:
//
//   - CommittedTokens are walked unconditionally. In the persistence path they
//     are nested inside the `if txInfo.Tokens != nil` guard, so a transaction
//     carrying committed tokens but no transfer tokens skips them entirely.
//     Under-reporting a dependency is the failure mode that matters here, so
//     this walk does not reproduce that.
//   - Quorum pledge tokens are always included. TokenChainIntegrityCheck gates
//     them on isFullnode; every caller of this file is the fullnode, where that
//     flag is true, and a quorum's split is a genuine dependency edge.
//
// fn may be called more than once for the same token; callers dedupe.
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

// transactionDependencies returns every distinct non-empty PreviousTransactionID
// the transaction declares, in traversal order.
//
// An empty PreviousTransactionID marks a genesis entry for that token — the
// token has no prior transaction — and is not a dependency, so it is omitted.
// A transaction with no dependencies at all (a pure genesis, such as the split
// leg of a transfer) yields nil.
//
// The result is deterministic for a given input: same order, same contents.
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

// transactionTokenIDs returns every distinct non-empty TokenID the transaction
// touches, in traversal order.
//
// Unlike transactionDependencies this includes genesis entries: a token minted
// by this transaction is still a token the transaction affects. Returns nil when
// the transaction references no tokens.
//
// The result is deterministic for a given input: same order, same contents.
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
