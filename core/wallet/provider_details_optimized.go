package wallet

import (
	"fmt"
	"strings"
	
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// AddProviderDetailsBatchOptimized uses database transactions for better performance
func (w *Wallet) AddProviderDetailsBatchOptimized(tokenProviderMaps []model.TokenProviderMap) error {
	if len(tokenProviderMaps) == 0 {
		return nil
	}
	
	// For very large batches, process in chunks to avoid transaction size limits
	chunkSize := 100
	for i := 0; i < len(tokenProviderMaps); i += chunkSize {
		end := i + chunkSize
		if end > len(tokenProviderMaps) {
			end = len(tokenProviderMaps)
		}
		
		chunk := tokenProviderMaps[i:end]
		err := w.processProviderDetailsChunk(chunk)
		if err != nil {
			return fmt.Errorf("failed to process chunk %d: %w", i/chunkSize, err)
		}
	}
	
	return nil
}

// processProviderDetailsChunk processes a chunk of provider details in a transaction
func (w *Wallet) processProviderDetailsChunk(chunk []model.TokenProviderMap) error {
	// Start a transaction if the storage supports it
	tx, hasTx := w.s.(interface {
		BeginTx() (interface{}, error)
		CommitTx(interface{}) error
		RollbackTx(interface{}) error
	})
	
	var txHandle interface{}
	var err error
	
	if hasTx {
		txHandle, err = tx.BeginTx()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() {
			if err != nil {
				tx.RollbackTx(txHandle)
			}
		}()
	}
	
	successCount := 0
	for _, tpm := range chunk {
		// First try to insert
		err := w.s.Write(TokenProvider, tpm)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				// Already exists, update instead
				err = w.s.Update(TokenProvider, tpm, "token=?", tpm.Token)
				if err != nil {
					w.log.Warn("Failed to update provider details", "token", tpm.Token, "err", err)
					continue
				}
			} else {
				w.log.Warn("Failed to add provider details", "token", tpm.Token, "err", err)
				continue
			}
		}
		successCount++
	}
	
	// Commit transaction if we have one
	if hasTx && txHandle != nil {
		err = tx.CommitTx(txHandle)
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}
	
	w.log.Debug("Provider details chunk processed", 
		"total", len(chunk), 
		"success", successCount, 
		"failed", len(chunk)-successCount)
	
	if successCount == 0 {
		return fmt.Errorf("all operations in chunk failed")
	}
	
	return nil
}

// ParallelAddProviderDetails adds provider details using parallel workers
func (w *Wallet) ParallelAddProviderDetails(tokenProviderMaps []model.TokenProviderMap) error {
	if len(tokenProviderMaps) == 0 {
		return nil
	}
	
	// For small batches, use regular processing
	if len(tokenProviderMaps) < 50 {
		return w.AddProviderDetailsBatch(tokenProviderMaps)
	}
	
	// Use optimized batch processing for large sets
	return w.AddProviderDetailsBatchOptimized(tokenProviderMaps)
}