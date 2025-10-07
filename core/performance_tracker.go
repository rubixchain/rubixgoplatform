package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// PerformanceTracker tracks timing and performance metrics
type PerformanceTracker struct {
	enabled     bool
	dirPath     string
	currentFile *os.File
	writer      *bufio.Writer
	mu          sync.Mutex
	log         logger.Logger
	
	// In-memory metrics for real-time analysis
	metrics     map[string]*MetricStats
	metricsMu   sync.RWMutex
	
	// Configuration
	retentionHours int
	maxFileSize    int64
	detailLevel    string
	lastRotation   time.Time
}

// MetricStats holds aggregated statistics for a metric
type MetricStats struct {
	Count       int64
	TotalTime   time.Duration
	MinTime     time.Duration
	MaxTime     time.Duration
	AvgTime     time.Duration
	LastUpdated time.Time
}

// TimingEntry represents a single timing measurement
type TimingEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	TxID        string                 `json:"tx_id,omitempty"`
	Operation   string                 `json:"operation"`
	Duration    int64                  `json:"duration_us"` // microseconds
	TokenCount  int                    `json:"token_count,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PerformanceConfig holds performance tracking configuration
type PerformanceConfig struct {
	Enabled         bool     `json:"enabled"`
	DataPath        string   `json:"data_path"`
	RetentionHours  int      `json:"retention_hours"`
	MaxFileSize     int64    `json:"max_file_size_mb"`
	DetailLevel     string   `json:"detail_level"` // "basic", "detailed", "verbose"
	TrackOperations []string `json:"track_operations"`
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker(cfg *PerformanceConfig, log logger.Logger) (*PerformanceTracker, error) {
	if !cfg.Enabled {
		return &PerformanceTracker{enabled: false}, nil
	}
	
	// Create performance directory
	perfDir := filepath.Join(cfg.DataPath, "performance")
	if err := os.MkdirAll(perfDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create performance directory: %w", err)
	}
	
	pt := &PerformanceTracker{
		enabled:        cfg.Enabled,
		dirPath:        cfg.DataPath,
		log:            log.Named("PerformanceTracker"),
		metrics:        make(map[string]*MetricStats),
		retentionHours: cfg.RetentionHours,
		maxFileSize:    cfg.MaxFileSize * 1024 * 1024, // Convert MB to bytes
		detailLevel:    cfg.DetailLevel,
		lastRotation:   time.Now(),
	}
	
	// Initialize first log file
	if err := pt.rotateFile(); err != nil {
		return nil, err
	}
	
	// Start cleanup routine
	go pt.cleanupOldFiles()
	
	return pt, nil
}

// TrackOperation tracks a single operation with timing
func (pt *PerformanceTracker) TrackOperation(operation string, metadata map[string]interface{}) func(error) {
	if !pt.enabled {
		return func(error) {}
	}
	
	start := time.Now()
	
	return func(err error) {
		duration := time.Since(start)
		
		entry := TimingEntry{
			Timestamp: start,
			Operation: operation,
			Duration:  duration.Microseconds(),
			Metadata:  metadata,
		}
		
		if err != nil {
			entry.Error = err.Error()
		}
		
		// Get current transaction ID if available
		if txID := pt.getCurrentTxID(); txID != "" {
			entry.TxID = txID
		}
		
		// Extract token count if provided
		if tc, ok := metadata["token_count"].(int); ok {
			entry.TokenCount = tc
		}
		
		// Record the entry
		pt.record(entry)
		
		// Update in-memory stats
		pt.updateStats(operation, duration)
	}
}

// record writes a timing entry to file
func (pt *PerformanceTracker) record(entry TimingEntry) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	
	// Check if rotation is needed
	if pt.shouldRotate() {
		if err := pt.rotateFile(); err != nil {
			pt.log.Error("Failed to rotate performance file", "error", err)
			return
		}
	}
	
	// Encode and write entry
	data, err := json.Marshal(entry)
	if err != nil {
		pt.log.Error("Failed to marshal timing entry", "error", err)
		return
	}
	
	if _, err := pt.writer.Write(data); err != nil {
		pt.log.Error("Failed to write timing entry", "error", err)
		return
	}
	
	if _, err := pt.writer.WriteString("\n"); err != nil {
		pt.log.Error("Failed to write newline", "error", err)
		return
	}
	
	// Flush periodically
	if err := pt.writer.Flush(); err != nil {
		pt.log.Error("Failed to flush writer", "error", err)
	}
}

// updateStats updates in-memory statistics
func (pt *PerformanceTracker) updateStats(operation string, duration time.Duration) {
	pt.metricsMu.Lock()
	defer pt.metricsMu.Unlock()
	
	stats, exists := pt.metrics[operation]
	if !exists {
		stats = &MetricStats{
			MinTime: duration,
			MaxTime: duration,
		}
		pt.metrics[operation] = stats
	}
	
	stats.Count++
	stats.TotalTime += duration
	stats.AvgTime = stats.TotalTime / time.Duration(stats.Count)
	stats.LastUpdated = time.Now()
	
	if duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}
}

// shouldRotate checks if file rotation is needed
func (pt *PerformanceTracker) shouldRotate() bool {
	if pt.currentFile == nil {
		return true
	}
	
	// Check time-based rotation (hourly)
	if time.Since(pt.lastRotation) >= time.Hour {
		return true
	}
	
	// Check size-based rotation
	fi, err := pt.currentFile.Stat()
	if err != nil {
		return true
	}
	
	return fi.Size() >= pt.maxFileSize
}

// rotateFile creates a new log file
func (pt *PerformanceTracker) rotateFile() error {
	// Close existing file
	if pt.currentFile != nil {
		if pt.writer != nil {
			pt.writer.Flush()
		}
		pt.currentFile.Close()
	}
	
	// Create new file
	now := time.Now()
	filename := fmt.Sprintf("perf_%s.jsonl", now.Format("2006-01-02_15"))
	filepath := filepath.Join(pt.dirPath, "performance", filename)
	
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create performance file: %w", err)
	}
	
	pt.currentFile = file
	pt.writer = bufio.NewWriter(file)
	pt.lastRotation = now
	
	return nil
}

// cleanupOldFiles removes old performance files
func (pt *PerformanceTracker) cleanupOldFiles() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		if !pt.enabled {
			return
		}
		
		perfDir := filepath.Join(pt.dirPath, "performance")
		cutoff := time.Now().Add(-time.Duration(pt.retentionHours) * time.Hour)
		
		files, err := os.ReadDir(perfDir)
		if err != nil {
			pt.log.Error("Failed to read performance directory", "error", err)
			continue
		}
		
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			
			info, err := file.Info()
			if err != nil {
				continue
			}
			
			if info.ModTime().Before(cutoff) {
				filepath := filepath.Join(perfDir, file.Name())
				if err := os.Remove(filepath); err != nil {
					pt.log.Error("Failed to remove old performance file", 
						"file", file.Name(), "error", err)
				}
			}
		}
	}
}

// GetStats returns current performance statistics
func (pt *PerformanceTracker) GetStats() map[string]*MetricStats {
	pt.metricsMu.RLock()
	defer pt.metricsMu.RUnlock()
	
	// Create a copy to avoid race conditions
	stats := make(map[string]*MetricStats)
	for k, v := range pt.metrics {
		copy := *v
		stats[k] = &copy
	}
	
	return stats
}

// Close gracefully shuts down the performance tracker
func (pt *PerformanceTracker) Close() error {
	if !pt.enabled {
		return nil
	}
	
	pt.mu.Lock()
	defer pt.mu.Unlock()
	
	pt.enabled = false
	
	if pt.writer != nil {
		if err := pt.writer.Flush(); err != nil {
			return err
		}
	}
	
	if pt.currentFile != nil {
		return pt.currentFile.Close()
	}
	
	return nil
}

// getCurrentTxID retrieves the current transaction ID from context
// This is a placeholder - actual implementation depends on how txID is stored
func (pt *PerformanceTracker) getCurrentTxID() string {
	// TODO: Implement based on actual context storage
	return ""
}

// Helper function for Core to track operations
func (c *Core) TrackOperation(operation string, metadata map[string]interface{}) func(error) {
	if c.perfTracker == nil {
		return func(error) {}
	}
	return c.perfTracker.TrackOperation(operation, metadata)
}