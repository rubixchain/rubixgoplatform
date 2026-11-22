package core

import (
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
