package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// TokenSyncManager manages token chain synchronization to prevent race conditions
type TokenSyncManager struct {
	log logger.Logger
	
	// Track ongoing syncs to prevent duplicates
	mu        sync.RWMutex
	syncState map[string]*tokenSyncInfo
}

type tokenSyncInfo struct {
	token     string
	startTime time.Time
	inProgress bool
	mu        sync.Mutex
}

// NewTokenSyncManager creates a new token sync manager
func NewTokenSyncManager(log logger.Logger) *TokenSyncManager {
	return &TokenSyncManager{
		log:       log.Named("TokenSyncManager"),
		syncState: make(map[string]*tokenSyncInfo),
	}
}

// AcquireSyncLock attempts to acquire a lock for syncing a specific token
// Returns true if lock acquired, false if sync already in progress
func (tsm *TokenSyncManager) AcquireSyncLock(token string) bool {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	
	// Check if sync already in progress
	if info, exists := tsm.syncState[token]; exists && info.inProgress {
		// Check if sync has been running too long (timeout after 5 minutes)
		if time.Since(info.startTime) > 5*time.Minute {
			tsm.log.Warn("Token sync timeout detected, allowing new sync", "token", token)
			info.inProgress = false
		} else {
			tsm.log.Debug("Token sync already in progress", "token", token, "duration", time.Since(info.startTime))
			return false
		}
	}
	
	// Create or update sync info
	if info, exists := tsm.syncState[token]; exists {
		info.inProgress = true
		info.startTime = time.Now()
	} else {
		tsm.syncState[token] = &tokenSyncInfo{
			token:      token,
			startTime:  time.Now(),
			inProgress: true,
		}
	}
	
	tsm.log.Debug("Acquired token sync lock", "token", token)
	return true
}

// ReleaseSyncLock releases the sync lock for a token
func (tsm *TokenSyncManager) ReleaseSyncLock(token string) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	
	if info, exists := tsm.syncState[token]; exists {
		info.inProgress = false
		tsm.log.Debug("Released token sync lock", "token", token, "duration", time.Since(info.startTime))
	}
}

// IsSyncInProgress checks if a sync is in progress for a token
func (tsm *TokenSyncManager) IsSyncInProgress(token string) bool {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()
	
	if info, exists := tsm.syncState[token]; exists && info.inProgress {
		// Check timeout
		if time.Since(info.startTime) > 5*time.Minute {
			return false
		}
		return true
	}
	return false
}

// WaitForSync waits for an ongoing sync to complete with timeout
func (tsm *TokenSyncManager) WaitForSync(token string, timeout time.Duration) error {
	start := time.Now()
	checkInterval := 100 * time.Millisecond
	
	for {
		if !tsm.IsSyncInProgress(token) {
			return nil
		}
		
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for token sync to complete")
		}
		
		time.Sleep(checkInterval)
		// Exponential backoff for check interval
		if checkInterval < 2*time.Second {
			checkInterval *= 2
		}
	}
}

// CleanupStaleSync removes sync entries older than specified duration
func (tsm *TokenSyncManager) CleanupStaleSync(maxAge time.Duration) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	
	now := time.Now()
	for token, info := range tsm.syncState {
		if !info.inProgress && now.Sub(info.startTime) > maxAge {
			delete(tsm.syncState, token)
			tsm.log.Debug("Cleaned up stale sync entry", "token", token)
		}
	}
}

// GetSyncStats returns statistics about ongoing syncs
func (tsm *TokenSyncManager) GetSyncStats() map[string]interface{} {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()
	
	activeSyncs := 0
	totalSyncs := len(tsm.syncState)
	var longestSync time.Duration
	var longestToken string
	
	for token, info := range tsm.syncState {
		if info.inProgress {
			activeSyncs++
			duration := time.Since(info.startTime)
			if duration > longestSync {
				longestSync = duration
				longestToken = token
			}
		}
	}
	
	return map[string]interface{}{
		"active_syncs":   activeSyncs,
		"total_entries":  totalSyncs,
		"longest_sync":   longestSync.String(),
		"longest_token":  longestToken,
	}
}