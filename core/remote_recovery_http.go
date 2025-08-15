package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// RemoteRecoverTokensHTTP initiates token recovery on a remote node using HTTP API
// This is an alternative implementation that uses direct HTTP calls instead of peer connections
func (c *Core) RemoteRecoverTokensHTTP(req *model.RemoteRecoveryRequest, targetNodeURL string) (*TokenRecoveryResult, error) {
	c.log.Info("========== INITIATING REMOTE TOKEN RECOVERY (HTTP) ==========",
		"target_url", targetNodeURL,
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

	// Ensure targetNodeURL has proper format
	if !strings.HasPrefix(targetNodeURL, "http://") && !strings.HasPrefix(targetNodeURL, "https://") {
		targetNodeURL = "http://" + targetNodeURL
	}
	
	// Remove trailing slash if present
	targetNodeURL = strings.TrimSuffix(targetNodeURL, "/")

	c.log.Info("STEP 1: Preparing recovery request for target node",
		"target_url", targetNodeURL,
		"target_did", req.TargetDID)

	// Step 2: Create the recovery request for the target node
	recoveryReq := map[string]interface{}{
		"sender_did":     req.TargetDID,  // The target node is the sender for its own recovery
		"transaction_id": req.TransactionID,
		"requester_did":  req.RequesterDID,
		"remote_request": true,
		"reason":         req.Reason,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(recoveryReq)
	if err != nil {
		c.log.Error("Failed to marshal recovery request",
			"error", err)
		return nil, fmt.Errorf("failed to marshal recovery request: %v", err)
	}

	c.log.Info("STEP 2: Sending HTTP recovery request to target node",
		"url", targetNodeURL+"/api/recover-lost-tokens",
		"transaction_id", req.TransactionID)

	// Step 3: Make HTTP request to target node
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	httpReq, err := http.NewRequest("POST", targetNodeURL+"/api/recover-lost-tokens", bytes.NewBuffer(jsonData))
	if err != nil {
		c.log.Error("Failed to create HTTP request",
			"error", err)
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := client.Do(httpReq)
	if err != nil {
		c.log.Error("STEP 2 FAILED: HTTP request to target node failed",
			"target_url", targetNodeURL,
			"error", err)
		return nil, fmt.Errorf("failed to send HTTP request to target node: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.log.Error("Failed to read response body",
			"error", err)
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Check HTTP status
	if resp.StatusCode == http.StatusNotFound {
		c.log.Error("STEP 2 FAILED: Target node does not have recovery endpoint",
			"target_url", targetNodeURL,
			"status", resp.StatusCode)
		return nil, fmt.Errorf("target node does not have the recovery API endpoint (404). Please ensure target node is running the latest version with recovery support")
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Error("STEP 2 FAILED: Target node returned error",
			"target_url", targetNodeURL,
			"status", resp.StatusCode,
			"response", string(body))
		return nil, fmt.Errorf("target node returned error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp struct {
		Status  bool                `json:"status"`
		Message string              `json:"message"`
		Result  *TokenRecoveryResult `json:"result"`
	}

	err = json.Unmarshal(body, &apiResp)
	if err != nil {
		c.log.Error("Failed to parse response from target node",
			"error", err,
			"response", string(body))
		return nil, fmt.Errorf("failed to parse response from target node: %v", err)
	}

	if !apiResp.Status {
		c.log.Error("STEP 2 FAILED: Target node recovery failed",
			"message", apiResp.Message)
		return nil, fmt.Errorf("target node recovery failed: %s", apiResp.Message)
	}

	if apiResp.Result == nil {
		c.log.Error("STEP 2 FAILED: No recovery result returned",
			"response", string(body))
		return nil, fmt.Errorf("no recovery result returned from target node")
	}

	c.log.Info("STEP 2 SUCCESS: Target node recovered tokens",
		"recovered_count", apiResp.Result.RecoveredTokenCount)

	// Step 4: Log the remote recovery in our database for audit
	c.log.Info("STEP 3: Recording remote recovery in local database")
	
	auditRecord := model.TokenRecovery{
		TransactionID: req.TransactionID,
		RecoveredAt:   time.Now(),
		RecoveredBy:   req.RequesterDID,
		TokenCount:    apiResp.Result.RecoveredTokenCount,
		RecoveryType:  "remote",
		RecoveryNotes: fmt.Sprintf("Remote recovery initiated for %s at %s, reason: %s", req.TargetDID, targetNodeURL, req.Reason),
	}
	
	err = c.w.GetStorage().Write("token_recovery", &auditRecord)
	if err != nil {
		c.log.Warn("Failed to record remote recovery audit",
			"error", err)
		// Don't fail the operation if audit logging fails
	}

	c.log.Info("========== REMOTE TOKEN RECOVERY COMPLETED (HTTP) ==========",
		"target_url", targetNodeURL,
		"target_did", req.TargetDID,
		"transaction_id", req.TransactionID,
		"recovered_tokens", apiResp.Result.RecoveredTokenCount,
		"status", apiResp.Result.Status,
		"timestamp", time.Now().Format("2006-01-02 15:04:05"))

	return apiResp.Result, nil
}