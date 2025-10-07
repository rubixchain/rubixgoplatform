package model

import (
	"time"
)

// FTTransactionToken stores the relationship between transactions and FT tokens
// This persists even after tokens are transferred out
type FTTransactionToken struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	TransactionID string    `gorm:"column:transaction_id;index;not null"`
	CreatorDID    string    `gorm:"column:creator_did;not null"`
	FTName        string    `gorm:"column:ft_name;not null"`
	TokenCount    int       `gorm:"column:token_count;not null"`
	Direction     string    `gorm:"column:direction;not null"` // "sent" or "received"
	CreatedAt     time.Time `gorm:"column:created_at"`
}

