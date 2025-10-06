package model

import (
	"time"
)

// FailedFTDownload represents a failed FT token download that needs retry
type FailedFTDownload struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	TransactionID string    `gorm:"column:transaction_id;index;not null"`
	TokenID       string    `gorm:"column:token_id;index;not null"`
	FTName        string    `gorm:"column:ft_name;index"`
	SenderDID     string    `gorm:"column:sender_did"`
	ReceiverDID   string    `gorm:"column:receiver_did;index"`
	TokenPath     string    `gorm:"column:token_path"`
	ErrorMessage  string    `gorm:"column:error_message"`
	RetryCount    int       `gorm:"column:retry_count;default:0"`
	Status        string    `gorm:"column:status;default:'pending'"` // pending, retrying, recovered, failed
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
	LastRetryAt   *time.Time `gorm:"column:last_retry_at"`
}

func (FailedFTDownload) TableName() string {
	return "failed_ft_downloads"
}

// FailedFTDownloadStatus constants
const (
	FailedFTStatusPending   = "pending"
	FailedFTStatusRetrying  = "retrying"
	FailedFTStatusRecovered = "recovered"
	FailedFTStatusFailed    = "failed"
)