package wallet

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// struct definition for Mapping token and reason the did is a provider

// Method takes token hash as input and returns the Provider details
func (w *Wallet) GetProviderDetails(token string) (*model.TokenProviderMap, error) {
	var tokenMap model.TokenProviderMap
	err := w.s.Read(TokenProvider, &tokenMap, "token=?", token)
	if err != nil {
		if err.Error() == "no records found" {
			//w.log.Debug("Data Not avilable in DB")
			return &tokenMap, err
		} else {
			w.log.Error("Error fetching details from DB", "error", err)
			return &tokenMap, err
		}
	}
	return &tokenMap, nil
}

// Method to add provider details to DB during ipfs ops
// checks if entry exist for token,did either write or updates

func (w *Wallet) AddProviderDetails(tokenProviderMap model.TokenProviderMap) error {
	var tpm model.TokenProviderMap
	err := w.s.Read(TokenProvider, &tpm, "token=?", tokenProviderMap.Token)
	if err != nil || tpm.Token == "" {
		w.log.Debug("Token Details not found: Creating new Record")
		// create new entry, but handle unique constraint error
		writeErr := w.s.Write(TokenProvider, tokenProviderMap)
		if writeErr != nil && strings.Contains(writeErr.Error(), "UNIQUE constraint failed") {
			// Someone else inserted, so update instead
			return w.s.Update(TokenProvider, tokenProviderMap, "token=?", tokenProviderMap.Token)
		}
		return writeErr
	}
	return w.s.Update(TokenProvider, tokenProviderMap, "token=?", tokenProviderMap.Token)
}

// Method to add provider details to DB in batch during ipfs ops
// Accepts a slice of TokenProviderMap and writes/updates them efficiently
func (w *Wallet) AddProviderDetailsBatch(tokenProviderMaps []model.TokenProviderMap) error {
	if len(tokenProviderMaps) == 0 {
		return nil
	}
	
	// Process each token provider individually to handle UNIQUE constraints properly
	// We don't use WriteBatch because it doesn't handle UNIQUE constraint violations
	var lastErr error
	successCount := 0
	
	for _, tpm := range tokenProviderMaps {
		err := w.AddProviderDetails(tpm)
		if err != nil {
			// Log the error but continue processing other entries
			w.log.Warn("Failed to add provider details in batch", "token", tpm.Token, "did", tpm.DID, "err", err)
			lastErr = err
			// If it's not a UNIQUE constraint error, it might be more serious
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				w.log.Error("Non-recoverable error in batch provider details", "err", err)
			}
		} else {
			successCount++
		}
	}
	
	// Log summary
	w.log.Debug("Batch provider details completed", 
		"total", len(tokenProviderMaps), 
		"success", successCount, 
		"failed", len(tokenProviderMaps)-successCount)
	
	// Return error only if all operations failed
	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("all provider detail operations failed: %w", lastErr)
	}
	
	return nil
}

// Method deletes entry ffrom DB during unpin op
func (w *Wallet) RemoveProviderDetails(token string, did string) error {
	return w.s.Delete(TokenProvider, nil, "did=? AND token=?", did, token)
}
