package core

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// IPFSScalabilityManager handles dynamic scaling and load balancing
type IPFSScalabilityManager struct {
	core *Core
	log  logger.Logger

	// Scaling state
	mu                 sync.RWMutex
	currentLoad        int
	maxLoad            int
	scaleUpThreshold   int
	scaleDownThreshold int

	// Performance metrics
	avgResponseTime time.Duration
	errorRate       float64
	successRate     float64

	// Dynamic adjustment
	adjustmentInterval time.Duration
	lastAdjustment     time.Time

	// Control
	scalingCtx    context.Context
	scalingCancel context.CancelFunc
	scalingWg     sync.WaitGroup
}

// NewIPFSScalabilityManager creates a new scalability manager
func NewIPFSScalabilityManager(core *Core) *IPFSScalabilityManager {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &IPFSScalabilityManager{
		core:               core,
		log:                core.log.Named("IPFSScaling"),
		currentLoad:        0,
		maxLoad:            core.ipfsHealth.GetMaxConcurrency(),
		scaleUpThreshold:   80, // 80% of max load
		scaleDownThreshold: 20, // 20% of max load
		adjustmentInterval: 30 * time.Second,
		scalingCtx:         ctx,
		scalingCancel:      cancel,
	}

	// Start scaling monitor
	sm.startScalingMonitor()

	return sm
}

// startScalingMonitor monitors load and adjusts concurrency dynamically
func (sm *IPFSScalabilityManager) startScalingMonitor() {
	sm.scalingWg.Add(1)
	go func() {
		defer sm.scalingWg.Done()

		ticker := time.NewTicker(sm.adjustmentInterval)
		defer ticker.Stop()

		for {
			select {
			case <-sm.scalingCtx.Done():
				sm.log.Info("IPFS scaling monitor stopped")
				return
			case <-ticker.C:
				sm.adjustConcurrency()
			}
		}
	}()

	sm.log.Info("IPFS scaling monitor started")
}

// adjustConcurrency adjusts concurrency based on current load and performance
func (sm *IPFSScalabilityManager) adjustConcurrency() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get current statistics
	healthStats := sm.core.ipfsHealth.GetStats()
	currentLoad := healthStats["current_load"].(int)
	maxConcurrent := healthStats["max_concurrent"].(int)

	// Calculate load percentage
	loadPercentage := float64(currentLoad) / float64(maxConcurrent) * 100

	// Determine if scaling is needed
	var newLimit int
	reason := ""

	if loadPercentage >= float64(sm.scaleUpThreshold) && sm.successRate > 0.95 {
		// Scale up if load is high and success rate is good
		newLimit = int(float64(maxConcurrent) * 1.2) // Increase by 20%
		reason = "high load with good success rate"
	} else if loadPercentage <= float64(sm.scaleDownThreshold) && sm.errorRate > 0.05 {
		// Scale down if load is low but error rate is high
		newLimit = int(float64(maxConcurrent) * 0.8) // Decrease by 20%
		reason = "low load with high error rate"
	} else {
		// No adjustment needed
		return
	}

	// Apply limits
	newLimit = sm.applyLimits(newLimit)

	// Update concurrency limit
	if newLimit != maxConcurrent {
		sm.core.ipfsHealth.SetMaxConcurrency(newLimit)
		sm.log.Info("Adjusted IPFS concurrency",
			"old_limit", maxConcurrent,
			"new_limit", newLimit,
			"reason", reason,
			"load_percentage", loadPercentage,
			"success_rate", sm.successRate,
			"error_rate", sm.errorRate)
	}
}

// applyLimits applies system and configuration limits
func (sm *IPFSScalabilityManager) applyLimits(proposedLimit int) int {
	// System-based limits
	maxSystemLimit := runtime.NumCPU() * 10 // 10x CPU cores

	// Configuration-based limits
	maxConfigLimit := 50 // Maximum allowed concurrency

	// Apply minimum limit
	minLimit := 5

	// Apply limits
	if proposedLimit < minLimit {
		proposedLimit = minLimit
	}
	if proposedLimit > maxSystemLimit {
		proposedLimit = maxSystemLimit
	}
	if proposedLimit > maxConfigLimit {
		proposedLimit = maxConfigLimit
	}

	return proposedLimit
}

// UpdateMetrics updates performance metrics
func (sm *IPFSScalabilityManager) UpdateMetrics(responseTime time.Duration, success bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update response time (simple moving average)
	if sm.avgResponseTime == 0 {
		sm.avgResponseTime = responseTime
	} else {
		sm.avgResponseTime = (sm.avgResponseTime + responseTime) / 2
	}

	// Update success/error rates (simple moving average)
	if success {
		sm.successRate = sm.successRate*0.9 + 0.1
		sm.errorRate = sm.errorRate * 0.9
	} else {
		sm.successRate = sm.successRate * 0.9
		sm.errorRate = sm.errorRate*0.9 + 0.1
	}
}

// GetScalabilityStats returns scalability statistics
func (sm *IPFSScalabilityManager) GetScalabilityStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	healthStats := sm.core.ipfsHealth.GetStats()

	return map[string]interface{}{
		"current_load":         healthStats["current_load"],
		"max_concurrent":       healthStats["max_concurrent"],
		"available_slots":      healthStats["available_slots"],
		"avg_response_time":    sm.avgResponseTime,
		"success_rate":         sm.successRate,
		"error_rate":           sm.errorRate,
		"scale_up_threshold":   sm.scaleUpThreshold,
		"scale_down_threshold": sm.scaleDownThreshold,
		"last_adjustment":      sm.lastAdjustment,
	}
}

// SetScaleUpThreshold sets the threshold for scaling up
func (sm *IPFSScalabilityManager) SetScaleUpThreshold(threshold int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.scaleUpThreshold = threshold
	sm.log.Info("Updated scale up threshold", "threshold", threshold)
}

// SetScaleDownThreshold sets the threshold for scaling down
func (sm *IPFSScalabilityManager) SetScaleDownThreshold(threshold int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.scaleDownThreshold = threshold
	sm.log.Info("Updated scale down threshold", "threshold", threshold)
}

// Stop stops the scalability manager
func (sm *IPFSScalabilityManager) Stop() {
	sm.scalingCancel()
	sm.scalingWg.Wait()
	sm.log.Info("IPFS scalability manager stopped")
}