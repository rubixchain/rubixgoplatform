package model

import (
	"time"
)

// FTTransactionHistory stores complete FT transaction details
// This is separate from regular TransactionDetails to have FT-specific fields
type FTTransactionHistory struct {
	ID              uint      `gorm:"primaryKey;autoIncrement"`
	TransactionID   string    `gorm:"column:transaction_id;uniqueIndex;not null"`
	TransactionType string    `gorm:"column:transaction_type"`
	BlockID         string    `gorm:"column:block_id"`
	Mode            int       `gorm:"column:mode"`
	SenderDID       string    `gorm:"column:sender_did;index"`
	ReceiverDID     string    `gorm:"column:receiver_did;index"`
	Amount          float64   `gorm:"column:amount"` // Total number of tokens
	TotalTime       float64   `gorm:"column:total_time"`
	Comment         string    `gorm:"column:comment"`
	DateTime        time.Time `gorm:"column:date_time"`
	Status          bool      `gorm:"column:status"`
	DeployerDID     string    `gorm:"column:deployer_did"`
	Epoch           int64     `gorm:"column:epoch"`
	FTName          string    `gorm:"column:ft_name;index"`
	CreatorDID      string    `gorm:"column:creator_did;index"`
	TokenCount      int       `gorm:"column:token_count"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (FTTransactionHistory) TableName() string {
	return "ft_transaction_history"
}