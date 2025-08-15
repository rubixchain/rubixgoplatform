package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// RemoteRecoverTokens initiates token recovery on a remote node (Node B) from this node (Node A)
func (c *Core) RemoteRecoverTokens(req *model.RemoteRecoveryRequest) (*TokenRecoveryResult, error) {
	c.log.Info("========== INITIATING REMOTE TOKEN RECOVERY ==========",
		"target_did", req.TargetDID,
		"transaction_id", req.TransactionID,
		"requester_did", req.RequesterDID,
		"reason", req.Reason,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	// Step 1: Validate the request
	if req.TargetDID == "" || req.TransactionID == "" {
		c.log.Error("Invalid remote recovery request",
			"target_did", req.TargetDID,
			"transaction_id", req.TransactionID)
		return nil, fmt.Errorf("target_did and transaction_id are required")
	}

	// Step 2: Get the target node's peer connection
	c.log.Info("STEP 1: Connecting to target node",
		"target_did", req.TargetDID)
	
	targetPeer, err := c.getPeer(req.TargetDID)
	if err != nil {
		c.log.Error("STEP 1 FAILED: Cannot connect to target node",
			"target_did", req.TargetDID,
			"error", err)
		return nil, fmt.Errorf("failed to connect to target node: %v", err)
	}
	defer targetPeer.Close()
	
	c.log.Info("STEP 1 SUCCESS: Connected to target node")

	// Step 3: Send recovery request to target node
	c.log.Info("STEP 2: Sending recovery request to target node",
		"transaction_id", req.TransactionID)

	// Create the recovery request for the target node
	recoveryReq := map[string]interface{}{
		"sender_did":     req.TargetDID,  // The target node is the sender for its own recovery
		"transaction_id": req.TransactionID,
		"requester_did":  req.RequesterDID,
		"remote_request": true,
		"reason":         req.Reason,
	}

	// Send the recovery request to the target node
	var recoveryResp TokenRecoveryResult
	err = targetPeer.SendJSONRequest("POST", "/api/recover-lost-tokens", nil, &recoveryReq, &recoveryResp, true)
	if err != nil {
		c.log.Error("STEP 2 FAILED: Target node failed to recover tokens",
			"target_did", req.TargetDID,
			"transaction_id", req.TransactionID,
			"error", err)
		return nil, fmt.Errorf("target node failed to recover tokens: %v", err)
	}

	c.log.Info("STEP 2 SUCCESS: Target node recovered tokens",
		"recovered_count", recoveryResp.RecoveredTokenCount)

	// Step 4: Log the remote recovery in our database for audit
	c.log.Info("STEP 3: Recording remote recovery in local database")
	
	auditRecord := model.TokenRecovery{
		TransactionID: req.TransactionID,
		RecoveredAt:   time.Now(),
		RecoveredBy:   req.RequesterDID,
		TokenCount:    recoveryResp.RecoveredTokenCount,
		RecoveryType:  "remote",
		RecoveryNotes: fmt.Sprintf("Remote recovery initiated for %s, reason: %s", req.TargetDID, req.Reason),
	}
	
	err = c.w.GetStorage().Write("token_recovery_audit", &auditRecord)
	if err != nil {
		c.log.Warn("Failed to record remote recovery audit",
			"error", err)
		// Don't fail the operation if audit logging fails
	}

	c.log.Info("========== REMOTE TOKEN RECOVERY COMPLETED ==========",
		"target_did", req.TargetDID,
		"transaction_id", req.TransactionID,
		"recovered_tokens", recoveryResp.RecoveredTokenCount,
		"status", recoveryResp.Status,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	return &recoveryResp, nil
}

// HandleRemoteRecoveryRequest processes a recovery request from a remote node
// This is called on Node B when Node A requests recovery
func (c *Core) HandleRemoteRecoveryRequest(senderDID, transactionID string, requesterDID string, reason string) (*TokenRecoveryResult, error) {
	c.log.Info("========== PROCESSING REMOTE RECOVERY REQUEST ==========",
		"sender_did", senderDID,
		"transaction_id", transactionID,
		"requester_did", requesterDID,
		"reason", reason,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	// Verify that the sender DID matches our node's DID
	myDID := c.peerID
	if senderDID != myDID {
		c.log.Error("Remote recovery request DID mismatch",
			"requested_did", senderDID,
			"my_did", myDID)
		return nil, fmt.Errorf("sender DID does not match this node's DID")
	}

	// Log that this is a remote-initiated recovery
	c.log.Info("Remote recovery authorized",
		"requester", requesterDID,
		"reason", reason)

	// Perform the actual token recovery
	result, err := c.RecoverLostTokens(senderDID, transactionID)
	if err != nil {
		c.log.Error("Remote recovery failed",
			"error", err)
		return nil, err
	}

	// Update the recovery notes to indicate it was remote-initiated
	if result != nil {
		result.Message = fmt.Sprintf("Remote recovery initiated by %s: %s", requesterDID, result.Message)
	}

	c.log.Info("========== REMOTE RECOVERY REQUEST COMPLETED ==========",
		"recovered_tokens", result.RecoveredTokenCount,
		"status", result.Status)

	return result, nil
}

// VerifyRemoteRecoveryPermission checks if a remote recovery request is authorized
func (c *Core) VerifyRemoteRecoveryPermission(requesterDID, targetDID, transactionID string) bool {
	// You can add custom authorization logic here
	// For example, check if requester is in a whitelist, or has special permissions
	
	// For now, we'll allow any valid DID to request recovery
	// In production, you should implement proper authorization
	
	c.log.Info("Verifying remote recovery permission",
		"requester", requesterDID,
		"target", targetDID,
		"transaction", transactionID)
	
	// Basic validation
	if requesterDID == "" || targetDID == "" || transactionID == "" {
		return false
	}
	
	// You could add more checks here:
	// - Check if requester is in an authorized list
	// - Check if the transaction involves the requester
	// - Check time limits or other constraints
	
	return true
}