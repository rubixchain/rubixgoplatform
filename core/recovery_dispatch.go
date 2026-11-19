package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// recoveryChain reads an NFT's chain from this node's database. The
// dispatcher reads events from here, not from the pubsub message, so it only
// uses transactions that passed chain sync.
func (c *Core) recoveryChain(nftID string) ([]types.RecoveryTx, error) {
	txs, err := c.w.GetTransactionsAndTokenRoleByTokenID(nftID)
	if err != nil {
		return nil, err
	}
	chain := make([]types.RecoveryTx, 0, len(txs))
	for _, t := range txs {
		tx := types.RecoveryTx{ID: t.Tx.ID, Role: t.Role}
		if err := json.Unmarshal(t.Tx.Info, &tx.Info); err != nil {
			return nil, fmt.Errorf("transaction %s: %w", t.Tx.ID, err)
		}
		if tx.Info.Tokens != nil {
			for _, nft := range tx.Info.Tokens.NFT {
				if nft != nil && nft.TokenID == nftID {
					tx.Data = nft.Data
					break
				}
			}
		}
		chain = append(chain, tx)
	}
	return chain, nil
}

// findRecoveryTx returns the position of txID in chain, or -1.
func findRecoveryTx(chain []types.RecoveryTx, txID string) int {
	for i := range chain {
		if chain[i].ID == txID {
			return i
		}
	}
	return -1
}

// dispatchRecoveryEvent handles a synced event on a recovery topic. peerID is
// the publisher's peer, or "" when this node published the event itself.
func (c *Core) dispatchRecoveryEvent(topic, txID, peerID string) {
	recoveryNFTID := c.RecoveryNFTID()
	if recoveryNFTID == "" {
		return
	}
	if topic == recoveryNFTID {
		if err := c.acceptRecoveryRequest(txID, peerID); err != nil {
			c.log.Info("Recovery request dropped", "request_txn_id", txID, "reason", err)
		}
		return
	}
	if err := c.handleEphemeralEvent(topic, txID); err != nil {
		c.log.Info("Recovery message dropped", "ephemeral_nft", topic, "txn_id", txID, "reason", err)
	}
}

// acceptRecoveryRequest checks a request on the Recovery NFT and, if this
// node takes part, creates its session and subscribes to the ephemeral NFT.
func (c *Core) acceptRecoveryRequest(txID, peerID string) error {
	chain, err := c.recoveryChain(c.RecoveryNFTID())
	if err != nil {
		return fmt.Errorf("failed to read the Recovery NFT chain: %w", err)
	}
	i := findRecoveryTx(chain, txID)
	if i < 0 {
		return fmt.Errorf("request transaction is not in the local Recovery NFT chain")
	}
	tx := chain[i]

	var request types.RecoveryRequest
	if err := json.Unmarshal([]byte(tx.Data), &request); err != nil {
		return fmt.Errorf("invalid request data: %w", err)
	}
	if tx.Info.Initiator != request.DID {
		return fmt.Errorf("transaction initiator %s is not the request DID %s", tx.Info.Initiator, request.DID)
	}
	if err := checkRecoveryRequestTiming(chain, i, time.Now().Unix()); err != nil {
		return err
	}
	dc, err := c.SetupForienDID(request.DID, "")
	if err != nil {
		return fmt.Errorf("failed to set up DID %s: %w", request.DID, err)
	}
	if err := util.VerifyRecoveryRequest(dc, &request, c.networkMode); err != nil {
		return err
	}

	role, err := c.recoveryRole(request.DID)
	if err != nil {
		return err
	}
	if role == "" {
		return nil // not a participant
	}
	ephemeralID, err := c.EphemeralNFTID(txID)
	if err != nil {
		return err
	}
	session := &types.RecoverySession{
		RequestID:      txID,
		EphemeralNFTID: ephemeralID,
		UserDID:        request.DID,
		Role:           role,
		State:          constants.RecoveryState_Requested,
		RequestEpoch:   int64(tx.Info.Epoch),
	}
	created, err := c.w.CreateRecoverySession(session)
	if err != nil {
		return err
	}
	if !created {
		return nil // duplicate event
	}
	c.log.Info("Recovery session created", "request_txn_id", txID, "user_did", request.DID, "role", role)
	c.subscribeRecoveryTopic(ephemeralID)
	c.scheduleRecoveryDeadlines(session)

	// The user may have deployed the ephemeral NFT before this node
	// subscribed, and pubsub does not replay it. Fetch the chain once.
	if peerID != "" {
		if err := c.SyncTransactionChainsFromPeer(peerID+"."+request.DID, []string{ephemeralID}, nil, nil, false, false); err != nil {
			c.log.Debug("Ephemeral NFT not fetched yet", "ephemeral_nft", ephemeralID, "err", err)
		}
		if err := c.handleEphemeralEvent(ephemeralID, ""); err != nil {
			c.log.Debug("Ephemeral NFT not deployed yet", "ephemeral_nft", ephemeralID, "err", err)
		}
	}
	return nil
}

// checkRecoveryRequestTiming applies the age rule (Q6) and the 48-hour rule
// (Q10) to the request at chain[i]. now is this node's Unix time in seconds.
// Earlier requests come from the shared chain, so every node agrees.
func checkRecoveryRequestTiming(chain []types.RecoveryTx, i int, now int64) error {
	req := chain[i].Info
	if now-int64(req.Epoch) > constants.RecoveryRequest_MaxAgeSeconds {
		return fmt.Errorf("request is older than %d seconds", constants.RecoveryRequest_MaxAgeSeconds)
	}
	deployRole := int16(models.GetTokenRoleID(constants.TokenRole_Deploy))
	for _, prev := range chain[:i] {
		if prev.Role == deployRole || prev.Info.Initiator != req.Initiator {
			continue
		}
		if req.Epoch-prev.Info.Epoch < constants.RecoveryRequest_CooldownSeconds {
			return fmt.Errorf("DID %s made a request less than 48 hours earlier (%s)", req.Initiator, prev.ID)
		}
	}
	return nil
}

