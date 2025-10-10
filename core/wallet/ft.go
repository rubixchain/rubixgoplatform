package wallet

import (
	"time"
)

type FTToken struct {
	TokenID        string    `gorm:"column:token_id;primaryKey"`
	FTName         string    `gorm:"column:ft_name"`
	DID            string    `gorm:"column:owner_did"`
	CreatorDID     string    `gorm:"column:creator_did"`
	TokenStatus    int       `gorm:"column:token_status"`
	TokenValue     float64   `gorm:"column:token_value"`
	TokenStateHash string    `gorm:"column:token_state_hash"`
	TransactionID  string    `gorm:"column:transaction_id"`
	RBTLockStatus  int       `gorm:"column:rbt_lock_status;default:1"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

type FT struct {
	ID               string `gorm:"column:id;primaryKey;autoIncrement"`
	FTName           string `gorm:"column:ft_name"`
	FTCount          int    `gorm:"column:ft_count"`
	FTCreatedCount   int    `gorm:"column:ft_created_count"`
	FTAvailableCount int    `gorm:"column:ft_available_count"`
	CreatorDID       string `gorm:"column:creator_did"`
	HighValueFT      bool   `gorm:"column:high_value_ft"`
}
