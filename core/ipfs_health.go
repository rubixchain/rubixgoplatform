package core

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// IPFSHealthManager manages IPFS health and provides global semaphore control
type IPFSHealthManager struct {
	ipfs *ipfsnode.Shell
	log  logger.Logger
	cfg  *config.Config

	// Health state
	mu              sync.RWMutex
	isHealthy       bool
	lastHealthCheck time.Time
	healthCheckURL  string

	// Global semaphore for all IPFS operations
	globalSem     chan struct{}
	maxConcurrent int

	// Health checker control
	healthCheckerCtx    context.Context
	healthCheckerCancel context.CancelFunc
	healthCheckerWg     sync.WaitGroup

	// Recovery state
	recoveryMu        sync.Mutex
	isRecovering      bool
	waitingGoroutines int32
}

// NewIPFSHealthManager creates a new IPFS health manager
func NewIPFSHealthManager(ipfs *ipfsnode.Shell, cfg *config.Config, log logger.Logger) *IPFSHealthManager {
	ctx, cancel := context.WithCancel(context.Background())

	hm := &IPFSHealthManager{
		ipfs:                ipfs,
		log:                 log.Named("IPFSHealth"),
		cfg:                 cfg,
		isHealthy:           false,
		healthCheckURL:      fmt.Sprintf("http://127.0.0.1:%d/api/v0/version", cfg.CfgData.Ports.IPFSPort),
		globalSem:           make(chan struct{}, 10), // Default to 10 concurrent operations
		maxConcurrent:       10,
		healthCheckerCtx:    ctx,
		healthCheckerCancel: cancel,
	}

	// Start health checker
	hm.startHealthChecker()

	return hm
}

// SetMaxConcurrency sets the maximum number of concurrent IPFS operations
func (hm *IPFSHealthManager) SetMaxConcurrency(max int) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Only update if the value actually changed
	if hm.maxConcurrent == max {
		return
	}

	// For now, just update the max value
	// We cannot safely replace the channel without risking panics
	// The actual concurrency will be limited by the current channel size
	oldMax := hm.maxConcurrent
	hm.maxConcurrent = max

	// Log the change but note that it won't take effect until restart
	hm.log.Info("IPFS concurrency limit updated (will take effect on next restart)", "old", oldMax, "new", max)
	
	// TODO: Implement a safer way to resize the semaphore channel
	// Options:
	// 1. Use a different concurrency control mechanism (e.g., worker pool)
	// 2. Implement a versioned semaphore system
	// 3. Require a restart for concurrency changes
}

// GetMaxConcurrency returns the current maximum concurrency limit
func (hm *IPFSHealthManager) GetMaxConcurrency() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.maxConcurrent
}

// IsHealthy returns whether IPFS is currently healthy
func (hm *IPFSHealthManager) IsHealthy() bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.isHealthy
}

// WaitForHealth blocks until IPFS is healthy or context is cancelled
func (hm *IPFSHealthManager) WaitForHealth(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if hm.IsHealthy() {
				return nil
			}
			hm.log.Debug("Waiting for IPFS to become healthy...")
			time.Sleep(1 * time.Second)
		}
	}
}

