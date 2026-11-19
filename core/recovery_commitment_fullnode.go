package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/recoverycommit"
	"github.com/rubixchain/rubixgoplatform/did"
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

// RegisterFullnodeDID sets the local DID this fullnode uses to
// publish recovery messages. The DID is an ordinary DID created on this node.
// A test signature checks the password, which is kept in memory only, so
// the DID must be registered again after a restart.
func (c *Core) RegisterFullnodeDID(didStr, privKeyPassword string) error {
	if !c.fullNode {
		return fmt.Errorf("only a fullnode can register a fullnode recovery DID")
	}
	local, err := c.w.IsLocalDID(didStr)
	if err != nil {
		return err
	}
	if !local {
		return fmt.Errorf("DID %s is not a local DID on this node", didStr)
	}
	dc := did.InitDIDLiteWithPassword(didStr, c.didDir, privKeyPassword)
	hash := sha256.Sum256([]byte(didStr))
	if _, err := dc.Sign(hash[:]); err != nil {
		return fmt.Errorf("failed to sign with DID %s: %w", didStr, err)
	}
	c.fullnodeDCMu.Lock()
	c.fullnodeDC = dc
	c.fullnodeDCMu.Unlock()
	c.log.Info("Fullnode recovery DID registered", "did", didStr)
	return nil
}

// publishUnpledgedCommitment runs on a fullnode when a session opens its
// commit window, so the message arrives while commitments are accepted. The
// registered fullnode recovery DID builds the commitment over the user's
// unpledged tokens and executes the ephemeral NFT with it. It publishes even
// when the set is empty: that tells the user the fullnode found no free
// tokens, which silence would not.
func (c *Core) publishUnpledgedCommitment(s *types.RecoverySession) {
	c.fullnodeDCMu.RLock()
	dc := c.fullnodeDC
	c.fullnodeDCMu.RUnlock()
	if dc == nil {
		c.log.Error("Unpledged commitment not published: no fullnode recovery DID is registered", "request_txn_id", s.RequestID)
		return
	}
	if err := c.doPublishUnpledgedCommitment(s, dc); err != nil {
		c.log.Error("Failed to publish unpledged commitment", "request_txn_id", s.RequestID, "fullnode_did", dc.GetDID(), "err", err)
	}
}

func (c *Core) doPublishUnpledgedCommitment(s *types.RecoverySession, dc types.DIDCrypto) error {
	fullnodeDID := dc.GetDID()
	tree, excluded, err := c.ComputeUnpledgedCommitment(s.RequestID, s.UserDID, fullnodeDID)
	if err != nil {
		return err
	}
	for _, x := range excluded {
		c.log.Info("Unpledged commitment: token left out", "request_txn_id", s.RequestID, "token_id", x.TokenID, "reason", x.Reason)
	}
	body, err := json.Marshal(tree.Commitment)
	if err != nil {
		return fmt.Errorf("failed to encode commitment: %w", err)
	}
	msg := &types.RecoveryMessage{
		Type:      constants.RecoveryMessageType_Unpledged,
		Body:      body,
		SenderDID: fullnodeDID,
	}
	txID, err := c.executeEphemeralNFT(uuid.New().String(), dc, s.RequestID, msg)
	if err != nil {
		return err
	}
	c.log.Info("Unpledged commitment published", "request_txn_id", s.RequestID, "fullnode_did", fullnodeDID,
		"txn_id", txID, "token_count", tree.Commitment.TokenCount, "root", tree.Commitment.Root)
	return nil
}

// auditRecoveryCommitments runs on a fullnode when a session's commit window
// closes. It collects the commitments on the local ephemeral NFT chain,
// checks them with VerifyRecoveryCommitments and stores the report.
// Challenges are not published yet (M12); mismatches are only logged.
func (c *Core) auditRecoveryCommitments(requestID string) {
	session, err := c.w.GetRecoverySession(requestID)
	if err != nil || session == nil || session.Role != constants.RecoveryRole_Fullnode {
		if err != nil {
			c.log.Error("Recovery audit: failed to read session", "request_txn_id", requestID, "err", err)
		}
		return
	}
	chain, err := c.recoveryChain(session.EphemeralNFTID)
	if err != nil {
		c.log.Error("Recovery audit: failed to read the ephemeral NFT chain", "request_txn_id", requestID, "err", err)
		return
	}
	submitted := collectRecoveryCommitments(chain, session.UserDID)
	report, err := c.VerifyRecoveryCommitments(requestID, session.UserDID, submitted)
	if err != nil {
		c.log.Error("Recovery audit failed", "request_txn_id", requestID, "err", err)
		return
	}
	if err := c.w.SaveRecoveryCommitmentReport(report); err != nil {
		c.log.Error("Recovery audit: failed to store report", "request_txn_id", requestID, "err", err)
		return
	}
	matched := 0
	for _, chk := range report.Checks {
		if chk.Match {
			matched++
		}
	}
	c.log.Info("Recovery commitments audited", "request_txn_id", requestID, "submitted", len(report.Checks),
		"matched", matched, "missing_quorums", report.MissingQuorums)
}

// collectRecoveryCommitments returns the commitments in an ephemeral NFT
// chain. Messages that fail the sender rules are skipped, and so is a
// commitment whose committer_did is not its sender, so no DID can commit in
// another quorum's name.
func collectRecoveryCommitments(chain []types.RecoveryTx, userDID string) []types.RecoveryCommitment {
	var commitments []types.RecoveryCommitment
	for _, tx := range chain {
		msg, err := parseRecoveryMessage(tx.Data, tx.Info.Initiator, userDID)
		if err != nil || msg.Type != constants.RecoveryMessageType_Commitment {
			continue
		}
		var cm types.RecoveryCommitment
		if err := json.Unmarshal(msg.Body, &cm); err != nil || cm.CommitterDID != msg.SenderDID {
			continue
		}
		commitments = append(commitments, cm)
	}
	return commitments
}
