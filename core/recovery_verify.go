package core

import (
	"fmt"
	"sort"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/types"
)

// verifyRecovery runs on the user's node when a session's sync window closes.
// It checks every published quorum and unpledged commitment against the
// chains its committer delivered and stores the result [R12].
func (c *Core) verifyRecovery(requestID string) {
	session, err := c.w.GetRecoverySession(requestID)
	if err != nil || session == nil || session.Role != constants.RecoveryRole_User {
		if err != nil {
			c.log.Error("Recovery verification: failed to read session", "request_txn_id", requestID, "err", err)
		}
		return
	}
	v, err := c.recoveryVerification(session)
	if err != nil {
		c.log.Error("Recovery verification failed", "request_txn_id", requestID, "err", err)
		return
	}
	matched := 0
	for _, chk := range v.Checks {
		if chk.Match {
			matched++
		}
	}
	c.log.Info("Recovery deliveries verified", "request_txn_id", requestID, "committers", len(v.Checks), "matched", matched)
}

// recoveryVerification returns the stored verification of session, or runs
// and stores it if there is none yet.
func (c *Core) recoveryVerification(session *types.RecoverySession) (*types.RecoveryVerification, error) {
	v, err := c.w.GetRecoveryVerification(session.RequestID)
	if err != nil || v != nil {
		return v, err
	}
	chain, err := c.recoveryChain(session.EphemeralNFTID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the ephemeral NFT chain: %w", err)
	}
	deliveries, err := c.w.GetRecoveryDeliveryChains(session.RequestID)
	if err != nil {
		return nil, err
	}
	v = &types.RecoveryVerification{
		RequestID: session.RequestID,
		UserDID:   session.UserDID,
		Checks: verifyRecoveryDeliveries(session.RequestID, session.UserDID,
			collectRecoveryCommitments(chain, session.UserDID),
			collectUnpledgedCommitments(chain, session.UserDID), deliveries),
	}
	if err := c.w.SaveRecoveryVerification(v); err != nil {
		return nil, err
	}
	return v, nil
}

// verifyRecoveryDeliveries rebuilds each published commitment, quorum
// commitments as quorum ones and unpledged commitments as fullnode ones, from
// the chains its committer delivered and compares the two. A committer that
// published tokens but delivered nothing fails; a commitment with no tokens
// needs no delivery. The rebuilt commitment takes the request ID and user DID
// from the session and its committer type from the message type, so a
// commitment naming another request or the wrong committer type fails.
// Checks are sorted by committer DID.
func verifyRecoveryDeliveries(requestID, userDID string, quorum, unpledged []types.RecoveryCommitment, deliveries map[string]map[string][]types.TransactionWithRole) []types.RecoveryDeliveryCheck {
	checks := make([]types.RecoveryDeliveryCheck, 0, len(quorum)+len(unpledged))
	for i, pub := range append(append([]types.RecoveryCommitment(nil), quorum...), unpledged...) {
		build := recoverycommit.BuildQuorumCommitment
		if i >= len(quorum) {
			build = recoverycommit.BuildUnpledgedCommitment
		}
		chk := types.RecoveryDeliveryCheck{CommitterDID: pub.CommitterDID, Published: pub}
		chains, ok := deliveries[pub.CommitterDID]
		if !ok && pub.TokenCount != 0 {
			chk.Mismatches = []string{"no chains delivered"}
			checks = append(checks, chk)
			continue
		}
		recomputed, err := recomputeRecoveryCommitment(build, requestID, userDID, pub.CommitterDID, chains)
		if err != nil {
			chk.Mismatches = []string{err.Error()}
			checks = append(checks, chk)
			continue
		}
		chk.Recomputed = recomputed
		chk.Mismatches = recoverycommit.CompareCommitments(*recomputed, pub)
		chk.Match = len(chk.Mismatches) == 0
		checks = append(checks, chk)
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].CommitterDID < checks[j].CommitterDID })
	return checks
}

// recomputeRecoveryCommitment hashes the delivered chains the way the
// committer hashed its own (every transaction of each token's chain, in chain
// order) and builds the commitment with build, which is
// recoverycommit.BuildQuorumCommitment or BuildUnpledgedCommitment.
func recomputeRecoveryCommitment(build func(requestID, userDID, committerDID string, tokens []types.RecoveryTokenCommitment) (*types.RecoveryCommitmentTree, error), requestID, userDID, committerDID string, chains map[string][]types.TransactionWithRole) (*types.RecoveryCommitment, error) {
	tokens := make([]types.RecoveryTokenCommitment, 0, len(chains))
	for tokenID, txs := range chains {
		ids := make([]string, len(txs))
		for i, tx := range txs {
			ids[i] = tx.Tx.ID
		}
		tc, err := recoverycommit.BuildTokenCommitment(tokenID, ids)
		if err != nil {
			return nil, fmt.Errorf("token %s: %w", tokenID, err)
		}
		tokens = append(tokens, tc)
	}
	tree, err := build(requestID, userDID, committerDID, tokens)
	if err != nil {
		return nil, err
	}
	return &tree.Commitment, nil
}
