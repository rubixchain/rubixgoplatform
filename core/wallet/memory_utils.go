package wallet

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

// ResourceMonitor provides system resource information
type ResourceMonitor struct{}

// GetMemoryStats returns current memory statistics
func (rm *ResourceMonitor) GetMemoryStats() (totalMB, availableMB uint64) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Use runtime memory stats as a cross-platform solution
	// This gives us Go's view of memory usage
	totalMB = memStats.Sys / (1024 * 1024) // Total memory obtained from OS
	
	// Available memory is roughly: system memory - allocated memory
	allocatedMB := memStats.Alloc / (1024 * 1024)
	if totalMB > allocatedMB {
		availableMB = totalMB - allocatedMB
	} else {
		availableMB = 1024 // Default 1GB available
	}
	
	return totalMB, availableMB
}

// CalculateDynamicWorkers determines optimal worker count based on:
// - Available system memory
// - Number of tokens to process
// - Estimated memory per operation
func (rm *ResourceMonitor) CalculateDynamicWorkers(tokenCount int) int {
	totalMB, availableMB := rm.GetMemoryStats()
	
	// Reserve memory for system stability (keep at least 25% free for large operations)
	var reservePercent float64
	if tokenCount > 500 {
		reservePercent = 0.25 // 25% reserve for very large operations
	} else {
		reservePercent = 0.20 // 20% reserve for normal operations
	}
	reserveMB := uint64(float64(totalMB) * reservePercent)
	usableMB := availableMB - reserveMB
	
	// Ensure we have at least 2GB usable
	if usableMB < 2048 {
		return 1 // Minimal workers when memory is critically low
	}
	
	// Dynamic memory per worker based on token count
	// For pinning operations, we need less memory per worker
	// More workers with less memory each is better for IPFS stability
	var memoryPerWorkerMB uint64
	switch {
	case tokenCount <= 100:
		memoryPerWorkerMB = 512 // 512MB per worker for small batches
	case tokenCount <= 250:
		memoryPerWorkerMB = 768 // 768MB per worker for medium batches
	case tokenCount <= 500:
		memoryPerWorkerMB = 1024 // 1GB per worker for large batches
	case tokenCount <= 1000:
		memoryPerWorkerMB = 1536 // 1.5GB per worker for 1000 tokens
	default:
		memoryPerWorkerMB = 2048 // 2GB per worker for very large batches
	}
	
	// Calculate workers based on available memory
	memoryBasedWorkers := int(usableMB / memoryPerWorkerMB)
	if memoryBasedWorkers < 1 {
		memoryBasedWorkers = 1
	}
	
	// Optimal workers based on token count for balance of speed and stability
	var optimalWorkers int
	cpuCount := runtime.NumCPU()
	
	// For stability: fewer workers for larger operations to prevent memory exhaustion
	switch {
	case tokenCount <= 10:
		optimalWorkers = min(tokenCount, cpuCount) // One worker per token for tiny batches
	case tokenCount <= 50:
		optimalWorkers = max(cpuCount/2, 4) // Half CPUs but at least 4
	case tokenCount <= 100:
		optimalWorkers = max(cpuCount, 8) // Full CPU count but at least 8
	case tokenCount <= 250:
		optimalWorkers = min(16, cpuCount*2) // Up to 16 workers
	case tokenCount <= 500:
		optimalWorkers = min(12, cpuCount) // Reduce to 12 workers for stability
	case tokenCount <= 1000:
		optimalWorkers = min(10, cpuCount) // Further reduce to 10 workers
	default:
		optimalWorkers = min(8, cpuCount/2) // Minimum workers for very large batches
	}
	
	// Use the smaller of memory-based and optimal calculations
	workers := min(memoryBasedWorkers, optimalWorkers)
	
	// Ensure at least 1 worker
	if workers < 1 {
		workers = 1
	}
	
	return workers
}

// GetResourceStats returns current resource utilization
func (rm *ResourceMonitor) GetResourceStats() map[string]interface{} {
	totalMB, availableMB := rm.GetMemoryStats()
	usedMB := totalMB - availableMB
	usagePercent := float64(usedMB) / float64(totalMB) * 100
	
	return map[string]interface{}{
		"memory_total_mb":     totalMB,
		"memory_available_mb": availableMB,
		"memory_used_mb":      usedMB,
		"memory_usage_pct":    usagePercent,
		"cpu_count":           runtime.NumCPU(),
		"goroutines":          runtime.NumGoroutine(),
	}
}