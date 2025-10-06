package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
)

// RemoteRecoverTokens triggers token recovery on a remote node (exactly like UpdateTokenStatus)
func (c *Core) RemoteRecoverTokens(req *model.RemoteRecoveryRequest) (*TokenRecoveryResult, error) {
	c.log.Info("Initiating token recovery",
		"target_did", req.TargetDID,
		"transaction_id", req.TransactionID,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	// Validate the request
	if req.TargetDID == "" || req.TransactionID == "" {
		return nil, fmt.Errorf("target_did and transaction_id are required")
	}

	// Get peer connection to the target node (just like UpdateTokenStatus does)
	p, err := c.getPeer(req.TargetDID)
	if err != nil {
		c.log.Error("Failed to get peer for recovery", "target_did", req.TargetDID, "err", err)
		return nil, fmt.Errorf("failed to connect to target node: %v", err)
	}
	defer p.Close()

	// Prepare the recovery request for the remote node
	recoveryReq := map[string]interface{}{
		"sender_did":     req.TargetDID, // The target node is the sender for its own recovery
		"transaction_id": req.TransactionID,
	}

	// Send the recovery request to the remote node's recover-lost-tokens endpoint
	var recoveryResp struct {
		Status  bool                 `json:"status"`
		Message string               `json:"message"`
		Result  *TokenRecoveryResult `json:"result"`
	}

	c.log.Info("Sending recovery request to node",
		"target_did", req.TargetDID,
		"endpoint", setup.APIRecoverLostTokens)

	// Send JSON request to the remote node (just like UpdateTokenStatus does)
	err = p.SendJSONRequest("POST", setup.APIRecoverLostTokens, nil, &recoveryReq, &recoveryResp, false)
	if err != nil {
		// Check if this is a 404 error (endpoint not found)
		if isEndpointNotFoundError(err) {
			c.log.Warn("Target node doesn't have recovery endpoint, may be running older version",
				"target_did", req.TargetDID,
				"endpoint", setup.APIRecoverLostTokens,
				"error", err)
			return nil, fmt.Errorf("target node doesn't support remote recovery (endpoint %s not found). Ensure target node is running master-v4 or compatible version", setup.APIRecoverLostTokens)
		}
		
		c.log.Error("Failed to send recovery request to node",
			"target_did", req.TargetDID,
			"err", err)
		return nil, fmt.Errorf("remote recovery request failed: %v", err)
	}

	if !recoveryResp.Status {
		c.log.Error("Remote node failed to recover tokens",
			"target_did", req.TargetDID,
			"message", recoveryResp.Message)
		return nil, fmt.Errorf("remote recovery failed: %s", recoveryResp.Message)
	}

	// Log the successful recovery
	c.log.Info("Remote token recovery successful",
		"target_did", req.TargetDID,
		"transaction_id", req.TransactionID,
		"recovered_count", recoveryResp.Result.RecoveredTokenCount,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	// Add audit information about who requested it
	if recoveryResp.Result != nil && req.RequesterDID != "" {
		recoveryResp.Result.Message = fmt.Sprintf("[Remotely initiated by %s] %s", req.RequesterDID, recoveryResp.Result.Message)
	}

	return recoveryResp.Result, nil
}

// isEndpointNotFoundError checks if the error indicates a 404/endpoint not found
func isEndpointNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "404") || 
		   strings.Contains(errStr, "not found") || 
		   strings.Contains(errStr, "no route") ||
		   strings.Contains(errStr, "path not found")
}