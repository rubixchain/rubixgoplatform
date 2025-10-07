package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PerformanceAnalyzer analyzes performance data
type PerformanceAnalyzer struct {
	dataPath string
}

// AnalysisReport contains performance analysis results
type AnalysisReport struct {
	StartTime          time.Time
	EndTime            time.Time
	TotalOperations    int
	TopBottlenecks     []BottleneckInfo
	OperationBreakdown map[string]OperationStats
	TimelineAnalysis   []TimelinePoint
	Recommendations    []string
}

// BottleneckInfo identifies slow operations
type BottleneckInfo struct {
	Operation   string
	AvgDuration time.Duration
	MaxDuration time.Duration
	Count       int
	Impact      float64 // Percentage of total time
}

// OperationStats contains statistics for an operation
type OperationStats struct {
	Count         int
	TotalDuration time.Duration
	AvgDuration   time.Duration
	MinDuration   time.Duration
	MaxDuration   time.Duration
	ErrorCount    int
	TokenStats    TokenOperationStats
}

// TokenOperationStats contains token-specific statistics
type TokenOperationStats struct {
	TotalTokens       int
	AvgTokensPerOp    float64
	MaxTokensPerOp    int
	AvgTimePerToken   time.Duration
}

// TimelinePoint represents performance at a point in time
type TimelinePoint struct {
	Timestamp       time.Time
	OpsPerSecond    float64
	AvgResponseTime time.Duration
	ActiveOps       int
}

// NewPerformanceAnalyzer creates a new analyzer
func NewPerformanceAnalyzer(dataPath string) *PerformanceAnalyzer {
	return &PerformanceAnalyzer{
		dataPath: dataPath,
	}
}

// GenerateReport analyzes performance data and generates a report
func (pa *PerformanceAnalyzer) GenerateReport(startTime, endTime time.Time) (*AnalysisReport, error) {
	report := &AnalysisReport{
		StartTime:          startTime,
		EndTime:            endTime,
		OperationBreakdown: make(map[string]OperationStats),
	}
	
	// Read and analyze performance files
	perfDir := filepath.Join(pa.dataPath, "performance")
	files, err := pa.getFilesInTimeRange(perfDir, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}
	
	// Process each file
	for _, file := range files {
		if err := pa.processFile(file, report, startTime, endTime); err != nil {
			return nil, fmt.Errorf("failed to process file %s: %w", file, err)
		}
	}
	
	// Generate analysis
	pa.identifyBottlenecks(report)
	pa.analyzeTimeline(report)
	pa.generateRecommendations(report)
	
	return report, nil
}

// processFile reads and processes a single performance file
func (pa *PerformanceAnalyzer) processFile(filepath string, report *AnalysisReport, startTime, endTime time.Time) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry TimingEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Skip malformed entries
		}
		
		// Check if entry is within time range
		if entry.Timestamp.Before(startTime) || entry.Timestamp.After(endTime) {
			continue
		}
		
		// Update operation statistics
		pa.updateOperationStats(report, &entry)
		report.TotalOperations++
	}
	
	return scanner.Err()
}

// updateOperationStats updates statistics for an operation
func (pa *PerformanceAnalyzer) updateOperationStats(report *AnalysisReport, entry *TimingEntry) {
	stats, exists := report.OperationBreakdown[entry.Operation]
	if !exists {
		stats = OperationStats{
			MinDuration: time.Duration(entry.Duration) * time.Microsecond,
		}
	}
	
	duration := time.Duration(entry.Duration) * time.Microsecond
	
	stats.Count++
	stats.TotalDuration += duration
	stats.AvgDuration = stats.TotalDuration / time.Duration(stats.Count)
	
	if duration < stats.MinDuration {
		stats.MinDuration = duration
	}
	if duration > stats.MaxDuration {
		stats.MaxDuration = duration
	}
	
	if entry.Error != "" {
		stats.ErrorCount++
	}
	
	// Update token statistics
	if entry.TokenCount > 0 {
		stats.TokenStats.TotalTokens += entry.TokenCount
		if entry.TokenCount > stats.TokenStats.MaxTokensPerOp {
			stats.TokenStats.MaxTokensPerOp = entry.TokenCount
		}
		stats.TokenStats.AvgTokensPerOp = float64(stats.TokenStats.TotalTokens) / float64(stats.Count)
		stats.TokenStats.AvgTimePerToken = duration / time.Duration(entry.TokenCount)
	}
	
	report.OperationBreakdown[entry.Operation] = stats
}

