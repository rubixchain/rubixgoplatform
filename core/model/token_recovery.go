package model

import (
	"time"
)

// TokenRecovery tracks recovered transactions to prevent double recovery
type TokenRecovery struct {
	TransactionID string    `gorm:"column:transaction_id;primaryKey"`
	RecoveredAt   time.Time `gorm:"column:recovered_at;autoCreateTime"`
	RecoveredBy   string    `gorm:"column:recovered_by"`
	TokenCount    int       `gorm:"column:token_count"`
	TokenIDs      string    `gorm:"column:token_ids"`     // JSON array of token IDs
	RecoveryType  string    `gorm:"column:recovery_type"` // "normal" or "exception"
	RecoveryNotes string    `gorm:"column:recovery_notes"`
}

// RecoveredToken - DEPRECATED: No longer used as tokens can be recovered multiple times
// Kept for reference and potential future use if per-token tracking is needed
type RecoveredToken struct {
	TokenID               string    `gorm:"column:token_id;primaryKey"`
	OriginalTransactionID string    `gorm:"column:original_transaction_id"`
	RecoveredAt           time.Time `gorm:"column:recovered_at;autoCreateTime"`
	RecoveredBy           string    `gorm:"column:recovered_by"`
	RecoveryTransactionID string    `gorm:"column:recovery_transaction_id"` // New transaction after recovery
}

// RemoteRecoveryRequest represents a request to recover tokens for another DID
type RemoteRecoveryRequest struct {
	TargetDID     string `json:"target_did"`     // DID whose tokens should be recovered
	TransactionID string `json:"transaction_id"` // Transaction ID to recover
	RequesterDID  string `json:"requester_did"`  // Optional: DID requesting the recovery
	Reason        string `json:"reason"`         // Optional: Reason for remote recovery
}