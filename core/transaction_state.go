package core

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// TransactionState represents the current state of a transaction
type TransactionState string

const (
	TxStateInitiated     TransactionState = "initiated"
	TxStateTokensLocked  TransactionState = "tokens_locked"
	TxStateTokensPledged TransactionState = "tokens_pledged"
	TxStateConsensus     TransactionState = "consensus"
	TxStateFinalized     TransactionState = "finalized"
	TxStateFailed        TransactionState = "failed"
	TxStateRolledBack    TransactionState = "rolled_back"
)

// TransactionStateChange represents a state change that can be rolled back
type TransactionStateChange struct {
	Type        string                 `json:"type"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data"`
	RollbackFunc func() error          `json:"-"`
}

// TransactionStateManager manages transaction states and rollback operations
type TransactionStateManager struct {
	mu          sync.RWMutex
	states      map[string]*TransactionStateInfo
	core        *Core
	log         logger.Logger
}

// TransactionStateInfo holds all information about a transaction's state
type TransactionStateInfo struct {
	TransactionID string                    `json:"transaction_id"`
	State         TransactionState          `json:"state"`
	StartTime     time.Time                 `json:"start_time"`
	LastUpdate    time.Time                 `json:"last_update"`
	Participants  TransactionParticipants   `json:"participants"`
	Changes       []TransactionStateChange  `json:"changes"`
	TokenInfo     []TokenInfo               `json:"token_info"`
	Error         string                    `json:"error,omitempty"`
}

// TransactionParticipants holds information about all participants
type TransactionParticipants struct {
	Sender   string   `json:"sender"`
	Receiver string   `json:"receiver"`
	Quorums  []string `json:"quorums"`
}

// TokenInfo holds information about tokens involved in the transaction
type TokenInfo struct {
	TokenID       string  `json:"token_id"`
	TokenValue    float64 `json:"token_value"`
	OriginalState int     `json:"original_state"`
	ParentTokenID string  `json:"parent_token_id,omitempty"`
	IsNewPart     bool    `json:"is_new_part"`
}

// NewTransactionStateManager creates a new transaction state manager
func NewTransactionStateManager(core *Core) *TransactionStateManager {
	return &TransactionStateManager{
		states: make(map[string]*TransactionStateInfo),
		core:   core,
		log:    core.log,
	}
}

// StartTransaction initializes tracking for a new transaction
func (tsm *TransactionStateManager) StartTransaction(txID string, sender, receiver string, quorums []string, tokens []wallet.Token) error {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	if _, exists := tsm.states[txID]; exists {
		return fmt.Errorf("transaction %s already exists", txID)
	}

	tokenInfo := make([]TokenInfo, len(tokens))
	for i, token := range tokens {
		tokenInfo[i] = TokenInfo{
			TokenID:       token.TokenID,
			TokenValue:    token.TokenValue,
			OriginalState: token.TokenStatus,
			ParentTokenID: token.ParentTokenID,
			IsNewPart:     false, // Will be updated if token is split
		}
	}

	tsm.states[txID] = &TransactionStateInfo{
		TransactionID: txID,
		State:         TxStateInitiated,
		StartTime:     time.Now(),
		LastUpdate:    time.Now(),
		Participants: TransactionParticipants{
			Sender:   sender,
			Receiver: receiver,
			Quorums:  quorums,
		},
		Changes:   make([]TransactionStateChange, 0),
		TokenInfo: tokenInfo,
	}

	tsm.log.Info("Started transaction tracking",
		"transaction_id", txID,
		"sender", sender,
		"receiver", receiver,
		"token_count", len(tokens))

	return nil
}

// RecordStateChange records a state change that can be rolled back
func (tsm *TransactionStateManager) RecordStateChange(txID string, changeType string, data map[string]interface{}, rollbackFunc func() error) error {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	info, exists := tsm.states[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	change := TransactionStateChange{
		Type:         changeType,
		Timestamp:    time.Now(),
		Data:         data,
		RollbackFunc: rollbackFunc,
	}

	info.Changes = append(info.Changes, change)
	info.LastUpdate = time.Now()

	tsm.log.Debug("Recorded state change",
		"transaction_id", txID,
		"change_type", changeType)

	return nil
}

// UpdateState updates the transaction state
func (tsm *TransactionStateManager) UpdateState(txID string, newState TransactionState) error {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	info, exists := tsm.states[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	info.State = newState
	info.LastUpdate = time.Now()

	tsm.log.Info("Updated transaction state",
		"transaction_id", txID,
		"new_state", newState)

	return nil
}

// RollbackTransaction rolls back all changes for a failed transaction
func (tsm *TransactionStateManager) RollbackTransaction(txID string) error {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	info, exists := tsm.states[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	if info.State == TxStateRolledBack {
		tsm.log.Info("Transaction already rolled back", "transaction_id", txID)
		return nil
	}

	tsm.log.Info("Starting transaction rollback",
		"transaction_id", txID,
		"current_state", info.State,
		"changes_count", len(info.Changes))

	// Roll back changes in reverse order
	for i := len(info.Changes) - 1; i >= 0; i-- {
		change := info.Changes[i]
		if change.RollbackFunc != nil {
			tsm.log.Debug("Rolling back change",
				"transaction_id", txID,
				"change_type", change.Type,
				"change_index", i)

			if err := change.RollbackFunc(); err != nil {
				tsm.log.Error("Failed to rollback change",
					"transaction_id", txID,
					"change_type", change.Type,
					"error", err)
				// Continue with other rollbacks even if one fails
			}
		}
	}

	info.State = TxStateRolledBack
	info.LastUpdate = time.Now()

	tsm.log.Info("Transaction rollback completed",
		"transaction_id", txID)

	return nil
}

// GetTransactionState returns the current state of a transaction
func (tsm *TransactionStateManager) GetTransactionState(txID string) (*TransactionStateInfo, error) {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	info, exists := tsm.states[txID]
	if !exists {
		return nil, fmt.Errorf("transaction %s not found", txID)
	}

	// Return a copy to prevent external modifications
	infoCopy := *info
	return &infoCopy, nil
}

// CleanupOldTransactions removes old transaction states
func (tsm *TransactionStateManager) CleanupOldTransactions(olderThan time.Duration) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removed := 0

	for txID, info := range tsm.states {
		if info.LastUpdate.Before(cutoff) &&
			(info.State == TxStateFinalized || info.State == TxStateRolledBack) {
			delete(tsm.states, txID)
			removed++
		}
	}

	if removed > 0 {
		tsm.log.Info("Cleaned up old transactions",
			"removed_count", removed)
	}
}

// ExportState exports the transaction state for debugging
func (tsm *TransactionStateManager) ExportState(txID string) (string, error) {
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()

	info, exists := tsm.states[txID]
	if !exists {
		return "", fmt.Errorf("transaction %s not found", txID)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}