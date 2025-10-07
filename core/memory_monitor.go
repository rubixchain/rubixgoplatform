package core

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryMonitor tracks memory usage and provides backpressure mechanisms
type MemoryMonitor struct {
	mu                    sync.RWMutex
	lastMemStats          runtime.MemStats
	lastUpdate            time.Time
	highMemoryThreshold   uint64 // Bytes
	criticalMemoryThreshold uint64 // Bytes
	isHighMemory          atomic.Bool
	isCriticalMemory      atomic.Bool
}

// NewMemoryMonitor creates a new memory monitor
func NewMemoryMonitor() *MemoryMonitor {
	mm := &MemoryMonitor{
		highMemoryThreshold:     80, // 80% of system memory
		criticalMemoryThreshold: 90, // 90% of system memory
	}
	
	// Initial read
	runtime.ReadMemStats(&mm.lastMemStats)
	mm.lastUpdate = time.Now()
	
	// Start monitoring goroutine
	go mm.monitorLoop()
	
	return mm
}

// monitorLoop continuously monitors memory usage
func (mm *MemoryMonitor) monitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		mm.updateMemStats()
	}
}

// updateMemStats updates memory statistics
func (mm *MemoryMonitor) updateMemStats() {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	runtime.ReadMemStats(&mm.lastMemStats)
	mm.lastUpdate = time.Now()
	
	// Calculate memory usage percentage
	usedMemory := mm.lastMemStats.Alloc
	totalMemory := mm.lastMemStats.Sys
	usagePercent := (usedMemory * 100) / totalMemory
	
	// Update thresholds
	mm.isHighMemory.Store(usagePercent >= mm.highMemoryThreshold)
	mm.isCriticalMemory.Store(usagePercent >= mm.criticalMemoryThreshold)
}

// GetMemoryStats returns current memory statistics
func (mm *MemoryMonitor) GetMemoryStats() (used, total, available uint64) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	// If stats are stale, update them
	if time.Since(mm.lastUpdate) > 10*time.Second {
		mm.mu.RUnlock()
		mm.updateMemStats()
		mm.mu.RLock()
	}
	
	used = mm.lastMemStats.Alloc
	total = mm.lastMemStats.Sys
	available = total - used
	return
}

// IsMemoryPressureHigh returns true if memory usage is high
func (mm *MemoryMonitor) IsMemoryPressureHigh() bool {
	return mm.isHighMemory.Load()
}

// IsMemoryPressureCritical returns true if memory usage is critical
func (mm *MemoryMonitor) IsMemoryPressureCritical() bool {
	return mm.isCriticalMemory.Load()
}

// WaitForMemory blocks until memory pressure is reduced or timeout
func (mm *MemoryMonitor) WaitForMemory(timeout time.Duration) bool {
	if !mm.IsMemoryPressureHigh() {
		return true
	}
	
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
			if !mm.IsMemoryPressureHigh() {
				return true
			}
			// Force GC to try to free memory
			runtime.GC()
		}
	}
	
	return false
}

// ForceGC forces garbage collection and returns freed memory
func (mm *MemoryMonitor) ForceGC() uint64 {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	
	runtime.GC()
	runtime.GC() // Double GC for thorough cleanup
	
	runtime.ReadMemStats(&mm.lastMemStats)
	mm.lastUpdate = time.Now()
	
	if mm.lastMemStats.Alloc < before.Alloc {
		return before.Alloc - mm.lastMemStats.Alloc
	}
	return 0
}