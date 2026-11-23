package core

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types"
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
// the root. The tree is nil when no token is left: a quorum holding nothing
// for the user does not publish a commitment (Q11).
func (c *Core) GenerateRecoveryCommitment(requestID, userDID, quorumDID string) (*types.RecoveryCommitmentTree, []types.RecoveryExcludedToken, error) {
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
	for _, x := range excluded {
		c.log.Info("GenerateRecoveryCommitment: token left out of commitment",
			"requestID", requestID, "userDID", userDID, "tokenID", x.TokenID, "reason", x.Reason)
	}
	if len(tokens) == 0 {
		c.log.Info("GenerateRecoveryCommitment: quorum holds no pledged tokens for the user, no commitment",
			"requestID", requestID, "userDID", userDID, "quorumDID", quorumDID)
		return nil, excluded, nil
	}

	tree, err := recoverycommit.BuildQuorumCommitment(requestID, userDID, quorumDID, tokens)
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateRecoveryCommitment: %w", err)
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
func selectQuorumPledgedTokens(pledged map[string][]string, chains map[string][]types.RecoveryChainEntry) ([]types.RecoveryTokenCommitment, []types.RecoveryExcludedToken) {
	tokens := make([]types.RecoveryTokenCommitment, 0, len(pledged))
	var excluded []types.RecoveryExcludedToken

	for tokenID, txIDs := range pledged {
		chain := chains[tokenID]
		if len(chain) == 0 {
			excluded = append(excluded, types.RecoveryExcludedToken{TokenID: tokenID, Reason: "no local tokenchain"})
			continue
		}
		if !recoverycommit.IsRBTChain(tokenID, chain) {
			continue
		}
		if tip := chain[len(chain)-1].TransactionID; !slices.Contains(txIDs, tip) {
			excluded = append(excluded, types.RecoveryExcludedToken{TokenID: tokenID, Reason: "latest transaction " + tip + " is not a live pledge of this quorum"})
			continue
		}
		if err := recoverycommit.ValidateChain(chain); err != nil {
			excluded = append(excluded, types.RecoveryExcludedToken{TokenID: tokenID, Reason: "incomplete tokenchain: " + err.Error()})
			continue
		}
		tc, err := recoverycommit.BuildTokenCommitment(tokenID, recoverycommit.TransactionIDs(chain))
		if err != nil {
			excluded = append(excluded, types.RecoveryExcludedToken{TokenID: tokenID, Reason: err.Error()})
			continue
		}
		tokens = append(tokens, tc)
	}
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].TokenID < excluded[j].TokenID })
	return tokens, excluded
}

// publishQuorumCommitments runs when a quorum's session opens its commit
// window, that is once the user has deployed the ephemeral NFT. Executing it
// earlier would deploy it under the quorum's DID and block the recovery.
// Every quorum DID on this node that holds pledged tokens for the user
// publishes its commitment. They publish one after another, because each
// execution locks the ephemeral NFT on this node.
func (c *Core) publishQuorumCommitments(s *types.RecoverySession) {
	for quorumDID := range c.qc {
		if quorumDID == s.UserDID {
			continue
		}
		if err := c.publishQuorumCommitment(s, quorumDID); err != nil {
			c.log.Error("Failed to publish recovery commitment", "request_txn_id", s.RequestID, "quorum_did", quorumDID, "err", err)
		}
	}
}

// publishRecoveryMessage executes the ephemeral NFT of s with msg, signed by
// dc. Every participant publishes when the commit window opens, so their
// executions build on the same chain tip and all but the first are rejected
// with a chain mismatch. A rejected execution is retried after a random
// pause, by which time this node has synced the winning one, until the
// commit window closes.
func (c *Core) publishRecoveryMessage(s *types.RecoverySession, dc types.DIDCrypto, msg *types.RecoveryMessage) (string, error) {
	closes := time.Unix(s.RequestEpoch+constants.RecoveryWindow_CommitSeconds, 0)
	for {
		txID, err := c.executeEphemeralNFT(uuid.New().String(), dc, s.RequestID, msg)
		if err == nil || !strings.Contains(err.Error(), "chain mismatch") {
			return txID, err
		}
		pause := time.Second + time.Duration(rand.Int63n(int64(4*time.Second)))
		if time.Now().Add(pause).After(closes) {
			return "", fmt.Errorf("commit window closes before a retry: %w", err)
		}
		c.log.Info("Recovery message lost a race on the ephemeral NFT, retrying", "request_txn_id", s.RequestID,
			"sender_did", msg.SenderDID, "type", msg.Type, "retry_in", pause.Round(time.Millisecond))
		time.Sleep(pause)
	}
}

// publishQuorumCommitment builds quorumDID's commitment, executes the
// ephemeral NFT with it and stores the tree. It publishes nothing when the
// quorum holds nothing for the user (Q11).
func (c *Core) publishQuorumCommitment(s *types.RecoverySession, quorumDID string) error {
	tree, _, err := c.GenerateRecoveryCommitment(s.RequestID, s.UserDID, quorumDID)
	if err != nil {
		return err
	}
	if tree == nil {
		return nil
	}
	body, err := json.Marshal(tree.Commitment)
	if err != nil {
		return fmt.Errorf("failed to encode commitment: %w", err)
	}
	msg := &types.RecoveryMessage{
		Type:      constants.RecoveryMessageType_Commitment,
		Body:      body,
		SenderDID: quorumDID,
	}
	txID, err := c.publishRecoveryMessage(s, c.pqc[quorumDID], msg)
	if err != nil {
		return err
	}
	if err := c.w.SaveRecoveryQuorumCommitment(quorumDID, tree); err != nil {
		return err
	}
	c.log.Info("Recovery commitment published", "request_txn_id", s.RequestID, "quorum_did", quorumDID,
		"txn_id", txID, "token_count", tree.Commitment.TokenCount, "root", tree.Commitment.Root)
	return nil
}
