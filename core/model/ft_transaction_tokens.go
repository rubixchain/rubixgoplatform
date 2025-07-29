package model

// FTTransactionToken stores the relationship between transactions and FT tokens
// This persists even after tokens are transferred out
type FTTransactionToken struct {
	TransactionID string `gorm:"column:transaction_id;index"`
	CreatorDID    string `gorm:"column:creator_did"`
	FTName        string `gorm:"column:ft_name"`
	TokenCount    int    `gorm:"column:token_count"`
	Direction     string `gorm:"column:direction"` // "sent" or "received"
}