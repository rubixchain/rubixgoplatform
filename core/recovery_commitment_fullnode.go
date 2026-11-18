package core

import (
	"encoding/json"
	"fmt"
	"sort"

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
// reproduced from chains. A quorum with no expected tokens is expected to
// submit an empty commitment. Checks are sorted by quorum DID.
func verifyQuorumCommitments(requestID, userDID string, chains map[string][]types.RecoveryChainEntry, submitted []types.RecoveryCommitment) (*types.RecoveryCommitmentReport, error) {
	expected, excluded := expectedPledgedTokens(userDID, chains)
	report := &types.RecoveryCommitmentReport{RequestID: requestID, UserDID: userDID, Excluded: excluded}

	seen := make(map[string]bool, len(submitted))
	for _, sub := range submitted {
		chk := types.RecoveryCommitmentCheck{QuorumDID: sub.QuorumDID, Submitted: sub}
		switch {
		case sub.QuorumDID == "":
			chk.Mismatches = []string{"quorum_did is empty"}
		case seen[sub.QuorumDID]:
			chk.Mismatches = []string{"duplicate commitment from this quorum"}
		default:
			seen[sub.QuorumDID] = true
			tree, err := recoverycommit.BuildQuorumCommitment(requestID, userDID, sub.QuorumDID, expected[sub.QuorumDID])
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
// owner. A token goes to every quorum that pledged X, if X is still the tip of
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
		moved := [][]*models.TokenInfo{info.Tokens.RBT, info.Tokens.FT, info.Tokens.NFT, info.Tokens.SmartContract}
		isMoved := false
		live := true
		for _, list := range moved {
			isMoved = isMoved || recoverycommit.ContainsToken(list, tokenID)
			for _, t := range list {
				if t == nil {
					continue
				}
				if c := chains[t.TokenID]; len(c) == 0 || c[len(c)-1].TransactionID != tip.TransactionID {
					live = false
				}
			}
		}
		if !isMoved {
			continue // a pledge or committed token, not the user's
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
