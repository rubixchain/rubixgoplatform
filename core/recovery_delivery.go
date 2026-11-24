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

// deliverRecoveryChains runs on a quorum or a fullnode when a session's
// commit window closes. Each committer on this node that published a
// commitment with tokens sends the user a signed RecoveryChainsNotice on its
// own endpoint, the way a sender notifies a receiver with send_tokens. The
// user then pulls the chains from this node: a quorum's with
// sync_transaction_chain, the fullnode's with recovery_fullnode_chains.
func (c *Core) deliverRecoveryChains(requestID string) {
	session, err := c.w.GetRecoverySession(requestID)
	if err != nil || session == nil ||
		(session.Role != constants.RecoveryRole_Quorum && session.Role != constants.RecoveryRole_Fullnode) {
		if err != nil {
			c.log.Error("Recovery delivery: failed to read session", "request_txn_id", requestID, "err", err)
		}
		return
	}
	trees, err := c.w.GetRecoveryCommitmentTrees(requestID)
	if err != nil {
		c.log.Error("Recovery delivery: failed to read commitments", "request_txn_id", requestID, "err", err)
		return
	}
	for _, tree := range trees {
		committerDID := tree.Commitment.CommitterDID
		if len(tree.Tokens) == 0 {
			continue // an empty unpledged commitment needs no delivery
		}
		if err := c.sendRecoveryChainsNotice(session.UserDID, tree); err != nil {
			c.log.Error("Recovery delivery failed", "request_txn_id", requestID, "committer_did", committerDID, "err", err)
			continue
		}
		c.log.Info("Recovery chains delivered", "request_txn_id", requestID, "committer_did", committerDID, "token_count", len(tree.Tokens))
	}
}