// identifyBottlenecks finds the slowest operations
func (pa *PerformanceAnalyzer) identifyBottlenecks(report *AnalysisReport) {
	// Calculate total time spent
	var totalTime time.Duration
	for _, stats := range report.OperationBreakdown {
		totalTime += stats.TotalDuration
	}
	
	// Create bottleneck list
	bottlenecks := make([]BottleneckInfo, 0)
	for op, stats := range report.OperationBreakdown {
		if stats.Count == 0 {
			continue
		}
		
		impact := float64(stats.TotalDuration) / float64(totalTime) * 100
		bottlenecks = append(bottlenecks, BottleneckInfo{
			Operation:   op,
			AvgDuration: stats.AvgDuration,
			MaxDuration: stats.MaxDuration,
			Count:       stats.Count,
			Impact:      impact,
		})
	}
	
	// Sort by impact
	sort.Slice(bottlenecks, func(i, j int) bool {
		return bottlenecks[i].Impact > bottlenecks[j].Impact
	})
	
	// Keep top 10
	if len(bottlenecks) > 10 {
		bottlenecks = bottlenecks[:10]
	}
	
	report.TopBottlenecks = bottlenecks
}

// analyzeTimeline creates a timeline analysis
func (pa *PerformanceAnalyzer) analyzeTimeline(report *AnalysisReport) {
	// This is a simplified implementation
	// In a real implementation, you would bucket operations by time
	report.TimelineAnalysis = []TimelinePoint{
		{
			Timestamp:       report.StartTime,
			OpsPerSecond:    float64(report.TotalOperations) / report.EndTime.Sub(report.StartTime).Seconds(),
			AvgResponseTime: pa.calculateOverallAvgDuration(report),
		},
	}
}

// generateRecommendations creates optimization recommendations
func (pa *PerformanceAnalyzer) generateRecommendations(report *AnalysisReport) {
	recommendations := []string{}
	
	for _, bottleneck := range report.TopBottlenecks {
		// Token state validation recommendations
		if strings.Contains(bottleneck.Operation, "token.state_check") {
			if bottleneck.AvgDuration > 100*time.Millisecond {
				recommendations = append(recommendations, 
					fmt.Sprintf("Consider caching token state for %s (avg: %v)", 
						bottleneck.Operation, bottleneck.AvgDuration))
			}
		}
		
		// IPFS operation recommendations
		if strings.Contains(bottleneck.Operation, "ipfs.") {
			if bottleneck.Impact > 20 {
				recommendations = append(recommendations,
					fmt.Sprintf("IPFS operations consuming %.1f%% of time - consider batching or caching",
						bottleneck.Impact))
			}
		}
		
		// Database operation recommendations
		if strings.Contains(bottleneck.Operation, "db.") {
			if bottleneck.Count > 1000 && bottleneck.AvgDuration > 10*time.Millisecond {
				recommendations = append(recommendations,
					fmt.Sprintf("Database operation %s called %d times - consider batching",
						bottleneck.Operation, bottleneck.Count))
			}
		}
		
		// Parallel operation recommendations
		if strings.Contains(bottleneck.Operation, "parallel") {
			stats := report.OperationBreakdown[bottleneck.Operation]
			if stats.TokenStats.AvgTokensPerOp > 0 {
				throughput := float64(stats.TokenStats.TotalTokens) / bottleneck.AvgDuration.Seconds()
				if throughput < 100 { // Less than 100 tokens/second
					recommendations = append(recommendations,
						fmt.Sprintf("Low throughput in %s (%.0f tokens/sec) - check worker count",
							bottleneck.Operation, throughput))
				}
			}
		}
	}
	
	// General recommendations based on error rates
	for op, stats := range report.OperationBreakdown {
		if stats.Count > 0 {
			errorRate := float64(stats.ErrorCount) / float64(stats.Count) * 100
			if errorRate > 5 {
				recommendations = append(recommendations,
					fmt.Sprintf("High error rate in %s (%.1f%%) - investigate failures",
						op, errorRate))
			}
		}
	}
	
	report.Recommendations = recommendations
}

