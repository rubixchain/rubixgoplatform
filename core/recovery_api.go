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

// StartRecovery runs the user's side of starting a recovery of did, for the
// start recovery API. It answers on reqID's channel with the request
// transaction ID as the result.
func (c *Core) StartRecovery(reqID, did string) {
	br := c.startRecovery(reqID, did)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

// startRecovery signs a recovery request for did, puts it on the Recovery NFT
// and deploys the request's ephemeral NFT, which opens the commit window.
func (c *Core) startRecovery(reqID, did string) *models.BasicResponse {
	resp := &models.BasicResponse{}
	nftID := c.RecoveryNFTID()
	if nftID == "" {
		resp.Message = "recovery is off: no Recovery NFT ID is set for this network"
		return resp
	}
	// Executing an NFT this node does not hold would deploy it a second time.
	exists, err := c.w.TokenExists(c.Ctx, nil, nftID)
	if err != nil {
		resp.Message = fmt.Sprintf("failed to check Recovery NFT %s: %v", nftID, err)
		return resp
	}
	if !exists {
		resp.Message = fmt.Sprintf("Recovery NFT %s is not on this node; sync it before starting a recovery", nftID)
		return resp
	}
	// Every node drops a request made within 48 hours of the last one, so
	// refuse it here instead of putting it on the chain.
	chain, err := c.recoveryChain(nftID)
	if err != nil {
		resp.Message = "failed to read the Recovery NFT chain: " + err.Error()
		return resp
	}
	now := time.Now().Unix()
	if err := checkRecoveryCooldown(chain, did, now); err != nil {
		resp.Message = err.Error()
		return resp
	}

	request := &types.RecoveryRequest{
		DID:     did,
		Network: c.networkMode,
		Purpose: constants.RecoveryRequest_Purpose,
		Epoch:   now,
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "failed to set up DID: " + err.Error()
		return resp
	}
	if err := util.SignRecoveryRequest(dc, request); err != nil {
		resp.Message = err.Error()
		return resp
	}
	txID, err := c.ExecuteRecoveryNFT(reqID, request)
	if err != nil {
		resp.Message = err.Error()
		return resp
	}
	// ExecuteRecoveryNFT opens this node's session. Without one the request
	// failed a check, and deploying the ephemeral NFT would achieve nothing.
	session, err := c.w.GetRecoverySession(txID)
	if err != nil || session == nil {
		resp.Message = fmt.Sprintf("request %s is on the Recovery NFT, but this node did not accept it; see the node log", txID)
		return resp
	}
	if err := c.DeployEphemeralNFT(reqID, did, txID); err != nil {
		resp.Message = fmt.Sprintf("request %s sent, but deploying its ephemeral NFT failed: %v", txID, err)
		return resp
	}
	resp.Status = true
	resp.Message = fmt.Sprintf("Recovery started: request %s, ephemeral NFT %s", txID, session.EphemeralNFTID)
	// A string result tells the CLI signature loop that no more signatures
	// are needed.
	resp.Result = txID
	return resp
}

// PublishRecoveryOutcome publishes the user's outcome of the recovery started
// by requestID, for the outcome API. It answers on reqID's channel with the
// outcome's transaction ID as the result.
func (c *Core) PublishRecoveryOutcome(reqID, requestID string) {
	br := c.publishRecoveryOutcome(reqID, requestID)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

// publishRecoveryOutcome executes the ephemeral NFT with a mismatch report if
// any check of the user's verification failed, else with a completion notice
// [R13]. It runs the verification first if it has not run. After completion
// it writes the delivered chains into the wallet. It refuses before the sync
// window closes and after a mismatch; if completion is already on the chain
// it only writes the wallet again.
func (c *Core) publishRecoveryOutcome(reqID, requestID string) *models.BasicResponse {
	resp := &models.BasicResponse{}
	session, err := c.w.GetRecoverySession(requestID)
	if err != nil {
		resp.Message = "failed to read the recovery session: " + err.Error()
		return resp
	}
	if session == nil || session.Role != constants.RecoveryRole_User {
		resp.Message = fmt.Sprintf("no recovery of a local DID for request %s", requestID)
		return resp
	}
	switch session.State {
	case constants.RecoveryState_SyncClosed:
	case constants.RecoveryState_Blocked:
		resp.Message = "recovery is blocked"
		return resp
	default:
		resp.Message = fmt.Sprintf("the sync window closes at %d; publish the outcome after that",
			recoveryDeadline(session.RequestEpoch, constants.RecoveryState_SyncClosed))
		return resp
	}
	chain, err := c.recoveryChain(session.EphemeralNFTID)
	if err != nil {
		resp.Message = "failed to read the ephemeral NFT chain: " + err.Error()
		return resp
	}
	for _, tx := range chain {
		msg, err := parseRecoveryMessage(tx.Data, tx.Info.Initiator, session.UserDID)
		if err != nil {
			continue
		}
		switch msg.Type {
		case constants.RecoveryMessageType_Completion:
			// Writing the wallet is idempotent, so a repeated call retries it.
			return c.completeRecovery(session, tx.ID, "Recovery completion already published")
		case constants.RecoveryMessageType_Mismatch:
			resp.Message = fmt.Sprintf("the outcome is already published: mismatch in transaction %s", tx.ID)
			return resp
		}
	}
	v, err := c.recoveryVerification(session)
	if err != nil {
		resp.Message = "recovery verification failed: " + err.Error()
		return resp
	}

	var failed []types.RecoveryDeliveryCheck
	for _, chk := range v.Checks {
		if !chk.Match {
			failed = append(failed, chk)
		}
	}
	msg := &types.RecoveryMessage{Type: constants.RecoveryMessageType_Completion, SenderDID: session.UserDID}
	var body interface{} = types.RecoveryCompletionNotice{RequestID: requestID}
	if len(failed) > 0 {
		msg.Type = constants.RecoveryMessageType_Mismatch
		body = types.RecoveryMismatchReport{RequestID: requestID, Mismatches: failed}
	}
	if msg.Body, err = json.Marshal(body); err != nil {
		resp.Message = "failed to encode the outcome: " + err.Error()
		return resp
	}
	txID, err := c.ExecuteEphemeralNFT(reqID, requestID, msg)
	if err != nil {
		resp.Message = err.Error()
		return resp
	}
	c.log.Info("Recovery outcome published", "request_txn_id", requestID, "type", msg.Type,
		"failed_checks", len(failed), "txn_id", txID)
	if len(failed) == 0 {
		return c.completeRecovery(session, txID, fmt.Sprintf("Recovery completion published: %d commitments matched", len(v.Checks)))
	}
	resp.Status = true
	resp.Message = fmt.Sprintf("Recovery mismatch published: %d of %d commitments failed, transaction %s", len(failed), len(v.Checks), txID)
	// A string result tells the CLI signature loop that no more signatures
	// are needed.
	resp.Result = txID
	return resp
}

// completeRecovery writes the delivered chains into the wallet once the
// completion in transaction txID is on the chain, and reports it after done.
func (c *Core) completeRecovery(session *types.RecoverySession, txID, done string) *models.BasicResponse {
	resp := &models.BasicResponse{Result: txID}
	persisted, skipped, err := c.persistRecoveredChains(session)
	if err != nil {
		resp.Message = fmt.Sprintf("%s in transaction %s, but writing the wallet failed: %v; call the outcome API again to retry", done, txID, err)
		return resp
	}
	c.log.Info("Recovered chains written to the wallet", "request_txn_id", session.RequestID, "tokens", persisted, "skipped", skipped)
	resp.Status = true
	resp.Message = fmt.Sprintf("%s in transaction %s; %d tokens written to the wallet, %d skipped (see the node log)", done, txID, persisted, skipped)
	return resp
}

// GetRecoverySessionStatus returns this node's view of the recovery started
// by requestID, or nil if this node has no session for it.
func (c *Core) GetRecoverySessionStatus(requestID string) (*types.RecoverySessionStatus, error) {
	s, err := c.w.GetRecoverySession(requestID)
	if err != nil || s == nil {
		return nil, err
	}
	status := &types.RecoverySessionStatus{
		RequestID:      s.RequestID,
		EphemeralNFTID: s.EphemeralNFTID,
		UserDID:        s.UserDID,
		Role:           s.Role,
		State:          recoveryStateName(s.State),
		RequestEpoch:   s.RequestEpoch,
		CommitClosesAt: recoveryDeadline(s.RequestEpoch, constants.RecoveryState_SyncOpen),
		SyncClosesAt:   recoveryDeadline(s.RequestEpoch, constants.RecoveryState_SyncClosed),
		Messages:       []types.RecoveryMessageEntry{},
	}
	chain, err := c.recoveryChain(s.EphemeralNFTID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the ephemeral NFT chain: %w", err)
	}
	for _, tx := range chain {
		msg, err := parseRecoveryMessage(tx.Data, tx.Info.Initiator, s.UserDID)
		if err != nil {
			continue // the deploy, or a message that fails the sender rules
		}
		status.Messages = append(status.Messages, types.RecoveryMessageEntry{
			TxnID: tx.ID, Type: msg.Type, SenderDID: msg.SenderDID, Body: msg.Body,
		})
	}
	switch s.Role {
	case constants.RecoveryRole_Fullnode:
		if status.Report, err = c.w.GetRecoveryCommitmentReport(requestID); err != nil {
			return nil, err
		}
	case constants.RecoveryRole_User:
		if status.Deliveries, err = c.w.GetRecoveryDeliveries(requestID); err != nil {
			return nil, err
		}
		if status.Verification, err = c.w.GetRecoveryVerification(requestID); err != nil {
			return nil, err
		}
	}
	return status, nil
}

func recoveryStateName(state int) string {
	switch state {
	case constants.RecoveryState_Requested:
		return "requested"
	case constants.RecoveryState_CommitOpen:
		return "commit_open"
	case constants.RecoveryState_SyncOpen:
		return "sync_open"
	case constants.RecoveryState_SyncClosed:
		return "sync_closed"
	case constants.RecoveryState_Blocked:
		return "blocked"
	}
	return fmt.Sprintf("unknown (%d)", state)
}
