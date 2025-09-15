package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InitializeAdvisoryNode checks if advisory node is available and enables it
func (c *Core) InitializeAdvisoryNode() {
	// Skip if logger not initialized
	if c.log == nil {
		c.advisoryNodeEnabled = false
		return
	}
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	
	// Check if advisory node is available
	resp, err := client.Get(c.advisoryNodeURL + "/api/quorum/health")
	if err != nil {
		c.log.Info("Advisory node not available, using local quorum management", "url", c.advisoryNodeURL, "err", err)
		c.advisoryNodeEnabled = false
		return
	}
	defer resp.Body.Close()

	c.advisoryNodeEnabled = true
	c.log.Info("Advisory node connected", "url", c.advisoryNodeURL)
}

// RegisterQuorumWithAdvisory registers a quorum with the advisory node
func (c *Core) RegisterQuorumWithAdvisory(didStr string, balance float64, didType int) error {
	if !c.advisoryNodeEnabled {
		return nil
	}

	registration := map[string]interface{}{
		"did":      didStr,
		"peer_id":  c.peerID,
		"balance":  balance,
		"did_type": didType,
	}

	jsonData, err := json.Marshal(registration)
	if err != nil {
		return fmt.Errorf("failed to marshal registration: %v", err)
	}

	resp, err := http.Post(
		c.advisoryNodeURL+"/api/quorum/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to register with advisory node: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed: %s", string(body))
	}

	c.log.Info("Quorum registered with advisory node", "did", didStr, "balance", balance)
	return nil
}

// ConfirmQuorumAvailability confirms a quorum is available in advisory node
func (c *Core) ConfirmQuorumAvailability(didStr string) error {
	if !c.advisoryNodeEnabled {
		return nil
	}

	confirmReq := map[string]string{"did": didStr}
	jsonData, err := json.Marshal(confirmReq)
	if err != nil {
		return fmt.Errorf("failed to marshal confirm request: %v", err)
	}

	resp, err := http.Post(
		c.advisoryNodeURL+"/api/quorum/confirm-availability",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to confirm availability: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("availability confirmation failed: %s", string(body))
	}

	c.log.Info("Availability confirmed with advisory node", "did", didStr)
	return nil
}

// MaintainQuorumHeartbeat sends periodic heartbeats for a quorum
func (c *Core) MaintainQuorumHeartbeat(didStr string) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if !c.advisoryNodeEnabled {
			return
		}

		// Send heartbeat
		reqBody := map[string]string{"did": didStr}
		jsonData, _ := json.Marshal(reqBody)

		resp, err := http.Post(
			c.advisoryNodeURL+"/api/quorum/heartbeat",
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			c.log.Error("Heartbeat failed", "did", didStr, "err", err)
			continue
		}
		resp.Body.Close()

		// Update balance
		balance := c.GetAccountBalance(didStr)
		c.UpdateQuorumBalance(didStr, balance)
	}
}

// UpdateQuorumBalance updates the balance for a quorum in advisory node
func (c *Core) UpdateQuorumBalance(didStr string, balance float64) error {
	if !c.advisoryNodeEnabled {
		return nil
	}

	update := map[string]interface{}{
		"did":     didStr,
		"balance": balance,
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal balance update: %v", err)
	}

	req, err := http.NewRequest(
		"PUT",
		c.advisoryNodeURL+"/api/quorum/balance",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create balance update request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update balance: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("balance update failed: %s", string(body))
	}

	return nil
}

// GetQuorumsFromAdvisory fetches available quorums from advisory node
func (c *Core) GetQuorumsFromAdvisory(transactionAmount float64, count int, lastCharTID string) []string {
	if !c.advisoryNodeEnabled {
		// Fall back to local quorum management
		c.log.Info("Advisory node disabled, using local quorum management")
		local := c.qm.GetQuorum(QuorumTypeTwo, lastCharTID, c.peerID)
		c.log.Info("Got quorums from local management", "count", len(local), "quorums", local)
		return local
	}

	// Build request URL with transaction amount
	// Note: Advisory node handles quorum selection without last character filtering
	url := fmt.Sprintf("%s/api/quorum/available?count=%d&transaction_amount=%.4f",
		c.advisoryNodeURL, count, transactionAmount)

	resp, err := http.Get(url)
	if err != nil {
		c.log.Error("Failed to get quorums from advisory node, falling back to local", "err", err)
		local := c.qm.GetQuorum(QuorumTypeTwo, lastCharTID, c.peerID)
		c.log.Info("Got quorums from local management (fallback)", "count", len(local), "quorums", local)
		return local
	}
	defer resp.Body.Close()

	var result struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Quorums []struct {
			Type    int    `json:"type"`
			Address string `json:"address"`
		} `json:"quorums"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.log.Error("Failed to decode advisory node response", "err", err)
		local := c.qm.GetQuorum(QuorumTypeTwo, lastCharTID, c.peerID)
		c.log.Info("Got quorums from local management (decode error)", "count", len(local), "quorums", local)
		return local
	}

	if !result.Status {
		c.log.Error("Advisory node returned error", "message", result.Message)
		local := c.qm.GetQuorum(QuorumTypeTwo, lastCharTID, c.peerID)
		c.log.Info("Got quorums from local management (status error)", "count", len(local), "quorums", local)
		return local
	}

	// Extract addresses
	addresses := make([]string, len(result.Quorums))
	for i, q := range result.Quorums {
		addresses[i] = q.Address
	}

	c.log.Info("Got quorums from advisory node", "count", len(addresses), "quorums", addresses)
	return addresses
}

// GetAccountBalance retrieves the current balance for a DID
func (c *Core) GetAccountBalance(did string) float64 {
	// Get account info
	accountInfo, err := c.GetAccountInfo(did)
	if err != nil {
		c.log.Error("Failed to get account balance", "did", did, "err", err)
		return 0.0
	}
	return accountInfo.RBTAmount
}