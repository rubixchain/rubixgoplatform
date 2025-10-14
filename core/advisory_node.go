package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// GetAdvisoryNodeURL gets the current advisory node URL with failover support
func (c *Core) GetAdvisoryNodeURL() (string, error) {
	// Determine network type
	networkType := "testnet"
	if !c.testNet {
		networkType = "mainnet"
	}

	// Get URLs for this network ordered by priority
	urls, err := c.w.GetAdvisoryURLsByNetwork(networkType)
	if err != nil {
		return "", fmt.Errorf("failed to get advisory URLs: %v", err)
	}

	if len(urls) == 0 {
		return "", fmt.Errorf("no advisory URLs configured for network: %s", networkType)
	}

	// Try URLs in order of priority (default first, then by priority)
	for _, url := range urls {
		if url.IsActive {
			// Test URL health
			if c.testAdvisoryNodeHealth(url.URL) {
				// Update health status if it was marked unhealthy
				if !url.IsHealthy {
					c.w.UpdateAdvisoryURLHealth(url.ID, true)
				}
				return url.URL, nil
			} else {
				// Mark as unhealthy
				c.w.UpdateAdvisoryURLHealth(url.ID, false)
				c.log.Warn("Advisory URL health check failed", "url", url.URL, "network", networkType)
			}
		}
	}

	return "", fmt.Errorf("no healthy advisory URLs available for network: %s", networkType)
}

// testAdvisoryNodeHealth tests if an advisory node URL is healthy
func (c *Core) testAdvisoryNodeHealth(url string) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url + "/api/quorum/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// InitializeAdvisoryNode checks if advisory node is available and enables it
func (c *Core) InitializeAdvisoryNode() {
	// Skip if logger not initialized
	if c.log == nil {
		c.advisoryNodeEnabled = false
		return
	}

	// Initialize default advisory URLs if none exist
	err := c.w.InitializeDefaultAdvisoryURLs()
	if err != nil {
		c.log.Error("Failed to initialize default advisory URLs, falling back to hardcoded URL", "err", err)
		// Fallback to hardcoded URL for now
		advisoryNodeURL := "https://mainnet-pool.universe.rubix.net"
		if c.testNet {
			advisoryNodeURL = c.advisoryNodeTestnetURL
		} else {
			advisoryNodeURL = c.advisoryNodeMainnetURL
		}
		// Test the hardcoded URL
		if c.testAdvisoryNodeHealth(advisoryNodeURL) {
			c.advisoryNodeEnabled = true
			c.log.Info("Advisory node connected using fallback URL", "url", advisoryNodeURL)
		} else {
			c.log.Info("No advisory node available, using local quorum management")
			c.advisoryNodeEnabled = false
		}

		// Advisory node fallback is connected - quorum setup will be handled by explicit setupquorum commands
		return
	}

	// Get advisory node URL with failover
	advisoryURL, err := c.GetAdvisoryNodeURL()
	if err != nil {
		c.log.Info("No advisory node available, using local quorum management", "err", err)
		c.advisoryNodeEnabled = false
		return
	}

	// Store the current working URL
	if c.testNet {
		c.advisoryNodeTestnetURL = advisoryURL
		c.advisoryNodeEnabled = true

	} else {
		c.advisoryNodeMainnetURL = advisoryURL
		c.advisoryNodeEnabled = true
	}
	//c.advisoryNodeURL = advisoryURL
	c.advisoryNodeEnabled = true
	c.log.Info("Advisory node connected", "url", advisoryURL)

	// Advisory node is connected - quorum setup will be handled by explicit setupquorum commands
}