// getFilesInTimeRange finds performance files within the time range
func (pa *PerformanceAnalyzer) getFilesInTimeRange(dir string, startTime, endTime time.Time) ([]string, error) {
	var files []string
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "perf_") {
			continue
		}
		
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		// Check if file might contain data in our time range
		if info.ModTime().After(startTime.Add(-24*time.Hour)) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	
	return files, nil
}

// calculateOverallAvgDuration calculates the overall average duration
func (pa *PerformanceAnalyzer) calculateOverallAvgDuration(report *AnalysisReport) time.Duration {
	var totalDuration time.Duration
	var totalCount int
	
	for _, stats := range report.OperationBreakdown {
		totalDuration += stats.TotalDuration
		totalCount += stats.Count
	}
	
	if totalCount == 0 {
		return 0
	}
	
	return totalDuration / time.Duration(totalCount)
}

// ExportReport exports the report in the specified format
func (pa *PerformanceAnalyzer) ExportReport(report *AnalysisReport, format string, outputPath string) error {
	switch format {
	case "json":
		return pa.exportJSON(report, outputPath)
	case "text":
		return pa.exportText(report, outputPath)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// exportJSON exports report as JSON
func (pa *PerformanceAnalyzer) exportJSON(report *AnalysisReport, outputPath string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(outputPath, data, 0644)
}

// exportText exports report as human-readable text
func (pa *PerformanceAnalyzer) exportText(report *AnalysisReport, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	w := bufio.NewWriter(file)
	defer w.Flush()
	
	// Write header
	fmt.Fprintf(w, "Performance Analysis Report\n")
	fmt.Fprintf(w, "========================\n\n")
	fmt.Fprintf(w, "Time Range: %s to %s\n", report.StartTime.Format(time.RFC3339), report.EndTime.Format(time.RFC3339))
	fmt.Fprintf(w, "Total Operations: %d\n\n", report.TotalOperations)
	
	// Write bottlenecks
	fmt.Fprintf(w, "Top Bottlenecks:\n")
	fmt.Fprintf(w, "---------------\n")
	for i, b := range report.TopBottlenecks {
		fmt.Fprintf(w, "%d. %s\n", i+1, b.Operation)
		fmt.Fprintf(w, "   Impact: %.1f%% of total time\n", b.Impact)
		fmt.Fprintf(w, "   Avg Duration: %v\n", b.AvgDuration)
		fmt.Fprintf(w, "   Max Duration: %v\n", b.MaxDuration)
		fmt.Fprintf(w, "   Count: %d\n\n", b.Count)
	}
	
	// Write recommendations
	fmt.Fprintf(w, "Recommendations:\n")
	fmt.Fprintf(w, "---------------\n")
	for i, rec := range report.Recommendations {
		fmt.Fprintf(w, "%d. %s\n", i+1, rec)
	}
	
	return nil
}

// AnalyzeRealTime provides real-time performance insights
func (pa *PerformanceAnalyzer) AnalyzeRealTime(w io.Writer) error {
	// Get report for last hour
	endTime := time.Now()
	startTime := endTime.Add(-time.Hour)
	
	report, err := pa.GenerateReport(startTime, endTime)
	if err != nil {
		return err
	}
	
	// Write summary
	fmt.Fprintf(w, "\nReal-time Performance Summary (Last Hour):\n")
	fmt.Fprintf(w, "=========================================\n\n")
	
	// Top operations by time
	fmt.Fprintf(w, "Top Operations by Total Time:\n")
	for i, b := range report.TopBottlenecks {
		if i >= 5 {
			break
		}
		fmt.Fprintf(w, "  %s: %.1f%% (avg: %v)\n", b.Operation, b.Impact, b.AvgDuration)
	}
	
	// Operations with high error rates
	fmt.Fprintf(w, "\nOperations with Errors:\n")
	for op, stats := range report.OperationBreakdown {
		if stats.ErrorCount > 0 {
			errorRate := float64(stats.ErrorCount) / float64(stats.Count) * 100
			fmt.Fprintf(w, "  %s: %.1f%% error rate (%d/%d)\n", op, errorRate, stats.ErrorCount, stats.Count)
		}
	}
	
	return nil
}