// recoveryRole returns this node's role in a recovery of userDID, or "" if
// it does not take part. A quorum takes part only if it holds the user's
// pledged tokens (Q11).
func (c *Core) recoveryRole(userDID string) (string, error) {
	if local, err := c.w.IsLocalDID(userDID); err == nil && local {
		return constants.RecoveryRole_User, nil
	}
	if c.fullNode {
		return constants.RecoveryRole_Fullnode, nil
	}
	for quorumDID := range c.qc {
		if quorumDID == userDID {
			continue
		}
		pledged, err := c.w.GetRecoveryPledgedTokens(quorumDID, userDID)
		if err != nil {
			return "", err
		}
		if len(pledged) > 0 {
			return constants.RecoveryRole_Quorum, nil
		}
	}
	return "", nil
}

// handleEphemeralEvent handles transaction txID on an ephemeral NFT. With an
// empty txID it only checks whether the NFT has been deployed.
func (c *Core) handleEphemeralEvent(ephemeralID, txID string) error {
	session, err := c.w.GetRecoverySessionByEphemeralNFT(ephemeralID)
	if err != nil {
		return err
	}
	if session == nil {
		return nil // not a recovery topic, or this node does not take part
	}
	if session.State == constants.RecoveryState_Blocked {
		return fmt.Errorf("recovery is blocked")
	}
	chain, err := c.recoveryChain(ephemeralID)
	if err != nil {
		return fmt.Errorf("failed to read the ephemeral NFT chain: %w", err)
	}

	// The commit window opens once the user has deployed the ephemeral NFT.
	// Consensus checks the initiator's signature, so a deploy by any other
	// DID was not signed with the user's key, and the recovery is blocked.
	// Executions by other DIDs only drop that message (parseRecoveryMessage).
	deployRole := int16(models.GetTokenRoleID(constants.TokenRole_Deploy))
	if len(chain) == 0 || chain[0].Role != deployRole {
		return fmt.Errorf("ephemeral NFT is not deployed")
	}
	if chain[0].Info.Initiator != session.UserDID {
		if _, err := c.w.AdvanceRecoverySession(session.RequestID, constants.RecoveryState_Blocked); err != nil {
			return err
		}
		c.log.Error("Recovery blocked: ephemeral NFT was not deployed by the user", "request_txn_id", session.RequestID, "deployer", chain[0].Info.Initiator)
		return fmt.Errorf("ephemeral NFT was deployed by %s, not the user", chain[0].Info.Initiator)
	}
	opened, err := c.w.AdvanceRecoverySession(session.RequestID, constants.RecoveryState_CommitOpen)
	if err != nil {
		return err
	}
	// Only the event that opens the window publishes, so a repeated deploy
	// event does not publish twice. Publishing runs a transaction, so it
	// does not hold up this callback.
	if opened {
		switch session.Role {
		case constants.RecoveryRole_Quorum:
			go c.publishQuorumCommitments(session)
		case constants.RecoveryRole_Fullnode:
			go c.publishUnpledgedCommitment(session)
		}
	}
	if txID == "" || txID == chain[0].ID {
		return nil
	}

	i := findRecoveryTx(chain, txID)
	if i < 0 {
		return fmt.Errorf("transaction is not in the local ephemeral NFT chain")
	}
	msg, err := parseRecoveryMessage(chain[i].Data, chain[i].Info.Initiator, session.UserDID)
	if err != nil {
		return err
	}
	// Commitments are accepted only until the commit window closes.
	if session.State >= constants.RecoveryState_SyncOpen &&
		(msg.Type == constants.RecoveryMessageType_Commitment || msg.Type == constants.RecoveryMessageType_Unpledged) {
		return fmt.Errorf("%s from %s arrived after the commit window closed", msg.Type, msg.SenderDID)
	}
	// A fullnode reads the commitments from the chain when the commit window
	// closes (auditRecoveryCommitments). M9 acts on the user's messages.
	c.log.Info("Recovery message accepted", "request_txn_id", session.RequestID, "type", msg.Type, "sender_did", msg.SenderDID)
	return nil
}

// parseRecoveryMessage decodes a message from the ephemeral NFT and applies
// the sender rules (Q7). initiator is the DID that ran the transaction.
func parseRecoveryMessage(data, initiator, userDID string) (*types.RecoveryMessage, error) {
	var msg types.RecoveryMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return nil, fmt.Errorf("invalid message data: %w", err)
	}
	if msg.SenderDID != initiator {
		return nil, fmt.Errorf("sender %s is not the transaction initiator %s", msg.SenderDID, initiator)
	}
	switch msg.Type {
	case constants.RecoveryMessageType_Commitment, constants.RecoveryMessageType_Unpledged, constants.RecoveryMessageType_Challenge:
	case constants.RecoveryMessageType_Mismatch, constants.RecoveryMessageType_Completion:
		if initiator != userDID {
			return nil, fmt.Errorf("%s message from %s, not the user", msg.Type, initiator)
		}
	default:
		return nil, fmt.Errorf("unknown message type %q", msg.Type)
	}
	return &msg, nil
}
