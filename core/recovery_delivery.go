package core

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// deliverRecoveryChains runs on a quorum when a session's commit window
// closes. Each quorum DID on this node that published a commitment sends the
// user a signed RecoveryChainsNotice on its own endpoint, the way a sender
// notifies a receiver with send_tokens. The user then pulls the chains from this node with
// sync_transaction_chain.
func (c *Core) deliverRecoveryChains(requestID string) {
	session, err := c.w.GetRecoverySession(requestID)
	if err != nil || session == nil || session.Role != constants.RecoveryRole_Quorum {
		if err != nil {
			c.log.Error("Recovery delivery: failed to read session", "request_txn_id", requestID, "err", err)
		}
		return
	}
	trees, err := c.w.GetRecoveryQuorumCommitments(requestID)
	if err != nil {
		c.log.Error("Recovery delivery: failed to read commitments", "request_txn_id", requestID, "err", err)
		return
	}
	for _, tree := range trees {
		quorumDID := tree.Commitment.CommitterDID
		if err := c.sendRecoveryChainsNotice(session.UserDID, tree); err != nil {
			c.log.Error("Recovery delivery failed", "request_txn_id", requestID, "quorum_did", quorumDID, "err", err)
			continue
		}
		c.log.Info("Recovery chains delivered", "request_txn_id", requestID, "quorum_did", quorumDID, "token_count", len(tree.Tokens))
	}
}

// sendRecoveryChainsNotice signs a notice for tree's committed tokens and
// sends it to userDID's node.
func (c *Core) sendRecoveryChainsNotice(userDID string, tree *types.RecoveryCommitmentTree) error {
	quorumDID := tree.Commitment.CommitterDID
	dc := c.pqc[quorumDID]
	if dc == nil {
		return fmt.Errorf("quorum %s is not set up on this node", quorumDID)
	}
	notice := &types.RecoveryChainsNotice{
		RequestID:    tree.Commitment.RequestID,
		QuorumDID:    quorumDID,
		QuorumPeerID: c.peerID,
		Tokens:       tree.Tokens,
	}
	if err := util.SignRecoveryChainsNotice(dc, notice); err != nil {
		return err
	}
	p, err := c.getPeer(userDID)
	if err != nil {
		return fmt.Errorf("user %s is not reachable: %w", userDID, err)
	}
	defer p.Close()
	var resp models.BasicResponse
	if err := p.SendJSONRequest("POST", APIRecoverySendTokens, nil, notice, &resp, true, 2*time.Minute); err != nil {
		return err
	}
	if !resp.Status {
		return fmt.Errorf("user rejected the notice: %s", resp.Message)
	}
	return nil
}

// RecoverySendTokens handles POST /rubix/v1/internal/recovery_send_tokens on
// the user's node: a quorum's notice that its committed chains are ready.
func (c *Core) RecoverySendTokens(request *ensweb.Request) *ensweb.Result {
	resp := models.BasicResponse{Status: false}
	var notice types.RecoveryChainsNotice
	if err := c.l.ParseJSON(request, &notice); err != nil {
		resp.Message = "invalid recovery chains notice"
		return c.l.RenderJSON(request, &resp, http.StatusBadRequest)
	}
	if err := c.receiveRecoveryChains(&notice); err != nil {
		c.log.Info("Recovery chains notice rejected", "request_txn_id", notice.RequestID, "quorum_did", notice.QuorumDID, "reason", err)
		resp.Message = err.Error()
		return c.l.RenderJSON(request, &resp, http.StatusBadRequest)
	}
	resp.Status = true
	resp.Message = "recovery chains received"
	return c.l.RenderJSON(request, &resp, http.StatusOK)
}

