package core

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// VerifyRecoveryCommitments is the fullnode side of the account-recovery
// commitment. It rebuilds, from the fullnode's chains, the commitment each
// quorum should have produced under the GenerateRecoveryCommitment rule and
// compares it with the submitted one.
func (c *Core) VerifyRecoveryCommitments(requestID, userDID string, submitted []types.RecoveryCommitment) (*types.RecoveryCommitmentReport, error) {
	if !c.fullNode {
		return nil, fmt.Errorf("VerifyRecoveryCommitments: only a fullnode can verify recovery commitments")
	}
	if requestID == "" || userDID == "" {
		return nil, fmt.Errorf("VerifyRecoveryCommitments: request ID and user DID are required")
	}
	chains, err := c.w.GetFullNodeRecoveryTipChains(userDID)
	if err != nil {
		return nil, fmt.Errorf("VerifyRecoveryCommitments: %w", err)
	}
	report, err := verifyQuorumCommitments(requestID, userDID, chains, submitted)
	if err != nil {
		return nil, fmt.Errorf("VerifyRecoveryCommitments: %w", err)
	}
	for _, chk := range report.Checks {
		if !chk.Match {
			c.log.Error("VerifyRecoveryCommitments: quorum commitment does not match",
				"requestID", requestID, "userDID", userDID, "quorumDID", chk.QuorumDID, "mismatches", chk.Mismatches)
		}
	}
	return report, nil
}

// verifyQuorumCommitments checks each submitted commitment against the one
// reproduced from chains. A quorum with no expected tokens must not submit a
// commitment (Q11). Checks are sorted by quorum DID.
func verifyQuorumCommitments(requestID, userDID string, chains map[string][]types.RecoveryChainEntry, submitted []types.RecoveryCommitment) (*types.RecoveryCommitmentReport, error) {
	expected, excluded := expectedPledgedTokens(userDID, chains)
	report := &types.RecoveryCommitmentReport{RequestID: requestID, UserDID: userDID, Excluded: excluded}

	seen := make(map[string]bool, len(submitted))
	for _, sub := range submitted {
		chk := types.RecoveryCommitmentCheck{QuorumDID: sub.CommitterDID, Submitted: sub}
		switch {
		case sub.CommitterDID == "":
			chk.Mismatches = []string{"committer_did is empty"}
		case sub.CommitterType != constants.RecoveryCommitterType_Quorum:
			chk.Mismatches = []string{"committer_type must be quorum"}
		case seen[sub.CommitterDID]:
			chk.Mismatches = []string{"duplicate commitment from this quorum"}
		case len(expected[sub.CommitterDID]) == 0:
			seen[sub.CommitterDID] = true
			chk.Mismatches = []string{"quorum holds no pledged tokens for the user and must not commit"}
		default:
			seen[sub.CommitterDID] = true
			tree, err := recoverycommit.BuildQuorumCommitment(requestID, userDID, sub.CommitterDID, expected[sub.CommitterDID])
			if err != nil {
				return nil, err
			}
			chk.Expected = tree
			chk.Mismatches = recoverycommit.CompareCommitments(tree.Commitment, sub)
			chk.Match = len(chk.Mismatches) == 0
		}
		report.Checks = append(report.Checks, chk)
	}
	sort.SliceStable(report.Checks, func(i, j int) bool { return report.Checks[i].QuorumDID < report.Checks[j].QuorumDID })

	for q := range expected {
		if !seen[q] {
			report.MissingQuorums = append(report.MissingQuorums, q)
		}
	}
	sort.Strings(report.MissingQuorums)
	return report, nil
}

