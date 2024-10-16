package wallet

type FTToken struct {
	TokenID        string  `gorm:"column:token_id;primaryKey"`
	FTName         string  `gorm:"column:ft_name"`
	DID            string  `gorm:"column:did"`
	TokenStatus    int     `gorm:"column:token_status"`
	TokenValue     float64 `gorm:"column:token_value"`
	TokenStateHash string  `gorm:"column:token_state_hash"`
	TransactionID  string  `gorm:"column:transaction_id"`
}

type FT struct {
	FTName  string `gorm:"column:ft_name;primaryKey"`
	FTCount int    `gorm:"column:ft_count"`
}