// AcquireSemaphore acquires the global IPFS semaphore and waits for health
func (hm *IPFSHealthManager) AcquireSemaphore(ctx context.Context) error {
	// First wait for IPFS to be healthy
	if err := hm.WaitForHealth(ctx); err != nil {
		return fmt.Errorf("IPFS health check failed: %w", err)
	}

	// Get the current semaphore under lock to avoid race conditions
	hm.mu.RLock()
	sem := hm.globalSem
	hm.mu.RUnlock()

	// Then acquire semaphore
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSemaphore releases the global IPFS semaphore
func (hm *IPFSHealthManager) ReleaseSemaphore() {
	// Get the current semaphore under lock to avoid race conditions
	hm.mu.RLock()
	sem := hm.globalSem
	hm.mu.RUnlock()

	select {
	case <-sem:
		// Successfully released
	default:
		// Semaphore was already empty, this shouldn't happen in normal operation
		hm.log.Warn("Attempted to release semaphore that was already empty")
	}
}

// ExecuteWithHealthCheck executes an IPFS operation with health checks and semaphore control
func (hm *IPFSHealthManager) ExecuteWithHealthCheck(ctx context.Context, operation func() error) error {
	// Acquire semaphore and wait for health
	if err := hm.AcquireSemaphore(ctx); err != nil {
		return err
	}
	defer hm.ReleaseSemaphore()

	// Execute operation with retry logic
	return hm.executeWithRetry(ctx, operation)
}

// executeWithRetry executes an operation with exponential backoff retry
func (hm *IPFSHealthManager) executeWithRetry(ctx context.Context, operation func() error) error {
	maxRetries := 5
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := operation()
		if err == nil {
			return nil
		}

		// Check if this is a connection error
		if hm.isConnectionError(err) {
			hm.log.Warn("IPFS connection error detected", "attempt", attempt+1, "err", err)

			// Mark as unhealthy and trigger recovery
			hm.markUnhealthy()

			// Wait for recovery or context cancellation
			if err := hm.WaitForHealth(ctx); err != nil {
				return fmt.Errorf("IPFS recovery failed: %w", err)
			}

			// Continue with retry
			continue
		}

		// For non-connection errors, use exponential backoff
		if attempt < maxRetries-1 {
			delay := time.Duration(1<<attempt) * baseDelay
			hm.log.Debug("Retrying IPFS operation", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("IPFS operation failed after %d retries", maxRetries)
}

// isConnectionError checks if an error is a connection-related error
func (hm *IPFSHealthManager) isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return contains(errStr, "connection refused") ||
		contains(errStr, "dial tcp") ||
		contains(errStr, "connect: connection refused") ||
		contains(errStr, "no route to host") ||
		contains(errStr, "network is unreachable") ||
		contains(errStr, "timeout") ||
		contains(errStr, "EOF") ||
		contains(errStr, "connection reset")
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}

// markUnhealthy marks IPFS as unhealthy
func (hm *IPFSHealthManager) markUnhealthy() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if hm.isHealthy {
		hm.isHealthy = false
		hm.log.Warn("IPFS marked as unhealthy", "api_url", hm.healthCheckURL)
	}
}

// markHealthy marks IPFS as healthy
func (hm *IPFSHealthManager) markHealthy() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if !hm.isHealthy {
		hm.isHealthy = true
		hm.lastHealthCheck = time.Now()
		hm.log.Info("IPFS marked as healthy", "api_url", hm.healthCheckURL)
	}
}

// checkHealth performs a health check against the IPFS API
func (hm *IPFSHealthManager) checkHealth() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", hm.healthCheckURL, nil)
	if err != nil {
		hm.log.Debug("Failed to create health check request", "err", err, "url", hm.healthCheckURL)
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		hm.log.Debug("Health check failed", "err", err, "url", hm.healthCheckURL)
		return false
	}
	defer resp.Body.Close()

	//hm.log.Debug("Health check response", "status", resp.StatusCode, "url", hm.healthCheckURL)
	return resp.StatusCode == 200
}

// startHealthChecker starts the background health checker
func (hm *IPFSHealthManager) startHealthChecker() {
	hm.healthCheckerWg.Add(1)
	go func() {
		defer hm.healthCheckerWg.Done()

		// Give IPFS time to fully initialize before starting health checks
		time.Sleep(3 * time.Second)

		// Use configured interval or default to 2 seconds
		interval := 2 * time.Second
		if hm.cfg.CfgData.IPFSRecovery != nil && hm.cfg.CfgData.IPFSRecovery.MonitorInterval > 0 {
			interval = hm.cfg.CfgData.IPFSRecovery.MonitorInterval
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Do an initial health check
		healthy := hm.checkHealth()
		if healthy {
			hm.markHealthy()
		} else {
			hm.markUnhealthy()
		}

		for {
			select {
			case <-hm.healthCheckerCtx.Done():
				hm.log.Info("IPFS health checker stopped")
				return
			case <-ticker.C:
				healthy := hm.checkHealth()

				if healthy {
					hm.markHealthy()
				} else {
					hm.markUnhealthy()
				}
			}
		}
	}()

	hm.log.Info("IPFS health checker started")
}

// Stop stops the health manager
func (hm *IPFSHealthManager) Stop() {
	hm.healthCheckerCancel()
	hm.healthCheckerWg.Wait()
	hm.log.Info("IPFS health manager stopped")
}

// GetStats returns current statistics about the health manager
func (hm *IPFSHealthManager) GetStats() map[string]interface{} {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	return map[string]interface{}{
		"is_healthy":        hm.isHealthy,
		"last_health_check": hm.lastHealthCheck,
		"max_concurrent":    hm.maxConcurrent,
		"current_load":      len(hm.globalSem),
		"available_slots":   cap(hm.globalSem) - len(hm.globalSem),
	}
}