// RegisterQuorumWithAdvisory registers a quorum with the advisory node
func (c *Core) RegisterQuorumWithAdvisory(didStr string, balance float64, didType int) error {
	if !c.advisoryNodeEnabled {
		return nil
	}

	// Get current advisory URL with failover
	advisoryURL, err := c.GetAdvisoryNodeURL()
	if err != nil {
		return fmt.Errorf("no advisory node available: %v", err)
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
		advisoryURL+"/api/quorum/register",
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

	c.log.Info("Quorum registered with advisory node", "did", didStr, "balance", balance, "url", advisoryURL)
	return nil
}

// ConfirmQuorumAvailability confirms a quorum is available in advisory node
func (c *Core) ConfirmQuorumAvailability(didStr string) error {
	if !c.advisoryNodeEnabled {
		return nil
	}

	// Get current advisory URL with failover
	advisoryURL, err := c.GetAdvisoryNodeURL()
	if err != nil {
		return fmt.Errorf("no advisory node available: %v", err)
	}

	confirmReq := map[string]string{"did": didStr}
	jsonData, err := json.Marshal(confirmReq)
	if err != nil {
		return fmt.Errorf("failed to marshal confirm request: %v", err)
	}

	resp, err := http.Post(
		advisoryURL+"/api/quorum/confirm-availability",
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

	c.log.Info("Availability confirmed with advisory node", "did", didStr, "url", advisoryURL)
	return nil
}

// MaintainQuorumHeartbeat sends periodic heartbeats for a quorum and ensures availability
func (c *Core) MaintainQuorumHeartbeat(didStr string) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Track consecutive failures for exponential backoff
	consecutiveFailures := 0
	availabilityConfirmCounter := 0

	for range ticker.C {
		if !c.advisoryNodeEnabled {
			return
		}

		// Get current advisory URL with failover
		advisoryURL, urlErr := c.GetAdvisoryNodeURL()
		if urlErr != nil {
			c.log.Error("No advisory node available for heartbeat", "did", didStr, "err", urlErr)
			continue
		}

		// Send heartbeat
		reqBody := map[string]string{"did": didStr}
		jsonData, _ := json.Marshal(reqBody)

		resp, err := http.Post(
			advisoryURL+"/api/quorum/heartbeat",
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			consecutiveFailures++
			c.log.Error("Heartbeat failed", "did", didStr, "err", err, "consecutive_failures", consecutiveFailures)

			// If heartbeat fails multiple times, try to re-confirm availability
			if consecutiveFailures >= 3 {
				c.log.Warn("Multiple heartbeat failures, attempting to re-confirm availability", "did", didStr)
				if confirmErr := c.ConfirmQuorumAvailability(didStr); confirmErr != nil {
					c.log.Error("Failed to re-confirm availability after heartbeat failures", "did", didStr, "err", confirmErr)
				} else {
					c.log.Info("Successfully re-confirmed availability after heartbeat failures", "did", didStr)
					consecutiveFailures = 0 // Reset failure counter on successful confirmation
				}
			}
			continue
		}
		resp.Body.Close()
		consecutiveFailures = 0 // Reset failure counter on successful heartbeat

		// Update balance
		balance := c.GetAccountBalance(didStr)
		c.UpdateQuorumBalance(didStr, balance)

		// Periodically re-confirm availability (every 10 heartbeats = ~20 minutes)
		availabilityConfirmCounter++
		if availabilityConfirmCounter >= 10 {
			c.log.Debug("Periodic availability confirmation", "did", didStr)
			if confirmErr := c.ConfirmQuorumAvailability(didStr); confirmErr != nil {
				c.log.Error("Periodic availability confirmation failed", "did", didStr, "err", confirmErr)
			} else {
				c.log.Debug("Periodic availability confirmation successful", "did", didStr)
			}
			availabilityConfirmCounter = 0 // Reset counter
		}
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

	// Get current advisory URL with failover
	advisoryURL, urlErr := c.GetAdvisoryNodeURL()
	if urlErr != nil {
		return fmt.Errorf("no advisory node available: %v", urlErr)
	}

	req, err := http.NewRequest(
		"PUT",
		advisoryURL+"/api/quorum/balance",
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

	// Get current advisory URL with failover
	advisoryURL, err := c.GetAdvisoryNodeURL()
	if err != nil {
		c.log.Error("No advisory node available, falling back to local", "err", err)
		local := c.qm.GetQuorum(QuorumTypeTwo, lastCharTID, c.peerID)
		c.log.Info("Got quorums from local management (no advisory)", "count", len(local), "quorums", local)
		return local
	}

	// Build request URL with transaction amount
	// Note: Advisory node handles quorum selection without last character filtering
	url := fmt.Sprintf("%s/api/quorum/available?count=%d&transaction_amount=%.4f",
		advisoryURL, count, transactionAmount)

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

// Public methods for server access to advisory URL management

// GetAllAdvisoryURLs returns all advisory URLs
func (c *Core) GetAllAdvisoryURLs() ([]model.AdvisoryURL, error) {
	return c.w.GetAllAdvisoryURLs()
}

// GetAdvisoryURLsByNetwork returns advisory URLs for a specific network
func (c *Core) GetAdvisoryURLsByNetwork(networkType string) ([]model.AdvisoryURL, error) {
	return c.w.GetAdvisoryURLsByNetwork(networkType)
}

// AddAdvisoryURL adds a new advisory URL
func (c *Core) AddAdvisoryURL(advisoryURL *model.AdvisoryURL) error {
	return c.w.AddAdvisoryURL(advisoryURL)
}

// UpdateAdvisoryURL updates an existing advisory URL
func (c *Core) UpdateAdvisoryURL(id int, updates *model.AdvisoryURL) error {
	return c.w.UpdateAdvisoryURL(id, updates)
}

// SetDefaultAdvisoryURL sets an advisory URL as default
func (c *Core) SetDefaultAdvisoryURL(id int) error {
	return c.w.SetDefaultAdvisoryURL(id)
}

// DeleteAdvisoryURL deletes an advisory URL
func (c *Core) DeleteAdvisoryURL(id int) error {
	return c.w.DeleteAdvisoryURL(id)
}

// GetDefaultAdvisoryURL gets the default advisory URL for a network
func (c *Core) GetDefaultAdvisoryURL(networkType string) (*model.AdvisoryURL, error) {
	return c.w.GetDefaultAdvisoryURL(networkType)
}

// IsTestNet returns true if the node is running on testnet
func (c *Core) IsTestNet() bool {
	return c.testNet
}