// sendRecoveryChainsNotice signs a notice for tree's committed tokens with the
// committer's DID, a quorum DID or the registered fullnode recovery DID, and
// sends it to userDID's node.
func (c *Core) sendRecoveryChainsNotice(userDID string, tree *types.RecoveryCommitmentTree) error {
	committerDID := tree.Commitment.CommitterDID
	var dc types.DIDCrypto
	if tree.Commitment.CommitterType == constants.RecoveryCommitterType_Fullnode {
		c.fullnodeDCMu.RLock()
		dc = c.fullnodeDC
		c.fullnodeDCMu.RUnlock()
		if dc == nil || dc.GetDID() != committerDID {
			return fmt.Errorf("fullnode recovery DID %s is not registered on this node", committerDID)
		}
	} else if dc = c.pqc[committerDID]; dc == nil {
		return fmt.Errorf("quorum %s is not set up on this node", committerDID)
	}
	notice := &types.RecoveryChainsNotice{
		RequestID:       tree.Commitment.RequestID,
		CommitterDID:    committerDID,
		CommitterType:   tree.Commitment.CommitterType,
		CommitterPeerID: c.peerID,
		Tokens:          tree.Tokens,
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
// the user's node: a committer's notice that its committed chains are ready.
func (c *Core) RecoverySendTokens(request *ensweb.Request) *ensweb.Result {
	resp := models.BasicResponse{Status: false}
	var notice types.RecoveryChainsNotice
	if err := c.l.ParseJSON(request, &notice); err != nil {
		resp.Message = "invalid recovery chains notice"
		return c.l.RenderJSON(request, &resp, http.StatusBadRequest)
	}
	if err := c.receiveRecoveryChains(&notice); err != nil {
		c.log.Info("Recovery chains notice rejected", "request_txn_id", notice.RequestID, "committer_did", notice.CommitterDID, "reason", err)
		resp.Message = err.Error()
		return c.l.RenderJSON(request, &resp, http.StatusBadRequest)
	}
	resp.Status = true
	resp.Message = "recovery chains received"
	return c.l.RenderJSON(request, &resp, http.StatusOK)
}

// receiveRecoveryChains checks a committer's notice, pulls the chains from the
// committer and stores them apart from the wallet. The notice is accepted
// only for this node's own recovery, inside the sync window, if it is signed
// by the committer DID and that DID published a commitment of the notice's
// committer type for the same number of tokens: a quorum commitment for a
// quorum, an unpledged commitment for a fullnode.
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
	// The committer sends when its commit window closes, so accept it from
	// the end of the commit window, without the grace period for clock drift.
	now := time.Now().Unix()
	if now < session.RequestEpoch+constants.RecoveryWindow_CommitSeconds ||
		now > recoveryDeadline(session.RequestEpoch, constants.RecoveryState_SyncClosed) {
		return fmt.Errorf("notice arrived outside the sync window")
	}
	var chainsAPI string
	switch notice.CommitterType {
	case constants.RecoveryCommitterType_Quorum:
		chainsAPI = APISyncTransactionChain
	case constants.RecoveryCommitterType_Fullnode:
		chainsAPI = APIRecoveryFullnodeChains
	default:
		return fmt.Errorf("unknown committer type %q", notice.CommitterType)
	}
	dc, err := c.InitialiseDID(notice.CommitterDID)
	if err != nil {
		return fmt.Errorf("failed to set up committer DID %s: %w", notice.CommitterDID, err)
	}
	if err := util.VerifyRecoveryChainsNotice(dc, notice); err != nil {
		return err
	}
	// The signature covers the peer ID, so record it for later communication.
	c.AddPeerDetails(models.DID{DID: notice.CommitterDID, PeerID: notice.CommitterPeerID})
	committerAddr := notice.CommitterPeerID + "." + notice.CommitterDID
	if err := c.checkRecoveryNoticeCommitment(session, notice, committerAddr); err != nil {
		return err
	}

	tokenIDs := make([]string, len(notice.Tokens))
	for i, t := range notice.Tokens {
		tokenIDs[i] = t.TokenID
	}
	chains, err := c.fetchTransactionChains(committerAddr, chainsAPI, tokenIDs)
	if err != nil {
		return err
	}
	saved, err := c.w.SaveRecoveryDelivery(notice.RequestID, notice.CommitterDID, chains)
	if err != nil {
		return err
	}
	if saved {
		c.log.Info("Recovery chains stored", "request_txn_id", notice.RequestID, "committer_did", notice.CommitterDID,
			"committer_type", notice.CommitterType, "token_count", len(chains), "committed_count", len(notice.Tokens))
	}
	return nil
}

// checkRecoveryNoticeCommitment checks that notice.CommitterDID published a
// commitment of the notice's committer type for this request, covering as
// many tokens as the notice lists. If the commitment is not on the local
// ephemeral NFT chain, for example because its pubsub event was missed, the
// chain is synced from the committer once.
func (c *Core) checkRecoveryNoticeCommitment(session *types.RecoverySession, notice *types.RecoveryChainsNotice, committerAddr string) error {
	collect := collectRecoveryCommitments
	if notice.CommitterType == constants.RecoveryCommitterType_Fullnode {
		collect = collectUnpledgedCommitments
	}
	for synced := false; ; synced = true {
		chain, err := c.recoveryChain(session.EphemeralNFTID)
		if err != nil {
			return fmt.Errorf("failed to read the ephemeral NFT chain: %w", err)
		}
		for _, cm := range collect(chain, session.UserDID) {
			if cm.CommitterDID != notice.CommitterDID || cm.CommitterType != notice.CommitterType || cm.RequestID != notice.RequestID {
				continue
			}
			if cm.TokenCount != len(notice.Tokens) {
				return fmt.Errorf("notice lists %d tokens, the commitment %d", len(notice.Tokens), cm.TokenCount)
			}
			return nil
		}
		if synced {
			return fmt.Errorf("%s %s published no commitment for this request", notice.CommitterType, notice.CommitterDID)
		}
		if err := c.SyncTransactionChainsFromPeer(committerAddr, []string{session.EphemeralNFTID}, nil, nil, false, false); err != nil {
			return fmt.Errorf("%s %s commitment not found and the ephemeral NFT sync failed: %w", notice.CommitterType, notice.CommitterDID, err)
		}
	}
}

// fetchTransactionChains pulls tokenIDs' chains from peerAddr with chainsAPI,
// sync_transaction_chain or recovery_fullnode_chains. Unlike
// SyncTransactionChainsFromPeer it does not apply them to the wallet.
func (c *Core) fetchTransactionChains(peerAddr, chainsAPI string, tokenIDs []string) (map[string][]types.TransactionWithRole, error) {
	p, err := c.getPeer(peerAddr)
	if err != nil {
		return nil, fmt.Errorf("peer %s is not reachable: %w", peerAddr, err)
	}
	defer p.Close()
	req := syncTxChainRequest{DID: peerAddr, TokenIDs: tokenIDs}
	var resp syncTxChainResponse
	if err := p.SendJSONRequest("POST", chainsAPI, nil, &req, &resp, false, 2*time.Minute); err != nil {
		return nil, fmt.Errorf("chain sync from %s failed: %w", peerAddr, err)
	}
	if !resp.Status {
		return nil, fmt.Errorf("chain sync from %s failed: %s", peerAddr, resp.Message)
	}
	return resp.Data, nil
}

// RecoveryFullnodeChains handles POST /rubix/v1/internal/recovery_fullnode_chains
// on a fullnode. It takes and returns the same bodies as
// sync_transaction_chain, but reads the chains the fullnode keeps for other
// DIDs, so the user can pull the chains of the fullnode's unpledged
// commitment.
func (c *Core) RecoveryFullnodeChains(request *ensweb.Request) *ensweb.Result {
	var req syncTxChainRequest
	if err := c.l.ParseJSON(request, &req); err != nil {
		return c.l.RenderJSON(request, &syncTxChainResponse{Message: "failed to parse request"}, http.StatusOK)
	}
	if !c.fullNode {
		return c.l.RenderJSON(request, &syncTxChainResponse{Message: "this node is not a fullnode"}, http.StatusOK)
	}
	chains, err := c.w.GetFullNodeTransactionChains(req.TokenIDs)
	if err != nil {
		c.log.Error("RecoveryFullnodeChains: failed to read chains", "err", err)
		return c.l.RenderJSON(request, &syncTxChainResponse{Message: "failed to read chains"}, http.StatusOK)
	}
	return c.l.RenderJSON(request, &syncTxChainResponse{Status: true, Message: "ok", Data: chains}, http.StatusOK)
}
