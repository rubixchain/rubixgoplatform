package core

import (
	"runtime"
	"runtime/debug"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// MemoryOptimizer helps manage memory usage during large operations
type MemoryOptimizer struct {
	log logger.Logger
}

// NewMemoryOptimizer creates a new memory optimizer
func NewMemoryOptimizer(log logger.Logger) *MemoryOptimizer {
	return &MemoryOptimizer{
		log: log.Named("MemoryOptimizer"),
	}
}

// OptimizeForLargeOperation prepares the system for a large operation
func (mo *MemoryOptimizer) OptimizeForLargeOperation(tokenCount int) {
	// Force garbage collection before starting
	runtime.GC()
	debug.FreeOSMemory()
	
	// Set GC percentage based on operation size
	// Lower values = more aggressive GC
	if tokenCount > 500 {
		debug.SetGCPercent(10) // Very aggressive GC for large operations
	} else if tokenCount > 100 {
		debug.SetGCPercent(50) // Moderate GC
	} else {
		debug.SetGCPercent(100) // Normal GC
	}
	
	// Set memory limit to prevent OOM
	// This is a soft limit that triggers GC more aggressively
	rm := &ResourceMonitor{}
	totalMB, _ := rm.GetMemoryStats()
	
	// Use 80% of total memory as soft limit
	softLimitBytes := int64(totalMB) * 1024 * 1024 * 80 / 100
	debug.SetMemoryLimit(softLimitBytes)
	
	mo.log.Info("Memory optimization configured", 
		"token_count", tokenCount,
		"gc_percent", debug.SetGCPercent(-1), // Read current value
		"memory_limit_mb", softLimitBytes/1024/1024)
}

// RestoreDefaults restores default memory settings
func (mo *MemoryOptimizer) RestoreDefaults() {
	debug.SetGCPercent(100)
	debug.SetMemoryLimit(-1) // Remove limit
	runtime.GC()
	mo.log.Debug("Memory settings restored to defaults")
}

// MonitorMemoryPressure monitors memory usage during operation
func (mo *MemoryOptimizer) MonitorMemoryPressure(done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	rm := &ResourceMonitor{}
	highPressureCount := 0
	
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			stats := rm.GetResourceStats()
			usagePct := stats["memory_usage_pct"].(float64)
			
			if usagePct > 90 {
				highPressureCount++
				mo.log.Warn("High memory pressure detected", 
					"usage_pct", usagePct,
					"consecutive_count", highPressureCount)
				
				// Force aggressive GC if sustained high pressure
				if highPressureCount >= 3 {
					runtime.GC()
					debug.FreeOSMemory()
					mo.log.Info("Forced garbage collection due to memory pressure")
					highPressureCount = 0
				}
			} else if usagePct > 80 {
				mo.log.Debug("Elevated memory usage", "usage_pct", usagePct)
				// Gentle GC hint
				runtime.GC()
			} else {
				highPressureCount = 0
			}
		}
	}
}

// PeriodicGC runs periodic garbage collection during long operations
func (mo *MemoryOptimizer) PeriodicGC(done <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			runtime.GC()
			mo.log.Debug("Periodic GC completed")
		}
	}
}