// receiveRecoveryChains checks a quorum's notice, pulls the chains from the
// quorum with sync_transaction_chain and stores them apart from the wallet. The notice is accepted only for this
// node's own recovery, inside the sync window, if it is signed by the quorum
// DID and that DID published a commitment for the same number of tokens.
func (c *Core) receiveRecoveryChains(notice *types.RecoveryChainsNotice) error {
	session, err := c.w.GetRecoverySession(notice.RequestID)
	if err != nil {
		return err
	}
	if session == nil || session.Role != constants.RecoveryRole_User {
		return fmt.Errorf("no recovery of a local DID for request %s", notice.RequestID)
	}
	if session.State == constants.RecoveryState_Blocked {
		return fmt.Errorf("recovery is blocked")
	}
	// The quorum sends when its commit window closes, so accept it from the
	// end of the commit window, without the grace period for clock drift.
	now := time.Now().Unix()
	if now < session.RequestEpoch+constants.RecoveryWindow_CommitSeconds ||
		now > recoveryDeadline(session.RequestEpoch, constants.RecoveryState_SyncClosed) {
		return fmt.Errorf("notice arrived outside the sync window")
	}
	dc, err := c.SetupForienDID(notice.QuorumDID, "")
	if err != nil {
		return fmt.Errorf("failed to set up quorum DID %s: %w", notice.QuorumDID, err)
	}
	if err := util.VerifyRecoveryChainsNotice(dc, notice); err != nil {
		return err
	}
	quorumAddr := notice.QuorumPeerID + "." + notice.QuorumDID
	if err := c.checkRecoveryNoticeCommitment(session, notice, quorumAddr); err != nil {
		return err
	}

	tokenIDs := make([]string, len(notice.Tokens))
	for i, t := range notice.Tokens {
		tokenIDs[i] = t.TokenID
	}
	chains, err := c.fetchTransactionChains(quorumAddr, tokenIDs)
	if err != nil {
		return err
	}
	saved, err := c.w.SaveRecoveryDelivery(notice.RequestID, notice.QuorumDID, chains)
	if err != nil {
		return err
	}
	if saved {
		c.log.Info("Recovery chains stored", "request_txn_id", notice.RequestID, "quorum_did", notice.QuorumDID,
			"token_count", len(chains), "committed_count", len(notice.Tokens))
	}
	return nil
}

// checkRecoveryNoticeCommitment checks that notice.QuorumDID published a
// commitment for this request covering as many tokens as the notice lists.
// If the commitment is not on the local ephemeral NFT chain, for example
// because its pubsub event was missed, the chain is synced from the quorum
// once.
func (c *Core) checkRecoveryNoticeCommitment(session *types.RecoverySession, notice *types.RecoveryChainsNotice, quorumAddr string) error {
	for synced := false; ; synced = true {
		chain, err := c.recoveryChain(session.EphemeralNFTID)
		if err != nil {
			return fmt.Errorf("failed to read the ephemeral NFT chain: %w", err)
		}
		for _, cm := range collectRecoveryCommitments(chain, session.UserDID) {
			if cm.CommitterDID != notice.QuorumDID || cm.CommitterType != constants.RecoveryCommitterType_Quorum || cm.RequestID != notice.RequestID {
				continue
			}
			if cm.TokenCount != len(notice.Tokens) {
				return fmt.Errorf("notice lists %d tokens, the commitment %d", len(notice.Tokens), cm.TokenCount)
			}
			return nil
		}
		if synced {
			return fmt.Errorf("quorum %s published no commitment for this request", notice.QuorumDID)
		}
		if err := c.SyncTransactionChainsFromPeer(quorumAddr, []string{session.EphemeralNFTID}, nil, nil, false, false); err != nil {
			return fmt.Errorf("quorum %s commitment not found and the ephemeral NFT sync failed: %w", notice.QuorumDID, err)
		}
	}
}

// fetchTransactionChains pulls tokenIDs' chains from peerAddr with
// sync_transaction_chain. Unlike SyncTransactionChainsFromPeer it does not
// apply them to the wallet.
func (c *Core) fetchTransactionChains(peerAddr string, tokenIDs []string) (map[string][]types.TransactionWithRole, error) {
	p, err := c.getPeer(peerAddr)
	if err != nil {
		return nil, fmt.Errorf("peer %s is not reachable: %w", peerAddr, err)
	}
	defer p.Close()
	req := syncTxChainRequest{DID: peerAddr, TokenIDs: tokenIDs}
	var resp syncTxChainResponse
	if err := p.SendJSONRequest("POST", APISyncTransactionChain, nil, &req, &resp, false, 2*time.Minute); err != nil {
		return nil, fmt.Errorf("chain sync from %s failed: %w", peerAddr, err)
	}
	if !resp.Status {
		return nil, fmt.Errorf("chain sync from %s failed: %s", peerAddr, resp.Message)
	}
	return resp.Data, nil
}
