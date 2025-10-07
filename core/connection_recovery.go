package core

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// ConnectionRecovery manages connection recovery after IPFS restarts
type ConnectionRecovery struct {
	log              logger.Logger
	mu               sync.RWMutex
	failedConnections map[string]time.Time // Track failed connections
	recoveryDelay     time.Duration
	maxRetries        int
}

// NewConnectionRecovery creates a new connection recovery manager
func NewConnectionRecovery(log logger.Logger) *ConnectionRecovery {
	return &ConnectionRecovery{
		log:               log.Named("ConnectionRecovery"),
		failedConnections: make(map[string]time.Time),
		recoveryDelay:     5 * time.Second, // Wait 5 seconds after IPFS recovery
		maxRetries:        5,
	}
}

// IsConnectionError checks if an error is a connection-related error
func (cr *ConnectionRecovery) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	return contains(errStr, "EOF") ||
		contains(errStr, "connection reset by peer") ||
		contains(errStr, "broken pipe") ||
		contains(errStr, "connection refused") ||
		contains(errStr, "no such host") ||
		contains(errStr, "connection closed")
}

// MarkConnectionFailed marks a connection as failed
func (cr *ConnectionRecovery) MarkConnectionFailed(endpoint string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	
	cr.failedConnections[endpoint] = time.Now()
	cr.log.Debug("Marked connection as failed", "endpoint", endpoint)
}

// ShouldWaitForRecovery checks if we should wait before retrying
func (cr *ConnectionRecovery) ShouldWaitForRecovery(endpoint string) (bool, time.Duration) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	
	failedAt, exists := cr.failedConnections[endpoint]
	if !exists {
		return false, 0
	}
	
	timeSinceFail := time.Since(failedAt)
	if timeSinceFail < cr.recoveryDelay {
		waitTime := cr.recoveryDelay - timeSinceFail
		return true, waitTime
	}
	
	return false, 0
}

// ClearFailedConnection removes a connection from failed list
func (cr *ConnectionRecovery) ClearFailedConnection(endpoint string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	
	delete(cr.failedConnections, endpoint)
	cr.log.Debug("Cleared failed connection", "endpoint", endpoint)
}

// ExecuteWithRecovery executes an HTTP request with connection recovery
func (cr *ConnectionRecovery) ExecuteWithRecovery(
	ctx context.Context,
	endpoint string,
	requestFunc func() (*http.Response, error),
) (*http.Response, error) {
	
	// Check if we should wait for recovery
	if shouldWait, waitTime := cr.ShouldWaitForRecovery(endpoint); shouldWait {
		cr.log.Info("Waiting for connection recovery", 
			"endpoint", endpoint, 
			"wait_time", waitTime)
		
		select {
		case <-time.After(waitTime):
			// Continue after wait
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	var lastErr error
	for attempt := 0; attempt < cr.maxRetries; attempt++ {
		// Add exponential backoff between retries
		if attempt > 0 {
			backoff := time.Duration(attempt) * 2 * time.Second
			cr.log.Debug("Retry backoff", 
				"attempt", attempt, 
				"backoff", backoff,
				"endpoint", endpoint)
			
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		resp, err := requestFunc()
		
		if err == nil {
			// Success - clear any failed connection marker
			cr.ClearFailedConnection(endpoint)
			return resp, nil
		}
		
		lastErr = err
		
		// Check if this is a connection error
		if cr.IsConnectionError(err) {
			cr.log.Warn("Connection error detected", 
				"endpoint", endpoint,
				"attempt", attempt+1,
				"error", err)
			
			// Mark connection as failed
			cr.MarkConnectionFailed(endpoint)
			
			// For first connection error, wait longer for IPFS recovery
			if attempt == 0 {
				cr.log.Info("Waiting for IPFS recovery after connection failure",
					"wait_time", cr.recoveryDelay)
				
				select {
				case <-time.After(cr.recoveryDelay):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			
			continue
		}
		
		// Non-connection error, don't retry
		return nil, err
	}
	
	return nil, fmt.Errorf("failed after %d retries: %w", cr.maxRetries, lastErr)
}

// WaitForIPFSRecovery waits for IPFS to recover on all quorums
func (cr *ConnectionRecovery) WaitForIPFSRecovery(quorumAddrs []string) {
	cr.log.Info("Waiting for IPFS recovery on all quorums", 
		"quorum_count", len(quorumAddrs),
		"recovery_delay", cr.recoveryDelay)
	
	// Wait for standard recovery delay
	time.Sleep(cr.recoveryDelay)
	
	// Additional wait based on number of quorums
	additionalWait := time.Duration(len(quorumAddrs)) * 500 * time.Millisecond
	if additionalWait > 5*time.Second {
		additionalWait = 5 * time.Second
	}
	
	time.Sleep(additionalWait)
	
	cr.log.Info("IPFS recovery wait complete")
}