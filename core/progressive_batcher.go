package core

import (
	"math"
)

// ProgressiveBatchConfig provides dynamic batch sizing based on token count
type ProgressiveBatchConfig struct {
	BaseSize      int
	MaxSize       int
	GrowthFactor  float64
}

// DefaultProgressiveBatchConfig returns default progressive batch configuration
func DefaultProgressiveBatchConfig() *ProgressiveBatchConfig {
	return &ProgressiveBatchConfig{
		BaseSize:     50,
		MaxSize:      500,
		GrowthFactor: 1.5,
	}
}

// GetBatchSize calculates optimal batch size based on total items
func (pbc *ProgressiveBatchConfig) GetBatchSize(totalItems int) int {
	if totalItems <= pbc.BaseSize {
		return totalItems
	}
	
	// Calculate progressive batch size
	// For larger sets, use larger batches to reduce overhead
	batchSize := pbc.BaseSize
	
	switch {
	case totalItems < 100:
		batchSize = pbc.BaseSize
	case totalItems < 500:
		batchSize = int(float64(pbc.BaseSize) * pbc.GrowthFactor)
	case totalItems < 1000:
		batchSize = int(float64(pbc.BaseSize) * math.Pow(pbc.GrowthFactor, 2))
	case totalItems < 5000:
		batchSize = int(float64(pbc.BaseSize) * math.Pow(pbc.GrowthFactor, 3))
	case totalItems < 10000:
		batchSize = int(float64(pbc.BaseSize) * math.Pow(pbc.GrowthFactor, 4))
	default:
		batchSize = pbc.MaxSize
	}
	
	// Cap at max size
	if batchSize > pbc.MaxSize {
		batchSize = pbc.MaxSize
	}
	
	// Ensure we don't have too many small batches
	minBatches := 10
	if totalItems/batchSize > minBatches*10 {
		batchSize = totalItems / (minBatches * 5)
	}
	
	return batchSize
}

// GetOptimalBatches divides items into optimal batch sizes
func (pbc *ProgressiveBatchConfig) GetOptimalBatches(totalItems int) []int {
	if totalItems == 0 {
		return nil
	}
	
	batchSize := pbc.GetBatchSize(totalItems)
	batches := make([]int, 0)
	
	remaining := totalItems
	for remaining > 0 {
		if remaining < batchSize {
			batches = append(batches, remaining)
			break
		}
		batches = append(batches, batchSize)
		remaining -= batchSize
	}
	
	return batches
}

// TokenBatchConfig specific configuration for token operations
var (
	TokenSyncBatchConfig = &ProgressiveBatchConfig{
		BaseSize:     100,
		MaxSize:      1000,
		GrowthFactor: 2.0,
	}
	
	TokenValidationBatchConfig = &ProgressiveBatchConfig{
		BaseSize:     50,
		MaxSize:      500,
		GrowthFactor: 1.5,
	}
	
	ProviderDetailsBatchConfig = &ProgressiveBatchConfig{
		BaseSize:     25,
		MaxSize:      250,
		GrowthFactor: 2.0,
	}
	
	FTLockingBatchConfig = &ProgressiveBatchConfig{
		BaseSize:     100,
		MaxSize:      500,
		GrowthFactor: 1.5,
	}
)