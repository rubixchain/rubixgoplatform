package wallet

type FTToken struct {
	TokenID        string  `gorm:"column:token_id;primaryKey"`
	FTName         string  `gorm:"column:ft_name"`
	DID            string  `gorm:"column:owner_did"`
	CreatorDID     string  `gorm:"column:creator_did"`
	TokenStatus    int     `gorm:"column:token_status"`
	TokenValue     float64 `gorm:"column:token_value"`
	TokenStateHash string  `gorm:"column:token_state_hash"`
	TransactionID  string  `gorm:"column:transaction_id"`
	RBTLockStatus  int     `gorm:"column:rbt_lock_status"`
}

type FT struct {
	ID               int    `gorm:"column:id;primaryKey;autoIncrement"`
	FTName           string `gorm:"column:ft_name"`
	FTCreatedCount   int    `gorm:"column:ft_created_count"`
	FTAvailableCount int    `gorm:"column:ft_available_count"`
	CreatorDID       string `gorm:"column:creator_did"`
}

type FTIndex struct {
	FTIndexID  int    `gorm:"column:id;primaryKey;autoIncrement"`
	FTName     string `gorm:"column:ft_name"`
	CreatorDID string `gorm:"column:creator_did"`
	FTIndex    int    `gorm:"column:ft_index"`
}