// expectedPledgedTokens maps each quorum to the tokens it should commit for
// userDID. chains holds the tokens whose latest transaction X names userDID as
// owner. Only RBTs that X moved are considered. A token goes to every quorum that pledged X, if X is still the tip of
// all tokens X moved (otherwise the quorums have unpledged X) and its chain is
// valid. Excluded is sorted by token ID.
func expectedPledgedTokens(userDID string, chains map[string][]types.RecoveryChainEntry) (map[string][]types.RecoveryTokenCommitment, []types.RecoveryExcludedToken) {
	expected := make(map[string][]types.RecoveryTokenCommitment)
	var excluded []types.RecoveryExcludedToken
	exclude := func(tokenID, reason string) {
		excluded = append(excluded, types.RecoveryExcludedToken{TokenID: tokenID, Reason: reason})
	}

	for tokenID, chain := range chains {
		tip := chain[len(chain)-1]
		var info models.TransactionInfo
		if err := json.Unmarshal(tip.Info, &info); err != nil {
			exclude(tokenID, "decode latest transaction "+tip.TransactionID+": "+err.Error())
			continue
		}
		if info.Tokens == nil {
			continue
		}
		if !recoverycommit.ContainsToken(info.Tokens.RBT, tokenID) {
			continue // not an RBT the user received, or a pledge or committed token
		}
		// X is unpledged once any token it moved, of any type, moves on.
		moved := [][]*models.TokenInfo{info.Tokens.RBT, info.Tokens.FT, info.Tokens.NFT, info.Tokens.SmartContract}
		live := true
		for _, list := range moved {
			for _, t := range list {
				if t == nil {
					continue
				}
				if c := chains[t.TokenID]; len(c) == 0 || c[len(c)-1].TransactionID != tip.TransactionID {
					live = false
				}
			}
		}
		var pledgers []string
		for _, q := range info.Quorums {
			if q != nil && len(q.Tokens) > 0 && q.Did != userDID {
				pledgers = append(pledgers, q.Did)
			}
		}
		if !live || len(pledgers) == 0 {
			exclude(tokenID, "latest transaction "+tip.TransactionID+" is not pledged any more")
			continue
		}
		if err := recoverycommit.ValidateChain(chain); err != nil {
			exclude(tokenID, "incomplete tokenchain: "+err.Error())
			continue
		}
		tc, err := recoverycommit.BuildTokenCommitment(tokenID, recoverycommit.TransactionIDs(chain))
		if err != nil {
			exclude(tokenID, err.Error())
			continue
		}
		for _, q := range pledgers {
			expected[q] = append(expected[q], tc)
		}
	}
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].TokenID < excluded[j].TokenID })
	return expected, excluded
}

// ComputeUnpledgedCommitment builds this fullnode's root for free user tokens.
// Excluded tokens have missing or broken history and are not in the root.
func (c *Core) ComputeUnpledgedCommitment(requestID, userDID, fullnodeDID string) (*types.RecoveryCommitmentTree, []types.RecoveryExcludedToken, error) {
	if !c.fullNode {
		return nil, nil, fmt.Errorf("ComputeUnpledgedCommitment: only a fullnode can build this commitment")
	}
	if requestID == "" || userDID == "" || fullnodeDID == "" {
		return nil, nil, fmt.Errorf("ComputeUnpledgedCommitment: request ID, user DID and fullnode DID are required")
	}
	chains, err := c.w.GetFullNodeRecoveryChains(userDID)
	if err != nil {
		return nil, nil, fmt.Errorf("ComputeUnpledgedCommitment: %w", err)
	}
	tokens, excluded := selectUnpledgedTokens(userDID, chains)
	tree, err := recoverycommit.BuildUnpledgedCommitment(requestID, userDID, fullnodeDID, tokens)
	return tree, excluded, err
}

func selectUnpledgedTokens(userDID string, chains map[string][]types.RecoveryChainEntry) ([]types.RecoveryTokenCommitment, []types.RecoveryExcludedToken) {
	var tokens []types.RecoveryTokenCommitment
	var excluded []types.RecoveryExcludedToken
	for tokenID, chain := range chains {
		if !recoverycommit.IsRBTChain(tokenID, chain) {
			continue
		}
		var err error
		if err = recoverycommit.ValidateChain(chain); err == nil {
			var owner string
			owner, err = recoverycommit.CurrentOwner(tokenID, chain)
			if err == nil && owner != userDID {
				continue
			}
		}
		if err == nil {
			var pledgedBy []string
			pledgedBy, err = recoverycommit.IsLatestTxnPledged(tokenID, chain, chains)
			if err == nil && len(pledgedBy) != 0 {
				continue
			}
		}
		if err == nil {
			var token types.RecoveryTokenCommitment
			token, err = recoverycommit.BuildTokenCommitment(tokenID, recoverycommit.TransactionIDs(chain))
			if err == nil {
				tokens = append(tokens, token)
			}
		}
		if err != nil {
			excluded = append(excluded, types.RecoveryExcludedToken{TokenID: tokenID, Reason: err.Error()})
		}
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].TokenID < tokens[j].TokenID })
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].TokenID < excluded[j].TokenID })
	return tokens, excluded
}
