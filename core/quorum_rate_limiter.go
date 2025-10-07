package core

import (
	"context"
	"runtime"
	"sync"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// QuorumRateLimiter manages request rates to prevent quorum overload
type QuorumRateLimiter struct {
	log            logger.Logger
	mu             sync.RWMutex
	requestSlots   chan struct{}
	requestCounts  map[string]int // Track requests per quorum
	lastReset      time.Time
	maxConcurrent  int
	windowDuration time.Duration
}

// NewQuorumRateLimiter creates a new rate limiter for quorum requests
func NewQuorumRateLimiter(log logger.Logger) *QuorumRateLimiter {
	// Scale concurrent requests with available CPU cores
	// For 8-core systems, this allows 4 concurrent consensus operations
	maxConcurrent := runtime.NumCPU() / 2
	if maxConcurrent < 3 {
		maxConcurrent = 3 // Minimum of 3
	}
	
	return &QuorumRateLimiter{
		log:            log.Named("QuorumRateLimiter"),
		requestSlots:   make(chan struct{}, maxConcurrent),
		requestCounts:  make(map[string]int),
		lastReset:      time.Now(),
		maxConcurrent:  maxConcurrent,
		windowDuration: 1 * time.Minute,
	}
}

// AcquireSlot waits for an available request slot
func (qrl *QuorumRateLimiter) AcquireSlot(ctx context.Context, quorumID string) error {
	// Check if we need to reset counters
	qrl.maybeResetCounters()
	
	// Add delay based on request count for this quorum
	delay := qrl.calculateDelay(quorumID)
	if delay > 0 {
		qrl.log.Debug("Applying rate limit delay", "quorum", quorumID, "delay", delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	// Acquire a slot
	select {
	case qrl.requestSlots <- struct{}{}:
		qrl.incrementCount(quorumID)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSlot releases a request slot
func (qrl *QuorumRateLimiter) ReleaseSlot() {
	select {
	case <-qrl.requestSlots:
		// Slot released
	default:
		// Channel was empty, this shouldn't happen
		qrl.log.Warn("Attempted to release slot from empty channel")
	}
}

// calculateDelay returns delay based on recent request count
func (qrl *QuorumRateLimiter) calculateDelay(quorumID string) time.Duration {
	qrl.mu.RLock()
	count := qrl.requestCounts[quorumID]
	qrl.mu.RUnlock()
	
	// Progressive delays based on request count
	switch {
	case count < 5:
		return 0 // No delay for first few requests
	case count < 10:
		return 100 * time.Millisecond
	case count < 20:
		return 500 * time.Millisecond
	case count < 30:
		return 1 * time.Second
	default:
		return 2 * time.Second // Max delay
	}
}

// incrementCount increments the request count for a quorum
func (qrl *QuorumRateLimiter) incrementCount(quorumID string) {
	qrl.mu.Lock()
	qrl.requestCounts[quorumID]++
	qrl.mu.Unlock()
}

// maybeResetCounters resets counters if window has passed
func (qrl *QuorumRateLimiter) maybeResetCounters() {
	qrl.mu.Lock()
	defer qrl.mu.Unlock()
	
	if time.Since(qrl.lastReset) > qrl.windowDuration {
		qrl.requestCounts = make(map[string]int)
		qrl.lastReset = time.Now()
		qrl.log.Debug("Reset rate limit counters")
	}
}

// GetStats returns current rate limiter statistics
func (qrl *QuorumRateLimiter) GetStats() map[string]interface{} {
	qrl.mu.RLock()
	defer qrl.mu.RUnlock()
	
	// Copy request counts
	counts := make(map[string]int)
	for k, v := range qrl.requestCounts {
		counts[k] = v
	}
	
	return map[string]interface{}{
		"request_counts":    counts,
		"available_slots":   len(qrl.requestSlots),
		"max_concurrent":    qrl.maxConcurrent,
		"window_remaining":  qrl.windowDuration - time.Since(qrl.lastReset),
	}
}