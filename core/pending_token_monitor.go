package core

import (
	"time"
)

// PendingTokenMonitor handles self-healing of stuck pending tokens
type PendingTokenMonitor struct {
	c        *Core
	interval time.Duration
	timeout  time.Duration
	stopCh   chan struct{}
}

// NewPendingTokenMonitor creates a new pending token monitor
func NewPendingTokenMonitor(c *Core, interval, timeout time.Duration) *PendingTokenMonitor {
	return &PendingTokenMonitor{
		c:        c,
		interval: interval,
		timeout:  timeout,
		stopCh:   make(chan struct{}),
	}
}

// Start begins monitoring for stuck pending tokens
func (m *PendingTokenMonitor) Start() {
	go m.run()
	m.c.log.Info("Started pending token monitor",
		"check_interval", m.interval,
		"pending_timeout", m.timeout)
}

// Stop halts the monitoring process
func (m *PendingTokenMonitor) Stop() {
	close(m.stopCh)
	m.c.log.Info("Stopped pending token monitor")
}

// run is the main monitoring loop
func (m *PendingTokenMonitor) run() {
	// Initial delay to let system stabilize after startup
	time.Sleep(30 * time.Second)
	
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			m.processPendingTokens()
		case <-m.stopCh:
			return
		}
	}
}

// processPendingTokens finds and confirms old pending tokens
func (m *PendingTokenMonitor) processPendingTokens() {
	startTime := time.Now()
	
	// Process pending FT tokens
	pendingFTs, err := m.c.w.GetPendingFTTokensOlderThan(m.timeout)
	if err != nil {
		m.c.log.Error("Failed to get pending FT tokens",
			"error", err)
		return
	}
	
	if len(pendingFTs) == 0 {
		// No pending tokens, nothing to do
		return
	}
	
	m.c.log.Info("Found pending FT tokens to process",
		"transaction_count", len(pendingFTs),
		"timeout_threshold", m.timeout)
	
	successCount := 0
	failCount := 0
	
	// Process each transaction
	for txID, tokenIDs := range pendingFTs {
		m.c.log.Info("Self-confirming pending FT tokens",
			"transaction_id", txID,
			"token_count", len(tokenIDs))
		
		// Attempt to confirm the tokens
		err := m.c.w.ConfirmPendingFTTokens(txID, tokenIDs)
		if err != nil {
			m.c.log.Error("Failed to self-confirm FT tokens",
				"transaction_id", txID,
				"token_count", len(tokenIDs),
				"error", err)
			failCount++
			continue
		}
		
		m.c.log.Info("Successfully self-confirmed FT tokens",
			"transaction_id", txID,
			"token_count", len(tokenIDs))
		successCount++
	}
	
	// Also process regular RBT tokens if needed
	pendingRBTs, err := m.c.w.GetPendingTokensOlderThan(m.timeout)
	if err != nil {
		m.c.log.Error("Failed to get pending RBT tokens",
			"error", err)
	} else if len(pendingRBTs) > 0 {
		m.c.log.Info("Found pending RBT tokens to process",
			"transaction_count", len(pendingRBTs))
		
		for txID, tokenIDs := range pendingRBTs {
			err := m.c.w.ConfirmPendingTokens(txID, tokenIDs)
			if err != nil {
				m.c.log.Error("Failed to self-confirm RBT tokens",
					"transaction_id", txID,
					"error", err)
				failCount++
			} else {
				m.c.log.Info("Successfully self-confirmed RBT tokens",
					"transaction_id", txID,
					"token_count", len(tokenIDs))
				successCount++
			}
		}
	}
	
	if successCount > 0 || failCount > 0 {
		m.c.log.Info("Pending token monitor cycle completed",
			"duration", time.Since(startTime),
			"success_count", successCount,
			"fail_count", failCount)
	}
}

// GetStats returns monitoring statistics
func (m *PendingTokenMonitor) GetStats() (totalProcessed, totalFailed int) {
	// This could be enhanced to track statistics over time
	return 0, 0
}