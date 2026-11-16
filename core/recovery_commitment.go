package core

import (
	"fmt"
	"slices"
	"sort"

	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
)

// GenerateRecoveryCommitment is the quorum side of the account-recovery
// commitment. It commits to every token quorumDID is still pledged for on
// behalf of userDID (a live unpledge_sequence_info row whose transaction
// names userDID as owner) and whose local tokenchain ends at that pledged
// transaction, each token hashed over its whole local tokenchain.
//
// The returned tree keeps the per-token chain roots so that token proofs can
// be produced later. The excluded list names pledged tokens whose local chain
// is missing, incomplete or has moved past the pledge; they are not part of
// the root.
func (c *Core) GenerateRecoveryCommitment(requestID, userDID, quorumDID string) (*recoverycommit.CommitmentTree, []recoverycommit.ExcludedToken, error) {
	if requestID == "" || userDID == "" || quorumDID == "" {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: request ID, user DID and quorum DID are required")
	}
	if userDID == quorumDID {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: quorum %s cannot commit for its own DID", quorumDID)
	}
	if _, ok := c.qc[quorumDID]; !ok {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: quorum %s is not set up on this node", quorumDID)
	}

	pledged, err := c.w.GetRecoveryPledgedTokens(quorumDID, userDID)
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: %w", err)
	}
	tokenIDs := make([]string, 0, len(pledged))
	for id := range pledged {
		tokenIDs = append(tokenIDs, id)
	}
	chains, err := c.w.GetRecoveryTokenChains(tokenIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: %w", err)
	}
	tokens, excluded := selectQuorumPledgedTokens(pledged, chains)

	tree, err := recoverycommit.BuildQuorumCommitment(requestID, userDID, quorumDID, tokens)
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: %w", err)
	}
	for _, x := range excluded {
		c.log.Info("GenerateRecoveryCommitment: token left out of commitment",
			"requestID", requestID, "userDID", userDID, "tokenID", x.TokenID, "reason", x.Reason)
	}
	c.log.Info("GenerateRecoveryCommitment: commitment built",
		"requestID", requestID, "userDID", userDID, "quorumDID", quorumDID,
		"tokenCount", tree.Commitment.TokenCount, "root", tree.Commitment.Root)
	return tree, excluded, nil
}

// selectQuorumPledgedTokens computes the chain root of every pledged token.
// pledged maps each token to the transaction IDs of its live pledges; chains
// holds the local tokenchains.
//
// A token is committed only if the latest transaction of its local chain is
// one of its live pledged transactions; the root then covers the whole local
// chain. A token is excluded if it has no local chain, its latest transaction
// is not a live pledge (the chain moved on, so the row is stale, see
// recovery-architecture §5.7), or its chain fails
// recoverycommit.ValidateChain. Excluded is sorted by token ID.
func selectQuorumPledgedTokens(pledged map[string][]string, chains map[string][]recoverycommit.ChainEntry) ([]recoverycommit.TokenCommitment, []recoverycommit.ExcludedToken) {
	tokens := make([]recoverycommit.TokenCommitment, 0, len(pledged))
	var excluded []recoverycommit.ExcludedToken

	for tokenID, txIDs := range pledged {
		chain := chains[tokenID]
		if len(chain) == 0 {
			excluded = append(excluded, recoverycommit.ExcludedToken{TokenID: tokenID, Reason: "no local tokenchain"})
			continue
		}
		if tip := chain[len(chain)-1].TransactionID; !slices.Contains(txIDs, tip) {
			excluded = append(excluded, recoverycommit.ExcludedToken{TokenID: tokenID, Reason: "latest transaction " + tip + " is not a live pledge of this quorum"})
			continue
		}
		if err := recoverycommit.ValidateChain(chain); err != nil {
			excluded = append(excluded, recoverycommit.ExcludedToken{TokenID: tokenID, Reason: "incomplete tokenchain: " + err.Error()})
			continue
		}
		tc, err := recoverycommit.BuildTokenCommitment(tokenID, recoverycommit.TransactionIDs(chain))
		if err != nil {
			excluded = append(excluded, recoverycommit.ExcludedToken{TokenID: tokenID, Reason: err.Error()})
			continue
		}
		tokens = append(tokens, tc)
	}
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].TokenID < excluded[j].TokenID })
	return tokens, excluded
}
