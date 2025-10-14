package model

import "time"

// AdvisoryURL represents an advisory node service URL configuration
type AdvisoryURL struct {
	ID          int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	URL         string    `gorm:"column:url;not null" json:"url"`
	NetworkType string    `gorm:"column:network_type;not null" json:"network_type"` // "mainnet" or "testnet"
	IsDefault   bool      `gorm:"column:is_default;default:false" json:"is_default"`
	IsActive    bool      `gorm:"column:is_active;default:true" json:"is_active"`
	Priority    int       `gorm:"column:priority;default:1" json:"priority"` // Lower number = higher priority
	Description string    `gorm:"column:description" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	LastTested  time.Time `gorm:"column:last_tested" json:"last_tested"`
	IsHealthy   bool      `gorm:"column:is_healthy;default:true" json:"is_healthy"`
}

// TableName returns the table name for GORM
func (AdvisoryURL) TableName() string {
	return "advisory_urls"
}

// AdvisoryURLRequest represents request payload for CRUD operations
type AdvisoryURLRequest struct {
	URL         string `json:"url" binding:"required"`
	NetworkType string `json:"network_type" binding:"required"` // "mainnet" or "testnet"
	IsDefault   bool   `json:"is_default"`
	IsActive    bool   `json:"is_active"`
	Priority    int    `json:"priority"`
	Description string `json:"description"`
}

// AdvisoryURLResponse represents response for advisory URL operations
type AdvisoryURLResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

