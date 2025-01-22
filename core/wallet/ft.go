package wallet

type FTToken struct {
	TokenID        string  `gorm:"column:token_id;primaryKey"`
	FTName         string  `gorm:"column:ft_name"`
	FTSymbol       string  `gorm:"column:ft_symbol"`
	DID            string  `gorm:"column:owner_did"`
	CreatorDID     string  `gorm:"column:creator_did"`
	TokenStatus    int     `gorm:"column:token_status"`
	TokenValue     float64 `gorm:"column:token_value"`
	TokenStateHash string  `gorm:"column:token_state_hash"`
	TransactionID  string  `gorm:"column:transaction_id"`
}

type FT struct {
	ID         string `gorm:"column:id;primaryKey;autoIncrement"`
	FTName     string `gorm:"column:ft_name"`
	FTSymbol   string `gorm:"column:ft_symbol"`
	FTCount    int    `gorm:"column:ft_count"`
	CreatorDID string `gorm:"column:creator_did"`
}
